# Med Quiz Server

## CORS tùy chọn

CORS mặc định tắt, giữ nguyên routing (kể cả `OPTIONS /health` trả 405).
Cấu hình dùng env override profile JSON; giá trị env trống dùng mặc định profile.
Danh sách phân cách bằng dấu phẩy, bỏ khoảng trắng quanh từng phần tử.

| Biến | Mặc định | Quy tắc khi bật |
| --- | --- | --- |
| `CORS_ENABLED` | `false` | boolean |
| `CORS_ALLOWED_ORIGINS` | trống | bắt buộc origin HTTP/HTTPS cụ thể, ví dụ `http://localhost:3000,https://example.com` |
| `CORS_ALLOWED_METHODS` | `GET,HEAD` | danh sách HTTP method, phân biệt hoa/thường |
| `CORS_ALLOWED_HEADERS` | trống | header request được phép trong preflight, so sánh không phân biệt hoa/thường |
| `CORS_EXPOSED_HEADERS` | trống | header response cho JavaScript đọc thêm |
| `CORS_ALLOW_CREDENTIALS` | `false` | bật có chủ đích cho origin tin cậy |
| `CORS_MAX_AGE` | `10m` | duration không âm, nguyên giây; `0s` để không cache preflight |

Origin không có path (kể cả `/` cuối), query, fragment hay userinfo. Không hỗ trợ
wildcard, regex hoặc `null`; port nếu có phải từ 1 đến 65535. Danh sách method/header
không hỗ trợ wildcard. Khi bật, cấu hình sai làm startup thất bại với tên biến lỗi.
Khi tắt, các thiết lập CORS phụ được bỏ qua. Origin request phải khớp chính xác allowlist.

Ví dụ chạy local trong PowerShell:

```powershell
$env:CORS_ENABLED = 'true'
$env:CORS_ALLOWED_ORIGINS = 'http://localhost:3000'
$env:CORS_ALLOWED_HEADERS = 'Authorization,Content-Type'
go run ./cmd/api
```

Để dùng cookie/credentials, chủ động đặt `CORS_ALLOW_CREDENTIALS=true` và phía trình
duyệt dùng `fetch(url, { credentials: 'include' })`; chính sách cookie vẫn áp dụng.
CORS không thay authentication, authorization hay bảo vệ CSRF và không chặn client
ngoài trình duyệt. Actual request từ origin bị từ chối vẫn đi qua HTTP bình thường,
nhưng không nhận header cấp quyền CORS. Response lỗi từ origin được phép cũng có CORS.

Preflight là OPTIONS có cả Origin và Access-Control-Request-Method: hợp lệ trả 204
không body, origin/method/header bị từ chối trả 403 không cấp quyền CORS. OPTIONS khác
vẫn theo routing. Middleware CORS bao ngoài recovery; nếu thêm limiter sau này, đặt
limiter bên trong CORS để preflight được xử lý trước. Vary phân biệt origin và các
thuộc tính preflight, đồng thời giữ giá trị Vary hiện có.

Smoke HTTP sau khi bật, từ terminal khác:

```powershell
curl.exe -i http://localhost:8080/health -H "Origin: http://localhost:3000"
curl.exe -i -X OPTIONS http://localhost:8080/health -H "Origin: http://localhost:3000" -H "Access-Control-Request-Method: GET" -H "Access-Control-Request-Headers: Authorization"
```

Kỳ vọng 200 với body health và allow-origin cụ thể; preflight 204 không body. Sau khi
đặt `CORS_ENABLED=false` và khởi động lại, chạy lại: GET vẫn 200, OPTIONS trở lại 405,
không có header cấp quyền CORS. Với Compose, sửa `.env` rồi tạo lại service để nhận env
mới. Mitigation: tắt CORS hoặc khôi phục cấu hình trước và kiểm tra `/health` lại.
`go test ./cmd/api -run TestCORSSmoke -v` kiểm tra actual/preflight qua socket khi
bật/tắt; kiểm tra này không thay thử cross-origin bằng trình duyệt thật.

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
`{"status":"ok"}` với HTTP 200. Worker và model nghiệp vụ
chưa được triển khai. PostgreSQL là dependency tùy chọn. Các thư mục chưa có code
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

