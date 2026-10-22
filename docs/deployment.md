# Release bằng Docker trên máy chủ

Bản phát hành: `0.5.0`. Một container phục vụ cả frontend và API Go, SQLite/file đính kèm nằm trong volume `oanh_data`. Container chạy UID/GID 65532, root filesystem chỉ đọc, healthcheck qua API và tự khởi động lại với `unless-stopped`.

## Build và chạy

Chạy tại thư mục repository:

```bash
OANH_REVISION=$(git rev-parse HEAD) docker compose build
docker compose up -d --wait --wait-timeout 90
docker compose ps
```

Image: `oanh-crm:0.5.0`. Dockerfile pin Node.js/Go theo phiên bản và digest, build frontend bằng pnpm theo lockfile, biên dịch Go static rồi chỉ chép binary và file tĩnh vào image scratch. Healthcheck kiểm tra được kết nối database qua `GET /api/v1/session` trước hoặc sau khi khởi tạo tài khoản.

Bản release cũng được gắn thêm tag image theo commit để đối chiếu/giữ bản cũ. Mã nguồn dùng remote `origin` tại [toanerrors/oanh-crm](https://github.com/toanerrors/oanh-crm). Sau mỗi task có thay đổi: validation → commit → push nhánh/tag liên quan → release Docker → healthcheck. Image hiện được lưu local trên server, chưa cấu hình registry. Không force-push hoặc thay đổi tag đã phát hành; không in thông tin xác thực trong output Git.

## Cloudflare Tunnel

Khi `cloudflared` chạy trên cùng máy chủ, thêm route ứng dụng công khai với:

| Trường | Giá trị |
|---|---|
| Subdomain | `oanh` |
| Domain | `toanerrors.com` |
| Type | `HTTP` |
| Service URL | `http://127.0.0.1:8080` |
| HTTP Host Header | Để mặc định, giữ hostname công khai |

Truy cập ứng dụng qua `https://oanh.toanerrors.com`. TLS kết thúc ở Cloudflare, tunnel chuyển tới HTTP loopback. Container bật cookie Secure cho phiên truy cập HTTPS; chỉ cổng host 127.0.0.1:8080 được publish, không mở 0.0.0.0:8080 ra mạng.

Thiết lập một lần chủ sở hữu/doanh nghiệp trước khi mở quyền truy cập rộng. Release không tạo tài khoản hay mật khẩu mặc định và không nạp dữ liệu giả lập. Dữ liệu phát sinh được giữ trong volume qua restart hoặc thay container; backup/restore thực hiện từ Cài đặt theo [runbook](backup-restore.md).

Nếu chạy `cloudflared` trong container khác, địa chỉ loopback trỏ vào container đó; cần cấu hình đường mạng tới origin trên host hoặc network Docker phù hợp trước khi dùng Service URL ở trên.

Tham chiếu: [Cloudflare Tunnel: thêm route](https://developers.cloudflare.com/cloudflare-one/networks/routes/add-routes/), [Docker multi-stage build](https://docs.docker.com/build/building/multi-stage/).

## Kiểm tra và quản lý

```bash
curl --fail http://127.0.0.1:8080/api/v1/session
docker compose exec -T app /healthcheck
docker compose logs --tail 50 app
docker compose restart app
docker compose stop
```

Dùng `docker compose up -d --wait` để bật lại container đã stop. Chính sách `unless-stopped` hoạt động khi Docker daemon khởi động; container bị dừng chủ động không tự bật lại.

Cập nhật: tạo backup từ ứng dụng, giữ image đang chạy theo commit, build phiên bản mới rồi `docker compose up -d --wait`. Không scale nhiều writer. Không dùng `docker compose down -v` vì thao tác đó xóa volume; không copy riêng file SQLite đang hoạt động. Rollback image chỉ áp dụng khi schema tương thích; khôi phục dữ liệu cần quy trình restore có xác nhận.

Domain/Cloudflare do người vận hành cấu hình sau release; kiểm tra origin local không thay cho xác minh đường HTTPS bên ngoài. Các giới hạn nghiệp vụ và nghiệm thu còn lại ở [trạng thái dự án](PROJECT_STATUS.md).

## Nâng schema lên phiên bản 2

Release 0.3.0 thêm nháp nghiệp vụ, tài khoản ngân hàng, chiều kho và bộ tài khoản tham chiếu. Trước khi khởi động image mới, tạo backup đã xác minh. Nếu chưa có phiên chủ sở hữu để gọi API backup, dừng container và sao chép **toàn bộ** `/data` của container đã dừng vào thư mục riêng (gồm pointer/dataset/attachments), kiểm tra SQLite trên bản sao chỉ đọc rồi đóng gói/checksum; không copy riêng `.db` đang chạy. Giữ nguyên volume `oanh_data` khi thay image. Migration chạy trong transaction, bảo toàn tên tài khoản/bút toán cũ; kho cũ được gắn Kho mặc định. Nhánh mã nguồn của đợt này: `codex/misa-voucher-workspace`, tag dự kiến `v0.3.0`.

Image 0.2.0 không đọc schema 2. Rollback binary cần image hỗ trợ schema 2; khôi phục bản dữ liệu trước nâng cấp phải theo runbook, không tự chép đè volume hoặc làm mất phát sinh sau release.


## Nâng schema 2 → 3 (BCTC 0.4.0)

Thêm bảng bản nháp BCTC và index; không thay tài khoản/bút toán/chính sách. Backup được xác minh trước khi khởi động image mới, giữ volume hiện tại. Schema3 đọc và nâng backup1/2 trên bản chuẩn bị. Image0.3.0 không đọc schema3; không rollback binary cũ trực tiếp trên volume đã nâng. Giữ bản dữ liệu trước release và image theo commit; phục hồi dữ liệu phải theo runbook và có ủy quyền đúng phạm vi.

### Migration4 của 0.5.0

Release thêm số dư đầu kỳ, không tạo/chỉnh số dư thật trong lúc deploy. Backup nguội toàn bộ dữ liệu trước release và kiểm tra schema/integrity/FK/đối soát trước khi nâng. Image0.4.x không đọc schema4; giữ image theo commit và bản dữ liệu trước release. Khôi phục hoặc thay dataset phải theo runbook, không hạ schema trực tiếp trên volume hiện tại.
