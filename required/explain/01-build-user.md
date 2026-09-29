# Các bước dựng module user

## Bước 1 — Xác định hợp đồng

User trả về đúng ba trường: `id` (số nguyên dương do PostgreSQL cấp), `email`, `name`.
Đầu vào tạo/cập nhật chỉ nhận email và name. Không cho client chọn ID.
Email được trim, chuyển chữ thường, kiểm tra bằng `net/mail`, tối đa 254 byte;
email duy nhất không phân biệt hoa/thường. Đây là quy ước của luồng mẫu, không phải
xác minh email tồn tại. Name được trim, bắt buộc, tối đa 100 ký tự Unicode.

| Method | Path | Kết quả |
| --- | --- | --- |
| POST | /api/v1/users | 201, user mới, header Location |
| GET | /api/v1/users/{id} | 200, một user |
| GET | /api/v1/users?limit=20&offset=0 | 200, mảng user, rỗng là [] |
| PUT | /api/v1/users/{id} | 200, thay cả email và name |
| DELETE | /api/v1/users/{id} | 204, xóa bản ghi, không body |

Danh sách tăng dần theo ID; limit mặc định 20, tối đa 100, offset không âm.
Xóa/cập nhật/đọc ID không tồn tại trả 404; input sai trả 400; email trùng trả 409.
POST/PUT yêu cầu application/json (415 nếu sai), body tối đa 4096 byte (413).
JSON có trường lạ, thiếu trường bắt buộc hoặc nhiều giá trị nối tiếp bị từ chối.
Lỗi ngoài dự kiến trả 500, không lộ SQL, email hay thông tin kết nối.

## Bước 2 — Model: internal/user/user.go

`User` biểu diễn dữ liệu trả về. JSON tag quyết định tên trường trên HTTP.
`Input` tách đầu vào khỏi đầu ra để client không ghi ID.
Các biến lỗi `ErrInvalid`, `ErrNotFound`, `ErrEmailTaken` giúp các lớp trao đổi
kết quả nghiệp vụ mà không phụ thuộc HTTP status hay lỗi driver.

## Bước 3 — Service: internal/user/service.go

Service chứa quy tắc validation/normalization và điều phối CRUD. Validation nằm
ở đây để cả HTTP và module khác gọi trực tiếp đều đi qua cùng quy tắc.
`Repository` là interface mô tả đúng các thao tác service cần.
`NewService(repository)` nhận dependency từ ngoài; service không tự mở DB.
Go kiểm tra interface theo method: implementation không cần khai báo implements.
Trong test có thể thay PostgreSQL bằng repository giả.

## Bước 4 — Persistence: migration và internal/user/repository.go

Migration up tạo bảng users, identity ID, ràng buộc trường bắt buộc và unique index.
Migration down xóa bảng và dữ liệu: chỉ dùng khi đã xác định dữ liệu có thể bỏ.
Không tự chạy migration khi API khởi động.

`PostgresRepository` nhận `*sql.DB` từ pool sẵn có của ứng dụng. Query dùng placeholder
`$1`, `$2`, không ghép input vào SQL. Create/update dùng RETURNING để đọc kết quả
trong cùng statement. Delete kiểm tra số hàng bị ảnh hưởng.
Unique index quyết định tính duy nhất cả khi nhiều request chạy đồng thời;
không dùng cách SELECT trước rồi INSERT vì có thể tranh chấp.
Lỗi unique của index email được đổi thành ErrEmailTaken, sql.ErrNoRows thành ErrNotFound.
List luôn tạo slice rỗng để JSON là [] thay vì null, đồng thời kiểm tra rows.Err().

## Bước 5 — HTTP: internal/user/handler.go

Handler đọc ID/query/body, giới hạn kích thước JSON rồi gọi service.
Handler không viết SQL và không quyết định email có hợp lệ về nghiệp vụ hay không.
Sau lời gọi service, handler đổi lỗi nghiệp vụ sang status và error envelope
theo kiểu có sẵn của dự án: `{"error":{"code":"...","message":"..."}}`.
Mỗi request user có context timeout theo `USER_REQUEST_TIMEOUT` (mặc định `5s`, phải dương);
config được truyền vào handler khi khởi tạo, context đi xuyên service đến query DB.
Đăng ký route bằng chi đang có trong repo, không thêm framework.

## Bước 6 — Nối vào ứng dụng và cập nhật OpenAPI

Điểm khởi tạo ứng dụng cung cấp pool → repository → service → handler → router.
Việc nối nằm trong `cmd/api/application.go`, được `cmd/api/main.go` gọi khi tạo HTTP server.
`cmd/api` khai báo `/api/v1` và gốc `/users`. Handler trong internal/user chỉ đăng ký
`/` và `/{id}`, không biết prefix bên ngoài. Có DB thì đăng ký route ở mọi môi trường;
không có điều kiện riêng cho dev/test/prod. Database được chọn qua DATABASE_URL.
Header Location dựa trên path request, nên vẫn đúng khi mount module sang nhánh khác.
Module user không import cmd/api. Main là nơi biết và nối các thành phần.
Middleware CORS, rate limit và recovery của ứng dụng tiếp tục bao quanh router.
OpenAPI mô tả request, response và lỗi để Swagger thể hiện đúng contract.

## Bước 7 — Kiểm thử từng ranh giới

Unit test kiểm tra normalization, input lỗi và propagation lỗi của service.
HTTP test gửi request qua router, kiểm tra mã trạng thái, JSON, body lỗi và giới hạn.
Integration test dùng PostgreSQL riêng để chạy migration và toàn bộ vòng đời CRUD.
Test contract đối chiếu route thực tế với OpenAPI. Smoke check health/readiness
giúp xác nhận thay đổi không làm hỏng phần nền.

## Theo dõi một request cụ thể

`POST /api/v1/users` → router → handler.create → decode Input → service.Create → normalize
→ repository.Create → INSERT RETURNING → User → handler trả JSON 201.

Nếu email trùng: PostgreSQL unique violation → ErrEmailTaken → HTTP 409.
Nếu name rỗng: service trả ErrInvalid ngay, repository chưa được gọi.

Khi làm module tiếp theo, lặp lại trình tự này với use case thật của module đó;
không cần sao chép CRUD nếu nghiệp vụ khác. Module khác dùng service/interface của
user; không tự sửa bảng users hoặc gọi handler HTTP trong cùng process.
