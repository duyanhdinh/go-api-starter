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
├── Dockerfile
├── compose.yaml
├── .dockerignore
├── .env.example
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

## Logging

`internal/platform/logger` dùng `log/slog`, ghi vào stdout, không ghi file.
Profile `dev` và `test` dùng TextHandler với mức DEBUG; `prod` dùng JSONHandler
với mức INFO. Mỗi bản ghi production nằm trên một dòng JSON để Docker thu thập
và có thể chuyển tiếp tới Loki; dự án chưa cấu hình Loki.

Đặt `LOG_LEVEL=DEBUG`, `INFO`, `WARN` hoặc `ERROR` để ghi đè mức log.
Tên mức không phân biệt hoa/thường; hỗ trợ offset của slog như `INFO+2`.
Giá trị rỗng dùng profile, giá trị không hợp lệ làm API dừng trước khi mở listener.
Compose truyền `LOG_LEVEL` từ terminal hoặc `.env` vào container.

Khởi tạo logger ở entrypoint và truyền `*slog.Logger` vào module cần dùng:

```go
applicationLogger := logger.New(configuration.Environment, configuration.LogLevel)
orderLogger := applicationLogger.With("module", "order")
orderLogger.Info("Order created", "order_id", orderID)
```

Import package bằng `server/internal/platform/logger`. `With` tạo logger con,
giữ fields của logger cha mà không thay đổi logger cha. API dùng fields `service`,
`env`, `addr`; lỗi nội bộ HTTP cũng được chuyển qua cùng handler ở mức ERROR.
Lỗi cấu hình dùng logger khởi động ở mức INFO và định dạng theo `APP_ENV`.

Chỉ truyền message và fields đã xác định là an toàn. Không log password, token,
cookie, authorization header, secret, toàn bộ request/header/config hoặc dữ liệu
người dùng chưa kiểm soát. Kiểm tra cả nội dung error trước khi log. Logger không
tự che dữ liệu nhạy cảm trong message, field hay object; module gọi chịu trách
nhiệm chọn dữ liệu được phép ghi.

Sau triển khai, kiểm tra `/health` trả HTTP 200 và log startup xuất hiện trên stdout
(JSON khi `APP_ENV=prod`), ví dụ `docker compose logs api`. Nếu lỗi, khôi phục image
và env trước đó rồi tạo lại container như hướng dẫn rollback bên dưới.

## Chạy bằng Docker

Yêu cầu Docker với Linux containers và Docker Compose. Dockerfile build Go trong
một giai đoạn riêng rồi đóng gói binary và CA certificates vào image `scratch`,
chạy bằng UID/GID `65532:65532`. Image không chứa shell hay file `.env`.

Tạo file cấu hình local từ mẫu (chỉ thực hiện nếu chưa có `.env`):

```powershell
Copy-Item .env.example .env
docker compose up --build -d
curl.exe --fail http://localhost:8080/health
docker compose logs api
docker compose down
```

Sửa `APP_ENV` trong `.env` thành `dev`, `test` hoặc `prod`. Compose đọc file này
và truyền các biến đã khai báo trong `environment` vào container; ứng dụng vẫn
đọc bằng `os.Getenv`. Biến trong terminal ưu tiên hơn giá trị trong file `.env`.
Các timeout để trống sẽ dùng profile JSON. `HTTP_PORT` chỉ đổi cổng trên máy host;
Compose cố định `HTTP_ADDR=:8080` bên trong container, kể cả profile test, để
port mapping hoạt động. Cổng host chỉ bind `127.0.0.1` cho local.

`STOP_GRACE_PERIOD` là thời gian Docker chờ trước khi buộc dừng container; đặt
lớn hơn `HTTP_SHUTDOWN_TIMEOUT` (mặc định ứng dụng là `10s`). Sau khi đổi env,
chạy lại `docker compose up -d --no-build` để tạo lại container với cấu hình mới;
`docker compose restart` không cập nhật env.

Có thể tạo `.env.test` hoặc `.env.prod` từ mẫu, rồi dùng cùng image đã build:

```powershell
docker compose --env-file .env.test up -d --no-build
```

Đặt `APP_ENV=test` trong `.env.test`; tên file không tự chọn profile. Các file env
cá nhân được loại khỏi Git và Docker build context. `.env.example` chỉ chứa giá
trị mẫu, không chứa secret. Khi thêm secret mới, cần khai báo biến tương ứng
trong Compose và đọc/kiểm tra ở ứng dụng; không thêm secret vào JSON, Dockerfile
hay build args. Hiện ứng dụng chưa có thành phần cần secret.

Khi deploy, dùng cùng image và cấp biến môi trường qua nền tảng triển khai.
Ví dụ chạy trực tiếp image đã build với file env do hệ thống deploy cung cấp:

```sh
docker run -d --name med-quiz-api --env-file .env.prod -e HTTP_ADDR=:8080 -p 127.0.0.1:8080:8080 --stop-timeout 30 med-quiz-server:local
```

Compose hiện phục vụ local; cấu hình ingress/cổng public ở nền tảng deploy.
Smoke test phải trả HTTP 200 và `{"status":"ok"}` từ `/health`. Khi phát hành,
gắn tag riêng cho từng bản bằng `IMAGE_TAG`, giữ lại image và env bản trước.
Nếu smoke test thất bại, khôi phục tag/env cũ rồi chạy
`docker compose up -d --no-build`, sau đó kiểm tra `/health` lại.

Tham khảo [cách Docker Compose truyền biến môi trường](https://docs.docker.com/compose/how-tos/environment-variables/set-environment-variables/).

## Kiểm tra cấu hình và build

Yêu cầu Go 1.27 trở lên.

```sh
go build ./...
go test ./...
```

Module tạm đặt là `server`; cập nhật trong `go.mod` khi xác định đường
dẫn repository chính thức. Chưa có dependency bên ngoài nên chưa cần `go.sum`;
Go sẽ tạo file này khi cần lưu checksum cho dependency được thêm sau này.
