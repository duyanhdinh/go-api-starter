# Chạy và kiểm chứng CRUD user

## 1. Điều kiện chạy

Dùng Go theo `go.mod` và PostgreSQL theo cấu hình hiện có của repo.
API user được đăng ký khi có pool DB ở mọi môi trường, kể cả `APP_ENV=prod`.
Khi `DB_ENABLED=false`, các path user trả 404. Đổi DB bằng `DATABASE_URL`, không
cần sửa code theo môi trường. DB mới cần chạy migration trước khi dùng CRUD.
Đây là CRUD mẫu chưa có authentication/authorization; chạy local với dữ liệu thử.
Trước khi dùng cho người dùng thật cần xác định ai được đọc, sửa, xóa user và
thực thi quyền đó phía server. APP_ENV không quyết định quyền truy cập.

## 2. Bật PostgreSQL và tạo bảng

Nếu dùng Compose, làm theo phần PostgreSQL trong README: chuẩn bị `.env` với
credential local rồi chạy `docker compose --profile database up -d --wait postgres`.
Không sửa hoặc xóa volume DB đang chứa dữ liệu cần giữ.

Khi chạy Go trên host, cung cấp biến môi trường trong cùng terminal PowerShell:

```powershell
$env:APP_ENV = 'dev'
$env:HTTP_ADDR = '127.0.0.1:8080'
$env:DB_ENABLED = 'true'
$env:DATABASE_URL = 'postgres://<user>:<password>@127.0.0.1:<port>/<database>?sslmode=disable'
go run ./cmd/migrate status
go run ./cmd/migrate up
go run ./cmd/api
```

URL trên là placeholder; thay bằng thông tin DB local của bạn, URL-encode credential
nếu cần. Không commit URL thật. `go run` không tự nạp `.env` của Docker Compose.
API không tự migrate. DB bật nhưng chưa có bảng users sẽ gây lỗi 500 khi gọi CRUD;
`/ready` chỉ kiểm tra kết nối, không kiểm tra schema.

## 3. Gọi từng thao tác

Mở terminal PowerShell khác:

```powershell
$base = 'http://127.0.0.1:8080'
Invoke-RestMethod "$base/health"
Invoke-RestMethod "$base/ready"

$created = Invoke-RestMethod -Method Post -Uri "$base/api/v1/users" -ContentType 'application/json' -Body (@{email='an@example.com';name='An'} | ConvertTo-Json)
$created

Invoke-RestMethod "$base/api/v1/users/$($created.id)"
Invoke-RestMethod "$base/api/v1/users?limit=20&offset=0"

Invoke-RestMethod -Method Put -Uri "$base/api/v1/users/$($created.id)" -ContentType 'application/json' -Body (@{email='an.new@example.com';name='An Updated'} | ConvertTo-Json)

Invoke-RestMethod -Method Delete -Uri "$base/api/v1/users/$($created.id)"
```

Lần lượt kỳ vọng: health/ready 200, tạo 201, đọc/list/update 200, xóa 204 không body.
Đọc lại ID vừa xóa trả 404. Gửi lại email đã tồn tại khi tạo/cập nhật trả 409.
ID có thể có khoảng trống sau một INSERT thất bại; identity không đảm bảo liên tiếp.
List dùng offset nên không đảm bảo snapshot cố định nếu có ghi đồng thời.

Swagger ở `http://127.0.0.1:8080/docs/` khi bật docs. OpenAPI mô tả cả user;
điều kiện bật route được ghi trong mô tả contract. Khi route bị tắt, Swagger gọi
user vẫn nhận 404. Trình duyệt gọi từ origin khác cần cấu hình CORS cho các method
và Content-Type tương ứng; PowerShell không chịu CORS.

## 4. Các mức kiểm thử

```powershell
go test ./internal/user ./cmd/api
go run ./cmd/openapi-validate
go test ./...
go vet ./...
go build ./...
```

`service_test.go`: validation và normalization, không ghi storage khi input sai.
`handler_test.go`: vòng đời CRUD, trùng email, phân trang, JSON lỗi, body quá lớn,
Content-Type, 405 và không lộ lỗi storage. Repository trong các test này là fake
trong bộ nhớ, chỉ tồn tại ở test, không phải database fallback của ứng dụng.
`cmd/api/user_test.go`: kiểm tra route giống nhau giữa các môi trường, chỉ phụ thuộc DB bật/tắt,
prefix cũ không còn được đăng ký và health vẫn chạy.
`cmd/api/openapi_test.go`: kiểm tra route đăng ký khớp OpenAPI.

Integration test cần PostgreSQL riêng và biến opt-in:

```powershell
$env:TEST_DATABASE_URL = 'postgres://<user>:<password>@127.0.0.1:<test-port>/<test-db>?sslmode=disable'
go test -tags=integration ./... -count=1
```

Chỉ trỏ đến database thử nghiệm. Test tạo schema ngẫu nhiên, chạy migration thật,
đọc/ghi qua repository và HTTP handler, kiểm tra hai create đồng thời cùng email,
chạy down/up, rồi xóa schema do chính test tạo. Không đụng bảng ứng dụng ngoài schema đó.
Không có biến trên thì test PostgreSQL skip; PASS với skip không xác nhận SQL chạy đúng.

## 5. Migration và cách quay lại

`USER_REQUEST_TIMEOUT` điều chỉnh deadline context của request user, mặc định `5s`.
Giá trị phải là duration dương (ví dụ `2s`); timeout đọc/ghi HTTP vẫn áp dụng độc lập.
Nếu cấu hình mới gây lỗi vận hành, bỏ override hoặc đặt lại `5s`, khởi động lại API
và kiểm tra `/health`, `/ready`. Thay đổi này không cần migration dữ liệu.

Migration mới chỉ thêm bảng users; binary trước đó không sử dụng bảng này nên có
thể quay lại binary cũ và giữ nguyên bảng/dữ liệu. Sau khi quay lại, kiểm tra
`/health` và `/ready`. Production đăng ký cùng route `/api/v1/users` như dev/test khi bật DB.

`go run ./cmd/migrate down 1` xóa bảng users nếu đây là migration mới nhất: mất toàn bộ
user. Không dùng như rollback mặc định khi có dữ liệu cần giữ. Nếu cần xóa schema,
backup và kiểm tra khả năng khôi phục trước, đối chiếu migration status, chỉ thao
tác trên đúng database. Chưa có migration nào được chạy lên DB ứng dụng trong task này.

## 6. Kết quả kiểm chứng khi dựng mẫu

Đã chạy unit/HTTP tests, kiểm tra OpenAPI và integration suite với PostgreSQL 17
trong container tạm, credential ngẫu nhiên, port chỉ bind loopback. Integration
suite đạt và container đã được dừng/xóa. Không sử dụng DB ứng dụng để thử xóa dữ liệu.
Kết quả này kiểm chứng code/schema mẫu; chưa phải xác nhận deploy production.

Sau khi chuyển prefix, client dùng `/api/v1/users` thay cho `/users`. Nếu rollback
binary về bản dùng prefix cũ, client cũng cần đổi lại URL tương ứng.
`go test ./...`, `go vet ./...`, `go build ./...` và OpenAPI validator đạt.
`go test -race ./...` chưa chạy được do môi trường hiện tắt CGO; không coi unit
test thông thường là thay thế cho race detector.
