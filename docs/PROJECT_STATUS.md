# Trạng thái dự án

Cập nhật: 21/10/2026. Tài liệu ghi chức năng có mã nguồn và bằng chứng kiểm tra, không thay cho nghiệm thu kế toán.

## Phạm vi đã chốt

- Quy trình sau mỗi task có thay đổi: validation → commit → release Docker trên server hiện tại → healthcheck. Người dùng đã ủy quyền mặc định, không cần xác nhận lại cho commit/release thông thường; quy tắc được lưu trong `AGENTS.md`.
- Người dùng yêu cầu ứng dụng kế toán bao phủ workbook, UI/UX hoàn chỉnh và nghiệp vụ tự động; chính sách thay đổi trong Cài đặt.
- Go + SQLite; React + TypeScript strict + Vite SPA, Ant Design Form/Table, React Router, TanStack Query; pnpm. Frontend và driver Go `modernc.org/sqlite` đã được chấp thuận cài đặt.
- Một pháp nhân, một máy chủ ghi trên ổ cục bộ, VND theo đơn vị đồng. Tiền và decimal qua API là chuỗi; int64 và math/big ở backend, chuỗi/BigInt ở frontend.

## Release Docker 0.2.0

- Người dùng yêu cầu commit và release trên server hiện tại bằng Docker; domain `oanh.toanerrors.com` sẽ được cấu hình riêng ở Cloudflare Tunnel.
- Bổ sung Dockerfile nhiều stage, Compose một container chạy user thường/read-only, healthcheck, restart policy và volume `oanh_data`; origin publish ở `127.0.0.1:8080`, bật cookie Secure cho HTTPS qua tunnel.
- Đã build image `oanh-crm:0.2.0`, chạy container `oanh-app-1` healthy ở `127.0.0.1:8080`; UID 65532, root filesystem read-only, volume `oanh_data`. Restart giữ nguyên active dataset; API/database, HTML/SPA và màn hình khởi tạo không lỗi JavaScript đã kiểm tra. Host/Origin của domain HTTPS qua proxy được chấp nhận, request cross-origin bị chặn.
- Build Docker chạy 15 test frontend và production build thành công; healthcheck Go đã qua compile/vet. Không tạo tài khoản hay dữ liệu mẫu trên volume release. Commit/tag phát hành local trên nhánh `release/v0.2.0`; chưa cấu hình Git remote hoặc route Cloudflare, chưa kiểm tra HTTPS công khai.
- Runbook: [deployment.md](deployment.md).

## Đã triển khai

