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
│   └── pkg/
│       ├── database/
│       ├── logger/
│       └── config/
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
- `internal/pkg`: cấu hình, kết nối dữ liệu và logging dùng nội bộ.
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

## Kiểm tra

Yêu cầu Go 1.27 trở lên.

```sh
go build ./...
go test ./...
```

Module tạm đặt là `med_quiz/server`; cập nhật trong `go.mod` khi xác định đường
dẫn repository chính thức. Chưa có dependency bên ngoài nên chưa cần `go.sum`;
Go sẽ tạo file này khi cần lưu checksum cho dependency được thêm sau này.