## HTTP client outbound

`internal/platform/httpclient.New(configuration.HTTPClient)` trả `*http.Client` với
transport riêng theo các thiết lập chuẩn của `net/http`, không sửa hoặc dùng chung
`http.DefaultClient`/`http.DefaultTransport`. Không thêm dependency hay logging.
Hiện chưa có module gọi outbound nên startup/health không tạo client hoặc gọi mạng mẫu.
Khi có caller thực tế, khởi tạo một lần ở entrypoint và truyền cùng con trỏ vào
constructor của module; client và pool có thể dùng đồng thời từ nhiều goroutine.
Không sửa client/transport sau khi bắt đầu sử dụng.

Các profile dev/test/prod dùng cùng mặc định sau. Env không rỗng ghi đè profile;
`.env.example` và Compose hỗ trợ tất cả các biến này.

| Biến | Mặc định | Ý nghĩa |
| --- | --- | --- |
| `HTTP_CLIENT_TIMEOUT` | `30s` | Toàn bộ request, gồm kết nối, redirect và đọc body |
| `HTTP_CLIENT_CONNECT_TIMEOUT` | `5s` | Thiết lập kết nối TCP, gồm phân giải tên |
| `HTTP_CLIENT_TLS_HANDSHAKE_TIMEOUT` | `5s` | Bắt tay TLS |
| `HTTP_CLIENT_RESPONSE_HEADER_TIMEOUT` | `10s` | Chờ header sau khi gửi xong request, không gồm đọc body |
| `HTTP_CLIENT_IDLE_CONN_TIMEOUT` | `90s` | Thời gian giữ kết nối rảnh trong pool |
| `HTTP_CLIENT_MAX_IDLE_CONNS` | `100` | Tổng số kết nối rảnh tối đa |
| `HTTP_CLIENT_MAX_IDLE_CONNS_PER_HOST` | `10` | Kết nối rảnh tối đa mỗi host |
| `HTTP_CLIENT_MAX_CONNS_PER_HOST` | `50` | Kết nối tối đa mỗi host, gồm đang kết nối, hoạt động và rảnh |

Duration và số connection phải dương; giới hạn idle mỗi host không được vượt tổng
idle hoặc tổng connection mỗi host. Config loader và constructor đều kiểm tra giá
trị trước khi dùng. Không yêu cầu timeout tổng bằng tổng timeout thành phần: deadline
context, timeout tổng hoặc timeout của giai đoạn hiện tại, cái nào đến trước sẽ hủy
request. Chờ slot trong pool và đọc body vẫn chịu timeout tổng/context; idle timeout
chỉ quản lý tài nguyên giữa các request. TCP keep-alive là `30s`, chờ `100-continue`
là `1s`, cho phép thương lượng HTTP/2 như transport chuẩn.

TLS giữ xác minh certificate/hostname mặc định, không gắn credential hoặc cookie jar.
Proxy kế thừa `http.ProxyFromEnvironment` (`HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`);
redirect theo mặc định `http.Client`, dừng sau 10 request liên tiếp. Không thêm vòng
retry/backoff và không retry theo status. Transport chuẩn vẫn có thể tự gửi lại
request idempotent trong một số lỗi kết nối đã tái sử dụng; đây là hành vi `net/http`.
HTTP 4xx/5xx được trả nguyên vẹn, caller tự quyết định nghiệp vụ.

Ví dụ vòng đời ở entrypoint (imports `server/internal/platform/config` và
`server/internal/platform/httpclient`), truyền `outboundClient` vào module cần dùng
và giữ nó cho đến khi các module đã dừng:

```go
configuration, err := config.Load()
if err != nil {
    return err
}
outboundClient, err := httpclient.New(configuration.HTTPClient)
if err != nil {
    return err
}
defer outboundClient.CloseIdleConnections()
```