| Phần | Chức năng có mã nguồn |
|---|---|
| Nền tảng | Go HTTP/API và file tĩnh cùng origin; SQLite STRICT, migration/checksum, WAL/FULL, per-connection FK/timeout, một writer và read pool 3; khóa tiến trình Unix |
| Truy cập | Khởi tạo chủ sở hữu, cookie phiên, hash mật khẩu, khóa đăng nhập tạm thời, owner/accountant/viewer, quản lý người dùng và thu hồi phiên, nhật ký |
| Cấu hình | Phiên bản tăng theo ngày hiệu lực; khai báo chế độ kế toán, VAT, FIFO/bình quân, scale, làm tròn, ngày/giờ công, giảm trừ, biểu TNCN, tỷ lệ BH/TNDN, đường thẳng và phần dư tháng cuối; xác nhận trước dùng |
| Danh mục/kỳ | Đối tượng, tài khoản, hàng/dịch vụ, nhân viên; giữ mã/lịch sử, ngừng sử dụng; mở/khóa kỳ; mở lại có đảo kết chuyển liên kết |
| Sổ kế toán | Nháp nhiều dòng, version, ghi sổ/cấp số/audit/nguồn/idempotency/epoch cùng transaction; chứng từ đã ghi sổ bất biến; đảo chứng từ thủ công |
| Mua/bán/kho | VAT mua/bán theo cấu hình, xác nhận khấu trừ đầu vào từng dòng, FIFO hoặc bình quân sau mỗi lần nhập, chống âm kho/lùi ngày/trộn phương pháp và scale khi còn tồn |
| Lương | Ngày công tổng hợp, phụ cấp, làm thêm, bảo hiểm theo cơ sở đã nhập, giảm trừ và TNCN lũy tiến theo cấu hình; lưu cơ sở lương/BH/người phụ thuộc lịch sử |
| Tài sản/242 | Ghi tăng TSCĐ hoặc khoản CCDC/242; đường thẳng theo tháng, kiểm tra thứ tự kỳ, phần dư tháng cuối |
| Giá thành/thuế/kết chuyển | Kết chuyển giá thành không vượt chi phí dở dang 154; TNDN tạm tính từ doanh thu/chi phí và điều chỉnh đã xác nhận, chặn khoảng chồng lấn; kết chuyển doanh thu/chi phí qua 911/421 rồi khóa kỳ |
| Tra cứu/báo cáo | Dashboard server, chứng từ theo kỳ/lọc/sort/phân trang/tổng toàn bộ tập lọc; NKC, sổ cái, cân đối phát sinh, dư Nợ/Có theo đối tượng, chi tiết doanh thu/chi phí; truy chứng từ và cơ sở tính/chính sách |
| File | CSV toàn bộ tập lọc và chống formula injection; bản in chứng từ/sổ; file đính kèm theo hash; import CSV qua preview, xác thực cả gói rồi lưu nháp atomic |
| Khôi phục | VACUUM INTO snapshot, ZIP/manifest/checksum, schema fingerprint/integrity/FK/cân đối/khớp kho/tài sản; staging xác minh, backup trước restore, giữ dataset cũ, atomic pointer, epoch mới, thu hồi phiên, nhật ký ngoài dataset |
| UI/UX | Giao diện desktop/mobile, lazy route, trạng thái tải/lỗi/rỗng, form nháp giữ khi chuyển trang, cảnh báo trước thoát, tiền lớn chính xác; API thật mặc định, mẫu chỉ khi bật env tường minh |

Tài khoản tự động hiện dùng danh mục khởi tạo cố định. Khai báo chế độ kế toán trong policy không tự đổi chart/mẫu báo cáo. Thông số thuế/bảo hiểm là dữ liệu doanh nghiệp xác nhận, không phải bộ tham số pháp định đã được hệ thống cập nhật tự động. Không dùng kết quả lỗi hoặc tham số năm 2022 từ workbook làm chuẩn.

## Validation đã chạy

- Go: unit và integration trên SQLite file thật; `go test -race ./internal/... ./cmd/server` và `go vet ./internal/... ./cmd/server` đã đạt. Driver pin v1.60.1, runtime SQLite 3.53.4 trên Linux amd64.
- Các case: tiền trên 2^53/int64/làm tròn; reopen; posted immutability; rollback cả cấp số/audit/idempotency; gửi lại cùng intent và cạnh tranh ghi/khóa kỳ; stale version/epoch, trùng nguồn giữa nhập tay/CSV, cấu hình mọi read connection, từ chối tiến trình thứ hai.
- Nghiệp vụ: FIFO/bình quân với kết quả độc lập, nhiều dòng cùng hàng và âm kho rollback; policy tương lai không đổi giá tồn cũ; lương giả lập gross 2.600, thuế 185, net 2.315; cơ sở tính giữ sau đổi nhân viên; phân bổ 100 qua ba tháng 33/33/34; chi phí dở dang còn 400 không bị kết chuyển; TNDN giả lập 140, kết quả sau thuế 460, mở lại/kết chuyển lại không nhân đôi số.
- Báo cáo/import/recovery: tổng toàn bộ tập lọc và phân trang, cân đối hai bên, CSV bảo vệ cả metadata/header; preview không ghi và lỗi gói rollback; backup/restore có attachment, kiểm tra traversal/tampered staging, epoch mới và dữ liệu sau snapshot bị loại.
- Frontend: TypeScript strict và 15 test Node đạt. Playwright 7 luồng dữ liệu mẫu và 5 luồng backend thật đã đạt; bao phủ ghi nhận bán hàng, xem cơ sở tính, retry sau commit không trùng, tiền lớn, đổi cấu hình từ UI, backup và restore/đăng nhập lại/bỏ cache cũ, mobile 390px.
- Build frontend đạt, route tách chunk. Chunk chia sẻ Ant Design khoảng 700 kB / 227 kB gzip, Vite còn cảnh báo chunk trên 500 kB; chưa benchmark hiệu năng trên máy/dữ liệu mục tiêu.
- Ảnh desktop 1440px và mobile 390px đã được kiểm tra; bảng có vùng cuộn riêng. Kiểm tra console ở luồng khởi tạo/bán hàng không có lỗi JavaScript.
- OpenAPI ghi 36 operation thực tế; đã đối chiếu đủ method/path với router, kiểm tra các schema reference, Unicode NFC và liên kết tài liệu. Validation tiếng Việt trên 62 file: 0 lỗi, 2 cảnh báo định dạng ở fixture CSV cố ý giữ chuỗi số canonical. Lượt cuối Go race/vet, typecheck/build và 5 E2E backend thật đạt; hồi quy 7 E2E mẫu cũng đã được chạy.

