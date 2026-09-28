# Med Quiz Server

Khung dự án Go theo `architect.txt`, được tạo trực tiếp tại thư mục `server`.

```text
server/
├── cmd/
│   ├── api/main.go
│   └── worker/main.go
├── internal/
│   ├── order/
│   │   ├── handler.go
│   │   ├── service.go
│   │   ├── repository.go
│   │   └── order.go
│   ├── product/
│   │   ├── handler.go
│   │   ├── service.go
│   │   ├── repository.go
│   │   └── product.go
│   └── platform/
│       ├── database/
│       ├── logger/
│       └── config/
│           ├── config.go
│           ├── config_test.go
│           └── profiles/
│               ├── dev.json
│               ├── test.json
│               └── prod.json
├── pkg/
└── go.mod
```

## Trách nhiệm các thành phần

- `cmd/api`: điểm khởi chạy API server bằng thư viện chuẩn `net/http`.
- `cmd/worker`: điểm khởi chạy tác vụ nền trong tương lai.
- `internal/order`, `internal/product`: giữ tên module mẫu trong `architect.txt`.
- `handler.go`: tiếp nhận yêu cầu và trả phản hồi.
- `service.go`: xử lý nghiệp vụ.
- `repository.go`: truy cập dữ liệu.
- `order.go`, `product.go`: định nghĩa model của từng module.
- `internal/platform`: cấu hình, kết nối dữ liệu và logging dùng nội bộ.
- `pkg`: dành cho code dùng chung có thể được dự án khác import.

API server có endpoint kiểm tra hoạt động `GET /health`, trả JSON
`{"status":"ok"}` với HTTP 200. Worker, model nghiệp vụ và kết nối cơ sở dữ liệu
chưa được triển khai. Các thư mục chưa có code
dùng `.gitkeep` để có thể được lưu trong Git.

## Chạy API server

```sh
go run ./cmd/api
```

Server mặc định lắng nghe tại `:8080`. Đặt biến môi trường `HTTP_ADDR` để đổi
địa chỉ, ví dụ trong PowerShell:

```powershell
$env:HTTP_ADDR = "127.0.0.1:8081"
go run ./cmd/api
```

Kiểm tra bằng `curl http://localhost:8080/health` (đổi cổng nếu đã cấu hình).
Route không tồn tại trả HTTP 404; phương thức không được hỗ trợ tại `/health`
trả HTTP 405. `HEAD /health` được `net/http` hỗ trợ cùng route GET.
Nhấn Ctrl+C để dừng server; các request đang chạy có tối đa 10 giây để hoàn tất.
Khi triển khai, kiểm tra `/health` sau khi khởi động; nếu thất bại, dừng bản mới
và chạy lại binary cùng cấu hình của bản trước.

## Cấu hình dev / test / prod

`internal/platform/config` nạp profile theo `APP_ENV` (`dev`, `test`, `prod`), mặc định
là `dev`. Các file `internal/platform/config/profiles/*.json` được nhúng vào binary;
không phụ thuộc thư mục chạy. Sau khi sửa profile cần build lại binary.
Biến môi trường không rỗng ghi đè giá trị trong profile; biến rỗng dùng giá trị
profile. Không tự động đọc file `.env`. `go test` không tự đặt `APP_ENV=test`.

| Biến | dev / prod | test |
| --- | --- | --- |
| `HTTP_ADDR` | `:8080` | `127.0.0.1:0` |
| `HTTP_READ_HEADER_TIMEOUT` | `5s` | `5s` |
| `HTTP_READ_TIMEOUT` | `15s` | `15s` |
| `HTTP_WRITE_TIMEOUT` | `15s` | `15s` |
| `HTTP_IDLE_TIMEOUT` | `60s` | `60s` |
| `HTTP_SHUTDOWN_TIMEOUT` | `10s` | `10s` |

Profile test chỉ lắng nghe loopback và để hệ điều hành chọn cổng trống; log khởi
động hiển thị cổng thực tế. Dev/prod giữ các mặc định HTTP hiện tại. Chỉ cấu hình
các thành phần đã triển khai; chưa có cấu hình DB hay worker.

Ví dụ PowerShell (đổi `prod` thành `dev` hoặc `test` khi cần):

```powershell
$env:APP_ENV = "prod"
$env:HTTP_ADDR = "127.0.0.1:8081"
$env:HTTP_SHUTDOWN_TIMEOUT = "20s"
go run ./cmd/api
```

`APP_ENV` không hợp lệ, địa chỉ sai định dạng `host:port`, cổng ngoài 0–65535,
hoặc timeout không dương sẽ làm API dừng trước khi mở listener. Timeout dùng
định dạng Go duration, ví dụ `500ms`, `5s`, `1m`. Không lưu secret trong profile
vì chúng được commit và nhúng vào binary; truyền secret qua môi trường hoặc hệ
thống quản lý secret khi bổ sung thành phần cần chúng.

Sau khi triển khai, gọi `GET /health` trên địa chỉ/cổng đã cấu hình và kiểm tra
HTTP 200 cùng `{"status":"ok"}`. Nếu thất bại, dừng bản mới và khôi phục binary
cùng biến môi trường trước đó. Để bỏ override trong PowerShell, dùng
`Remove-Item Env:HTTP_ADDR` (tương tự với các biến khác).

## Kiểm tra cấu hình và build

Yêu cầu Go 1.27 trở lên.

```sh
go build ./...
go test ./...
```

Module tạm đặt là `server`; cập nhật trong `go.mod` khi xác định đường
dẫn repository chính thức. Chưa có dependency bên ngoài nên chưa cần `go.sum`;
Go sẽ tạo file này khi cần lưu checksum cho dependency được thêm sau này.
