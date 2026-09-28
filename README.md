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

- `cmd/api`: điểm khởi chạy REST API trong tương lai.
- `cmd/worker`: điểm khởi chạy tác vụ nền trong tương lai.
- `internal/order`, `internal/product`: giữ tên module mẫu trong `architect.txt`.
- `handler.go`: tiếp nhận yêu cầu và trả phản hồi.
- `service.go`: xử lý nghiệp vụ.
- `repository.go`: truy cập dữ liệu.
- `order.go`, `product.go`: định nghĩa model của từng module.
- `internal/pkg`: cấu hình, kết nối dữ liệu và logging dùng nội bộ.
- `pkg`: dành cho code dùng chung có thể được dự án khác import.

Các file Go hiện chỉ khai báo package và hàm `main` rỗng. Chưa có HTTP server,
worker, model nghiệp vụ hoặc kết nối cơ sở dữ liệu. Các thư mục chưa có code
dùng `.gitkeep` để có thể được lưu trong Git.

## Kiểm tra

Yêu cầu Go 1.27 trở lên.

```sh
go build ./...
go test ./...
```

Module tạm đặt là `med_quiz/server`; cập nhật trong `go.mod` khi xác định đường
dẫn repository chính thức. Chưa có dependency bên ngoài nên chưa cần `go.sum`;
Go sẽ tạo file này khi cần lưu checksum cho dependency được thêm sau này.