## Chưa hoàn tất

- Chưa bao phủ toàn bộ nghiệp vụ của 84 sheet. Chưa có nhập số dư chuyển đổi có mốc/cutoff riêng, chart cha/con có phiên bản, nhiều kho/đơn vị quy đổi, kiểm kê/điều chuyển/trả lại/điều chỉnh giá hồi tố hoặc đảo nghiệp vụ phân hệ tổng quát.
- Thu/chi và thanh toán công nợ hiện dùng chứng từ thủ công; chưa có phân bổ thanh toán theo hóa đơn, tuổi nợ, đối soát sao kê ngân hàng hoặc tích hợp hóa đơn điện tử.
- Lương nhận ngày công tổng hợp; chưa có bảng chấm công theo ngày, trần/điều kiện BH tự động, quy tắc miễn làm thêm theo từng đối tượng hoặc quyết toán TNCN năm. Chưa có thanh lý/điều chỉnh tài sản hay phân bổ giá thành nhiều dịch vụ theo tiêu thức.
- Chưa có bộ BCTC pháp định, lưu chuyển tiền tệ, mẫu tờ khai/XML và gửi cơ quan thuế, phiên bản mapping chế độ/tài khoản/báo cáo đã thẩm định. TNDN hiện là phép tạm tính một khoảng không chồng lấn, chưa xử lý nộp bổ sung/quyết toán và điều chỉnh tự động các khoảng đã ghi nhận.
- Chưa có parser XLSX, nhập workbook liên kết ngoài, lịch tác vụ định kỳ, lịch/retention backup hoặc dọn staging/blob mồ côi tự động.
- Chưa nghiệm thu production, tải lớn, bảo mật độc lập, Windows, mất điện/hết đĩa ở từng điểm chuyển dataset hoặc restore drill định kỳ. Không dùng kết quả test dữ liệu giả lập làm bảo đảm tuân thủ pháp lý hoặc số liệu doanh nghiệp thực tế.

## Dữ kiện và vận hành

- Workbook: 84 sheet, 60.135 ô công thức, 7.299 giá trị lỗi lưu sẵn, hai workbook liên kết chưa cung cấp; nhiều sheet là hướng dẫn/ví dụ/nhập tay. Phân tích/hash và ánh xạ tại `reference/`; chưa chạy macro hay tính lại workbook bằng Excel.
- Mã nguồn được quản lý Git trên nhánh `release/v0.2.0`, bản phát hành local `v0.2.0`; chưa có Git remote. Không đưa DB, backup, mật khẩu hoặc dữ liệu tài chính thực vào mã nguồn.
- Test/E2E dùng thư mục tạm và dữ liệu giả lập. Hướng dẫn chạy, khởi tạo và giới hạn triển khai ở README; chi tiết API ở `api/openapi.yaml`; runbook recovery ở `backup-restore.md`.

## Bước tiếp theo

1. Nghiệm thu tập chứng từ/kết quả độc lập cùng kế toán phụ trách cho từng cấu hình thực tế.
2. Hoàn thiện chart/mapping báo cáo có phiên bản, số dư chuyển đổi và các nghiệp vụ điều chỉnh/trả lại; sau đó triển khai hồ sơ BCTC/thuế đúng kỳ.
3. Bổ sung nghiệp vụ công nợ, ngân hàng, chấm công và phân bổ chi tiết còn thiếu theo ma trận workbook; đo hiệu năng và thử khôi phục trên máy mục tiêu trước triển khai.