Ví dụ hàm của caller dùng client đã được truyền vào (imports `context`, `fmt`, `io`,
`net/http`, `time`); `endpoint` phải do ứng dụng kiểm soát hoặc đã kiểm tra đích đến
tại trust boundary nếu nhận từ người dùng:

```go
func fetch(ctx context.Context, client *http.Client, endpoint string) ([]byte, error) {
    requestContext, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()
    request, err := http.NewRequestWithContext(requestContext, http.MethodGet, endpoint, nil)
    if err != nil {
        return nil, err
    }
    response, err := client.Do(request)
    if err != nil {
        return nil, err
    }
    defer response.Body.Close()
    if response.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("unexpected HTTP status: %d", response.StatusCode)
    }
    const maxBodyBytes = 1 << 20
    body, err := io.ReadAll(io.LimitReader(response.Body, maxBodyBytes+1))
    if err != nil {
        return nil, err
    }
    if len(body) > maxBodyBytes {
        return nil, fmt.Errorf("response body exceeds limit")
    }
    return body, nil
}
```

Caller luôn đóng body và chọn giới hạn đọc theo nghiệp vụ. Đọc đến EOF rồi đóng
giúp tái sử dụng kết nối; đóng sớm khi status/body không phù hợp có thể bỏ kết nối,
không drain body không giới hạn. `CloseIdleConnections` giải phóng kết nối rảnh,
không hủy request đang chạy; dừng caller/hủy context trước khi kết thúc vòng đời.
Không log trực tiếp error của `net/http` vì có thể chứa URL; không ghi token,
header, query, body hoặc URL có credential.

Kiểm tra local, không gọi Internet: `go test ./internal/platform/config ./internal/platform/httpclient`,
`go test ./...`, `go vet ./...`, `go build ./...`. Sau triển khai, kiểm tra `/health`
trả HTTP 200 và `{"status":"ok"}`. Nếu lỗi, khôi phục binary/image và env bản trước;
có thể bỏ các override `HTTP_CLIENT_*` để trở lại mặc định profile.

## Error handling

API trả lỗi JSON thống nhất, ví dụ:

```json
{"error":{"code":"not_found","message":"Resource not found"}}
```

- Route không tồn tại: HTTP 404, code `not_found`.
- Method không hỗ trợ tại `/health`: HTTP 405, code `method_not_allowed`, header `Allow: GET, HEAD`.
- Panic trong handler trước khi gửi response: HTTP 500, code `internal_error`.
- `HEAD` trả status/header tương ứng và không có body.

`cmd/api/handler.go` quản lý response và recovery bằng `net/http`. Panic được ghi qua
`slog`; không ghi giá trị panic, URL, header hoặc body request để tránh lộ dữ liệu.
Nếu response đã bắt đầu gửi, recovery hủy request bằng `http.ErrAbortHandler`;
không nối JSON lỗi vào response dở dang. Panic chủ động bằng `http.ErrAbortHandler`
được giữ nguyên. Recovery chỉ áp dụng cho goroutine đang xử lý HTTP request.
Lỗi ghi response được log, không thử ghi lại khi kết nối có thể đã đóng.
Lỗi listen/serve/shutdown được bọc bằng `%w` để giữ nguyên nguyên nhân cho `errors.Is/As`.
Các module nghiệp vụ và worker hiện còn trống, chưa có xử lý lỗi nghiệp vụ.

Kiểm tra bằng `go test ./...` và `go vet ./...`. Sau triển khai, kiểm tra `/health`
trả 200 và `{"status":"ok"}`, route không tồn tại trả JSON 404, `POST /health` trả
JSON 405. Nếu thất bại, khôi phục binary/image và cấu hình bản trước rồi kiểm tra lại.

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
dẫn repository chính thức. Dependency được khóa trong `go.mod` và `go.sum`.

## CI trên GitHub Actions

Xem phần PostgreSQL cuối tài liệu để chạy integration test riêng.

