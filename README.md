# Ứng dụng kế toán doanh nghiệp

Ứng dụng web dùng Go, SQLite và React + TypeScript + Ant Design. Dữ liệu được lưu bền vững tại máy chủ; giao diện mặc định kết nối API thật. Một pháp nhân, một tiến trình ghi, VND theo đơn vị đồng.

## Chạy bằng Docker

```bash
OANH_REVISION=$(git rev-parse HEAD) docker compose build
docker compose up -d --wait --wait-timeout 90
```

Origin trên server: `http://127.0.0.1:8080`. Cloudflare Tunnel ánh xạ hostname `oanh.toanerrors.com` tới origin này; truy cập công khai bằng HTTPS. SQLite/file đính kèm giữ trong volume `oanh_data`, không có tài khoản mặc định. Hướng dẫn cấu hình, cập nhật và backup ở [runbook release Docker](docs/deployment.md).

## Chạy ứng dụng

Yêu cầu Go 1.26.5, Node.js từ 22.18, pnpm 10.34.7 và ổ đĩa cục bộ. Backend hiện hỗ trợ hệ điều hành Unix; đã kiểm tra trên Linux amd64.

Tại `web/`:

```bash
corepack pnpm install --frozen-lockfile
corepack pnpm build
```

Tại `server/`:

```bash
go run ./cmd/server -data ../data -web ../web/dist
```

Mở [ứng dụng local](http://127.0.0.1:8080). Lần đầu, nhập doanh nghiệp, chế độ kế toán và tài khoản chủ sở hữu; không có mật khẩu mặc định. Tiếp theo vào **Cài đặt** để mở kỳ và xác nhận phiên bản chính sách, rồi thêm đối tượng/hàng hóa/nhân viên trước khi lập nghiệp vụ.

Trong phát triển, giữ backend ở cổng 8080 và chạy `corepack pnpm dev` tại `web/`. Vite chuyển `/api` tới backend. Chế độ dữ liệu mẫu chỉ bật bằng `VITE_DEMO_MODE=true corepack pnpm dev`; dữ liệu mẫu nằm trong bộ nhớ và mất khi tải lại.

Backend mặc định chỉ nghe localhost và từ chối Host ngoài localhost. Khi triển khai qua HTTPS cùng origin, cấu hình reverse proxy và chạy với `-secure-cookie=true`; bảo vệ cổng backend. Cấu hình Docker cùng Cloudflare Tunnel được mô tả trong runbook release; đường HTTPS công khai cần kiểm tra sau khi cấu hình domain.

## Chức năng hiện có

- Khởi tạo, đăng nhập, phân quyền chủ sở hữu/kế toán/chỉ xem; quản lý người dùng, kỳ và nhật ký.
- Nháp nhiều dòng, ghi sổ, đảo chứng từ thủ công, chống trùng nguồn và retry, kiểm tra version/epoch trong transaction.
- Mua/bán có VAT, điều kiện khấu trừ đầu vào; kho FIFO/bình quân sau lần nhập, không âm kho; lương/bảo hiểm/TNCN theo tham số đã xác nhận; tài sản/CCDC/242 đường thẳng; kết chuyển giá thành, TNDN tạm tính và kết quả kinh doanh.
- Chính sách có phiên bản, ngày hiệu lực và xác nhận; giữ cơ sở tính lịch sử. Không cài sẵn thuế suất pháp định từ workbook cũ.
- Sổ nhật ký, sổ cái, cân đối phát sinh, công nợ hai bên, chi tiết doanh thu/chi phí; lọc/phân trang/tổng và xuất CSV toàn bộ tập lọc từ server, in sổ, file đính kèm.
- Nhập CSV qua xem trước và ghi cả gói nháp; backup ZIP có kiểm tra, khôi phục qua staging, bản sao trước restore, đổi epoch và thu hồi phiên.

Đây chưa phải bộ chức năng bao phủ đủ 84 sheet hoặc hồ sơ kê khai/BCTC pháp định. Phạm vi còn thiếu và bằng chứng kiểm tra nằm trong [trạng thái dự án](docs/PROJECT_STATUS.md). Các tham số tính thuế/lương do doanh nghiệp xác nhận; cần nghiệm thu bộ chứng từ độc lập trước khi dùng số liệu thực tế.

## Kiểm tra

Tại `server/`:

```bash
go test -race ./internal/... ./cmd/server
go vet ./internal/... ./cmd/server
```

Tại `web/`:

```bash
corepack pnpm typecheck
corepack pnpm test
corepack pnpm exec playwright install chromium
corepack pnpm test:e2e
corepack pnpm test:e2e:live
```

E2E thật tạo dữ liệu giả lập trong thư mục tạm, dùng backend Go và SQLite thật, không dùng `data/` của doanh nghiệp. `test:e2e:live` tự build frontend; suite mẫu chạy Vite riêng. Không chạy hai suite trên cùng thư mục output đồng thời.

## Tài liệu

- [Trạng thái và giới hạn](docs/PROJECT_STATUS.md)
- [Kiến trúc](docs/architecture.md)
- [Quy tắc số liệu](docs/accounting-correctness.md)
- [Backup và khôi phục](docs/backup-restore.md)
- [Hợp đồng API](api/README.md)
- [Phân tích workbook](docs/reference/workbook-analysis.md), [đối chiếu 84 sheet](docs/reference/workbook-map.csv)