Skeleton lưu workflow mẫu tại `templates/.github/workflows/ci.yml`, nên CI
chưa tự chạy khi đưa repository lên GitHub. Các file trong `templates/` giữ
đường dẫn tương ứng với vị trí sử dụng tính từ thư mục gốc dự án.

Để bật CI cho dự án dùng GitHub Actions, copy file mẫu từ thư mục gốc bằng
PowerShell (nếu đã có workflow cùng tên, kiểm tra và hợp nhất trước):

```powershell
New-Item -ItemType Directory -Force .github/workflows | Out-Null
Copy-Item templates/.github/workflows/ci.yml .github/workflows/ci.yml
```

Commit và push `.github/workflows/ci.yml` để kích hoạt. Khi được kích hoạt,
workflow chạy trên mọi push và khi mở, mở lại hoặc cập nhật pull request,
không giới hạn tên nhánh. Nếu dùng nền tảng CI khác, dùng các lệnh kiểm tra
local bên dưới để xây dựng pipeline tương ứng.

Workflow `Go CI`, job/check `Go checks` chạy trên Ubuntu: kiểm tra `gofmt`
(chỉ đọc, thất bại nếu cần format), `go vet ./...`, `go test -race ./...` và
`go build ./...`. Phiên bản Go lấy từ `go.mod`. Job có timeout 15 phút;
lượt chạy mới hủy lượt cũ cùng PR hoặc cùng ref. Lỗi ở một bước làm job thất
bại và các bước sau không chạy; không tự sửa hoặc commit code.

Workflow chỉ cấp `contents: read`, không lưu credential sau checkout, không
dùng secret hay sự kiện `pull_request_target`. Unit test hiện chạy độc lập,
không cần DB hoặc dịch vụ ngoài. Khi thêm integration test cần dịch vụ,
tách bằng build tag `integration` và chạy riêng bằng
`go test -tags=integration ./...` với môi trường phù hợp; không đưa chúng vào
unit test mặc định.

Chưa có `go.sum` thì cache tắt. Khi thêm dependency, chạy `go mod tidy` ở
local và commit cả `go.mod` lẫn `go.sum`; cache tự bật theo checksum của
`go.sum`. CI dùng `GOFLAGS=-mod=readonly` để báo lỗi khi thiếu thông tin
dependency/checksum, thay vì tự cập nhật module để vượt kiểm tra.

Chạy tương đương từ thư mục gốc bằng Bash (Linux, WSL hoặc Git Bash):

```bash
set -euo pipefail
export CGO_ENABLED=1
export GOFLAGS=-mod=readonly
unformatted=$(gofmt -l .)
if [[ -n "$unformatted" ]]; then
  printf 'Go files need formatting:\n%s\n' "$unformatted"
  exit 1
fi
go vet ./...
go test -race ./...
go build ./...
```

Cần Go theo `go.mod` và C compiler tương thích với Go trên hệ điều hành đang
dùng để chạy race detector (Ubuntu runner có sẵn GCC). Nếu Windows thiếu
compiler, dùng môi trường Linux/WSL có Go và GCC; `go test ./...` không thay
thế được kiểm tra race.

Sau khi có workflow run trên GitHub, quản trị viên chọn check `Go checks`
trong branch protection/ruleset của nhánh cần bảo vệ. Task này không thay đổi
settings repository. Kết quả lệnh local không đồng nghĩa workflow đã chạy
thành công trên GitHub.

Tham khảo cấu hình [setup-go](https://github.com/actions/setup-go) và
[checkout](https://github.com/actions/checkout).

## PostgreSQL và migration

DB mặc định tắt (`DB_ENABLED=false`), không cần credential và không mở kết nối.
Khi bật, API mở một pool, ping trước khi listen và đóng pool sau HTTP shutdown;
cấu hình hoặc kết nối lỗi làm startup thất bại. API không tự chạy migration.
`GET /health` vẫn là liveness. `GET /ready` trả `200 {"status":"ok"}` khi DB tắt
hoặc ping thành công; trả `503 {"status":"unavailable"}` khi ping thất bại.
HEAD không có body; phương thức khác trả 405. Driver error không xuất hiện trong
log startup, CLI migration hoặc response readiness.

| Biến môi trường | Mặc định | Ràng buộc khi DB bật |
| --- | --- | --- |
| `DB_ENABLED` | `false` | boolean |
| `DB_PROVIDER` | `postgres` | chỉ hỗ trợ postgres |
| `DATABASE_URL` | trống | URL postgres/postgresql có host, user, tên database |
| `DB_MAX_OPEN_CONNS` | `10` | > 0 |
| `DB_MAX_IDLE_CONNS` | `2` | 0 đến max open |
| `DB_CONN_MAX_LIFETIME` | `30m` | > 0 |
| `DB_CONN_MAX_IDLE_TIME` | `5m` | > 0 và không vượt lifetime |
| `DB_CONNECT_TIMEOUT` | `5s` | > 0; timeout driver kết nối |
| `DB_PING_TIMEOUT` | `2s` | > 0; giới hạn toàn bộ ping, kể cả chờ pool |

Credential chỉ lấy từ môi trường. Không ghi URL thật vào source, profile,
command-line argument hoặc log; không dump toàn bộ config. `.env` dành cho local
không được commit; Compose đọc file này, còn `go run` đọc môi trường process.
Nếu URL không có `sslmode`, module đặt `verify-full`. Production dùng
`sslmode=verify-full&sslrootcert=/path/to/ca.crt`, hostname khớp certificate và
mount CA vào container nếu cần. `sslmode=disable` chỉ dùng local cô lập; không
dùng cho production. URL-encode username/password chứa ký tự đặc biệt.

Module dùng `database/sql`, [pgx](https://github.com/jackc/pgx) v5.11.0 và
[golang-migrate](https://github.com/golang-migrate/migrate) v4.20.1, khóa trong
`go.mod`/`go.sum`, tương thích toolchain Go 1.27 của repo. Chọn adapter pgx của
migrate để dùng chung driver. Đổi provider có thể cần driver, SQL/schema và
migration mới; đổi `DB_PROVIDER` không chuyển dữ liệu hay dialect.

### Local với Compose

Copy `.env.example` thành `.env`, tự cung cấp `POSTGRES_USER`,
`POSTGRES_PASSWORD`, `POSTGRES_DB`. Không để trống password. Các biến này dùng
để khởi tạo volume mới; đổi env không đổi credential trong volume đã có.

```sh
docker compose --profile database up -d --wait postgres
```

Đặt `DB_ENABLED=true` và `DATABASE_URL` trong `.env` theo mẫu
`postgres://<user>:<password>@postgres:5432/<database>?sslmode=disable`.
Để chạy API trên host, đặt các biến tương ứng vào môi trường process và dùng
host `127.0.0.1`, cổng `POSTGRES_PORT` (mặc định 5432).

```sh
docker compose up -d --build api
curl http://localhost:8080/health
curl http://localhost:8080/ready
```

Service postgres chỉ chạy khi bật profile hoặc gọi đích danh; API không có
`depends_on` bắt buộc. Volume `postgres_data` giữ dữ liệu. Không dùng
`docker compose down -v` trên dữ liệu cần giữ. Để quay về chạy không DB, đặt
`DB_ENABLED=false` và tạo lại service API; dữ liệu PostgreSQL vẫn giữ nguyên.

### Lệnh migration riêng

Chạy từ root repo. `create` không cần DB. Các lệnh DB dùng cùng env với API,
bắt buộc `DB_ENABLED=true`. Mặc định đọc `migrations/postgres`; có thể đặt
`MIGRATIONS_DIR` để thay đường dẫn. Chưa có migration nghiệp vụ.

```sh
go run ./cmd/migrate create add_example
go run ./cmd/migrate status
go run ./cmd/migrate up
go run ./cmd/migrate down 1
```

`create` tạo cặp timestamp `.up.sql`/`.down.sql` rỗng; điền và review cả hai
trước khi chạy. Mỗi version phải duy nhất; không sửa migration đã áp dụng.
`down` bắt buộc số bước dương, không có rollback toàn bộ ngầm định. Trong
container đã build, ví dụ: `docker compose run --rm --entrypoint /migrate api status`.
Migration SQL được COPY vào image; rebuild sau khi thêm SQL. Chỉ chạy `up`
bằng bước deploy có kiểm soát trước khi chuyển traffic, rồi kiểm tra `/ready`.

Migrate lưu version/dirty trong `schema_migrations`, khóa bằng PostgreSQL
advisory lock để các runner cùng DB/schema không chạy đồng thời. Sau khởi tạo,
runner có lock timeout 10s, migration statement timeout 1 phút; bước khởi tạo
metadata có thể chờ runner khác nhả lock. `status` có thể khởi tạo bảng metadata nếu chưa
có. Bao SQL nhiều bước trong `BEGIN`/`COMMIT` khi phù hợp; không giả định mọi
DDL đều transactional. Xem [hướng dẫn lỗi dirty](https://github.com/golang-migrate/migrate/blob/master/GETTING_STARTED.md).

Nếu migration lỗi, dừng runner, xem `status` và đối chiếu schema thực tế với SQL;
driver details bị ẩn để tránh lộ secret, cần xem diagnostics trên DB bằng quyền
vận hành phù hợp. Khôi phục backup hoặc sửa schema có kiểm soát. Chỉ sau khi xác
nhận schema khớp version mới chạy `go run ./cmd/migrate force VERSION` (dùng `-1`
cho chưa có migration). `force` chỉ đổi metadata, không chạy SQL và không sửa
dữ liệu. Không tự force, retry hoặc rollback khi chưa biết trạng thái dữ liệu.

### Backup, khôi phục và rollback

Trước migration phá hủy dữ liệu: dừng ghi hoặc lên kế hoạch nhất quán, tạo
backup bằng `pg_dump --format=custom --file=backup.dump` với credential từ
`PGHOST`, `PGPORT`, `PGUSER`, `PGDATABASE`, `PGPASSFILE` được cấp an toàn; bảo vệ
backup như dữ liệu nhạy cảm. Thử `pg_restore --dbname=<database-khoi-phuc-rieng> backup.dump`
và kiểm tra dữ liệu trước khi cho phép migration. Không thử restore
đè lên DB hiện có. Với production, ưu tiên snapshot/PITR đã kiểm chứng và migration
expand/contract để binary cũ và mới cùng hoạt động trong thời gian chuyển tiếp.

Nếu deploy lỗi, rollback image chỉ khi schema còn tương thích. Rollback binary
không tự hoàn tác schema; `down` có thể mất dữ liệu và không thay backup. Khi cần
restore, dừng ghi, restore sang DB mới, xác minh schema/dữ liệu, chuyển connection
secret có kiểm soát và smoke `/health`, `/ready` trước khi mở traffic. Task này
không deploy hay chạy migration trên DB thật.

### Integration test tách biệt

Unit test: `go test ./...`; kiểm tra tĩnh/build: `go vet ./...`, `go build ./...`.
Integration yêu cầu opt-in bằng build tag và `TEST_DATABASE_URL` trỏ tới PostgreSQL
riêng, có quyền tạo schema. Test chỉ tạo/xóa schema tên ngẫu nhiên do chính test
tạo, dùng migration thử, không rollback schema ứng dụng. Không trỏ biến này tới
DB production hoặc DB local chứa dữ liệu cần giữ.

```powershell
$env:TEST_DATABASE_URL = 'postgres://<user>:<password>@127.0.0.1:<test-port>/<test-db>?sslmode=disable'
go test -tags=integration ./... -count=1
```

Test kiểm chứng query, migration up/status/down, HTTP health/readiness qua socket,
readiness 503 khi pool không thể đáp ứng, timeout và phục hồi. Unit test còn dùng
TCP listener không phản hồi để kiểm tra startup thất bại có giới hạn thời gian.
Nếu không đặt `TEST_DATABASE_URL`, integration test báo skip; đây không phải bằng
chứng PostgreSQL đã được kiểm chứng.
