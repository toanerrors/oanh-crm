# Kiến trúc ứng dụng kế toán

Ngày chốt nền tảng: 21/10/2026. Mục 0 ghi hiện thực đã có; các mục tiếp theo giữ thiết kế đích. Trạng thái nghiệm thu nằm trong `PROJECT_STATUS.md`.

## 0. Hiện thực ngày 21/10/2026

- `server/cmd/server` nối dependency, chạy HTTP và phục vụ Vite build cùng origin. `identity` quản lý khởi tạo/phiên/quyền; `ledger` sở hữu lệnh transaction và sổ; `operations` dùng `ledger.SaveTx/PostTx` để ghi cùng biến động kho/lương/tài sản và cơ sở tính. Chưa tách package rỗng cho từng phân hệ dự kiến.
- `operations/treasury.go` bổ sung thu/chi, chuyển tiền và đảo phiếu tiền qua cùng đường transaction. Phân bổ công nợ và thông tin hóa đơn nằm trong payload nghiệp vụ bất biến; truy vấn công nợ đọc nguồn mua/bán/ghi tăng tài sản, phân bổ và chứng từ đảo trong một snapshot, có tổng toàn bộ tập lọc. Bút toán giảm công nợ ngoài luồng được đánh dấu để đối soát, không tự gán vào hóa đơn. Không thêm schema hay bảng số dư riêng.
- `internal/accounting` dùng int64 cùng math/big cho tiền, decimal, phần dư và kiểm tra tràn. `internal/platform/sqlite/schema.sql` là migration nhúng phiên bản 1, có checksum; schema STRICT và trigger bảo vệ chứng từ đã ghi sổ. Driver `modernc.org/sqlite` v1.60.1 đã được chấp thuận, SQLite nhúng kiểm tra thực tế 3.53.4. Không phụ thuộc SQLite CLI.
- Một write connection với `_txlock=immediate`, read pool tối đa 3; DSN cấu hình foreign_keys, busy_timeout, synchronous trên từng connection. WAL, FULL; khóa flock cho một tiến trình. Unix được hỗ trợ, Linux amd64 đã kiểm tra; nền tảng khác fail rõ ràng.
- Một lệnh nghiệp vụ qua `ledger.Write` kiểm tra user đang hoạt động/quyền và epoch trong transaction, tìm lại kết quả idempotency, thực thi nghiệp vụ, tăng revision, ghi audit/commands rồi commit. Hash tính từ DTO có kiểu. Nháp có version; nguồn chuẩn hóa theo loại nghiệp vụ, không tách namespace theo kênh nhập.
- Ghi sổ cấp counter theo loại/năm và posting order; kiểm tra Nợ/Có, kỳ, danh mục và đối tượng. Tổng không ép sang float. Chứng từ thủ công không được tự ghi tài khoản kho/tài sản kiểm soát phân hệ. Đảo thủ công có liên kết; chứng từ phân hệ chưa hỗ trợ đảo/trả lại tổng quát.
- Chính sách append theo ngày hiệu lực tăng dần, xác nhận tường minh; lưu JSON bất biến, ID/version và input/kết quả cùng chứng từ tự động. Tỷ lệ thuế/bảo hiểm không mặc định từ mẫu cũ. Khai báo chế độ kế toán trong policy là tham chiếu được xác nhận; không tự chuyển hệ thống tài khoản hoặc mẫu báo cáo theo tên chế độ. Tài khoản tự động hiện cố định theo danh mục khởi tạo.
- Kho FIFO hoặc bình quân sau lần nhập, một phạm vi kho; từ chối âm kho/lùi trước biến động cuối và trộn phương pháp/scale khi còn tồn, kể cả chính sách lên lịch tương lai. 154 là nhóm chi phí dở dang riêng, không tham gia kết chuyển doanh thu/chi phí. Kết chuyển giá thành không vượt dư 154; kết chuyển kết quả khóa kỳ. Mở lại kỳ đảo kết chuyển có liên kết trong cùng transaction và yêu cầu xử lý các kỳ kết chuyển sau trước.
- Cookie phiên ngẫu nhiên, chỉ lưu hash ở DB; mật khẩu PBKDF2-SHA256 600.000 vòng với salt ngẫu nhiên và so sánh constant time. Đăng nhập sai bị khóa tạm; cập nhật người dùng thu hồi phiên. CrossOriginProtection, Host local mặc định và header bảo vệ được bật. Chưa nghiệm thu triển khai HTTPS/proxy production hoặc kiểm thử bảo mật độc lập.
- Frontend mặc định API thật. Query key chứa actor/quyền/company/epoch/kỳ/bộ lọc; form nháp riêng, không persist tài chính/token ở trình duyệt. Mutation không optimistic update hoặc tự retry; giữ key sau lỗi kết nối. Đổi phiên/epoch hủy request và bỏ cache; restore yêu cầu đăng nhập lại.
- Workspace chỉ trả 100 chứng từ gần nhất theo kỳ; dashboard dùng metrics server. Danh sách và báo cáo đọc transaction snapshot, phân trang/lọc và tổng toàn bộ tập lọc ở server. Hiện truy vấn/tổng nhiều phần dùng math/big và quét tập dữ liệu trong snapshot; chưa benchmark dữ liệu lớn. CSV/in giới hạn 100.000 dòng, import 500 nháp/5.000 dòng/1 MiB.
- Recovery dùng VACUUM INTO + fsync + kiểm tra snapshot, archive và attachment; manifest/hash/schema fingerprint/integrity/FK/cân đối/khớp phân hệ. Con trỏ active dataset công bố atomic sau khi xác minh; restore giữ dataset cũ, backup trước chuyển, đổi epoch và thu hồi phiên. Nhật ký chuyển dataset ở `data/recovery/` tồn tại ngoài dataset. Xem runbook recovery về giới hạn kiểm thử crash.

### Đóng gói release Docker

Dockerfile nhiều stage build frontend/Go, runtime scratch chạy UID 65532, root filesystem read-only và `/tmp` tmpfs. Compose cấp volume `oanh_data` tại `/data`, một writer, giới hạn 2 CPU/1 GiB và log quay vòng. Host chỉ publish `127.0.0.1:8080`; cookie Secure bật cho HTTPS qua Cloudflare Tunnel. Healthcheck dùng binary Go riêng để đọc trạng thái API/database, không mở thêm endpoint hoặc thay quy tắc nghiệp vụ. Xem `deployment.md` về vận hành và giới hạn xác minh domain.

## 1. Quyết định nền tảng

| Hạng mục | Quyết định | Mục đích |
|---|---|---|
| Backend | Go, ưu tiên thư viện chuẩn | Luồng nghiệp vụ tường minh, dễ đóng gói và vận hành |
| Frontend | React, TypeScript strict, Vite SPA | Điều hướng phía client, chia bundle theo phân hệ, giao diện nhập liệu linh hoạt |
| UI | Ant Design | Thống nhất layout, menu, tab, dialog và các thành phần nhập liệu |
| Form và bảng | Ant Design Form, Ant Design Table | Biểu mẫu động, validation và danh sách/sổ sách có chung quy ước |
| Ô tiền | `MoneyInput` bọc `InputNumber` với `stringMode` | Giữ giá trị chuỗi chính xác; backend kiểm tra và tính lại |
| Tùy chỉnh giao diện | Theme token và CSS Modules | Một hệ thống UI và mật độ hiển thị thống nhất |
| Điều hướng | React Router | Điều hướng SPA, lazy route và giữ app shell |
| Server state | TanStack Query | Cache, hủy request, tải lại và đồng bộ dữ liệu sau mutation |
| Database | SQLite trên local disk | Đơn giản triển khai, snapshot để backup và chuyển máy |
| Phạm vi khởi đầu | Một pháp nhân, một tiến trình có quyền ghi | Tránh thiết kế SaaS khi nhu cầu chưa có; giới hạn ghi rõ ràng |
| Cách chia hệ thống | Một backend chia module nghiệp vụ | Ghi sổ liên phân hệ trong cùng transaction, tránh phân tán dữ liệu |
| Triển khai frontend | Vite build thành file tĩnh, Go phục vụ cùng origin | Một API origin, dễ đóng gói ứng dụng và quản lý quyền truy cập |
| Dữ liệu số | Tiền INTEGER minor units; decimal chính xác cho các giá trị còn lại | Không mất số qua DB, JSON và JavaScript |

Vite là công cụ phát triển/build. Tốc độ chuyển trang còn phụ thuộc router, lượng JS, cache, truy vấn, bảng dữ liệu và render; cần đo trên dữ liệu đại diện trước khi kết luận hiệu năng.

## 2. Cấu trúc thư mục đích

Các thư mục dưới đây được tạo khi bắt đầu triển khai phần tương ứng, không tạo sẵn package rỗng cho toàn bộ 12 phân hệ.

```text
.
├── AGENTS.md
├── README.md
├── api/
│   ├── README.md
│   └── openapi.yaml                  # tạo khi chốt hợp đồng API đầu tiên
├── docs/
│   ├── PROJECT_STATUS.md
│   ├── architecture.md
│   ├── accounting-correctness.md
│   ├── backup-restore.md
│   └── reference/                    # phân tích workbook và bảng đối chiếu
├── server/
│   ├── AGENTS.md
│   ├── go.mod
│   ├── go.sum
│   ├── cmd/server/main.go
│   ├── migrations/                  # SQL đánh số, kiểm soát checksum
│   ├── internal/
│   │   ├── app/                     # cấu hình và nối dependency
│   │   ├── accounting/              # kiểu tiền/decimal, chính sách làm tròn
│   │   ├── platform/
│   │   │   ├── sqlite/              # pool, transaction, migration
│   │   │   ├── httpserver/          # middleware, response/error, static SPA
│   │   │   ├── filestore/           # blob bất biến và xác minh hash
│   │   │   └── recovery/            # snapshot, gói backup, restore
│   │   └── modules/
│   │       ├── identity/            # session, vai trò và quyền
│   │       ├── organization/        # doanh nghiệp, kỳ, chế độ và tham số
│   │       ├── catalog/             # tài khoản, đối tác, hàng, kho, ngân hàng
│   │       ├── ledger/              # chứng từ, bút toán, số phiếu, sổ
│   │       ├── trade/               # mua/bán và nghiệp vụ đặc thù
│   │       ├── treasury/            # tiền mặt, ngân hàng và phí
│   │       ├── inventory/           # biến động kho, giá xuất, chi phí mua
│   │       ├── receivables/         # phải thu, phải trả và phân bổ thanh toán
│   │       ├── assets/              # TSCĐ, CCDC, khấu hao và TK242
│   │       ├── payroll/             # nhân viên, công, lương, BH và TNCN
│   │       ├── costing/             # đối tượng chi phí và giá thành dịch vụ
│   │       ├── tax/                 # kê khai, điều chỉnh và tạm tính TNDN
│   │       └── reporting/           # BCTC, mẫu in, export và thuyết minh
│   └── testdata/                    # dữ liệu giả lập, không chứa dữ liệu thật
├── web/
│   ├── AGENTS.md
│   ├── package.json
│   ├── pnpm-lock.yaml
│   └── src/
│       ├── app/                     # router, provider, shell, bootstrap context
│       ├── features/                # một feature theo màn hình/nghiệp vụ
│       └── shared/
│           ├── api/                 # transport và kiểu hợp đồng
│           ├── accounting/          # parser/formatter chính xác, không ghi sổ
│           └── ui/                  # thành phần chung có nhu cầu thật
└── data/                            # runtime, ngoài Git và có thể đặt ngoài repo
    ├── datasets/<dataset-id>/
    │   ├── accounting.sqlite
    │   └── attachments/
    ├── active-dataset.json          # con trỏ dataset được chuyển có kiểm soát
    ├── backups/
    └── staging/                     # import/restore chưa kích hoạt
```

Tên folder kỹ thuật không phải số lượng phân hệ sản phẩm. Ví dụ `receivables` quản lý cả phải thu/phải trả; hướng dẫn nghiệp vụ có thể là nội dung trợ giúp trong frontend thay vì một backend module riêng.

## 3. Ranh giới và luồng phụ thuộc

- HTTP handler nhận DTO và gọi application service; không tự mở transaction, ghi kho hoặc tính thuế.
- Application service sở hữu use case và transaction. Nó gọi các port công khai của module tham gia, cùng transaction handle.
- Module ledger sở hữu sổ kế toán đã ghi nhận. Treasury, inventory, receivables, assets và payroll giữ chi tiết chuyên biệt, có liên kết về chứng từ/bút toán.
- Không module nào được tự sửa bảng riêng của module khác. Khi một use case đi qua nhiều module, service điều phối sử dụng port được thiết kế cho transaction đó.
- Chỉ chia phần dùng chung khi có nhiều nơi sử dụng thực tế. Không thêm `pkg/`, `utils/`, generic repository hoặc dependency injection framework theo thói quen.

Một lệnh ghi sổ thực hiện theo thứ tự:

1. Xác thực request; parse số liệu chính xác và kiểm tra cấu trúc.
2. Vào transaction ghi; xác minh dataset, quyền, idempotency, version, kỳ và trạng thái nguồn.
3. Tính lại từ dòng chi tiết và policy có phiên bản; kiểm tra invariant và giới hạn số.
4. Tạo chứng từ/bút toán, cập nhật biến động kho/công nợ/tiền theo nghiệp vụ; cấp số; ghi audit và job/outbox nếu cần.
5. Ghi kết quả idempotency, commit, rồi trả dữ liệu authoritative. Mọi lỗi trước commit phải rollback toàn bộ.

Số dư được lưu để tăng tốc là dữ liệu dẫn xuất. Cần quy tắc cập nhật trong transaction, truy vấn đối soát và khả năng dựng lại; không duy trì các con số độc lập không có nguồn.

## 4. SQLite, đồng thời và độ bền

- Một tiến trình được phép ghi vào mỗi dataset. Bắt đầu bằng một write connection và read pool nhỏ; job ghi cũng đi qua đường ghi chung.
- Database có WAL, `synchronous=FULL`, khóa ngoại và timeout được cấu hình đúng trên từng connection. Dùng schema `STRICT` cho bảng nghiệp vụ.
- SQLite chỉ có một writer tại một thời điểm; WAL cho phép reader hoạt động cùng writer nhưng không biến SQLite thành hệ thống nhiều writer đồng thời. Theo dõi thời gian chờ ghi, `SQLITE_BUSY`, thời gian transaction và kích thước WAL. [Tài liệu SQLite WAL](https://sqlite.org/wal.html).
- Baseline SQLite nhúng từ 3.51.3 trở lên vì các phiên bản này có bản sửa WAL-reset. Kiểm tra `SELECT sqlite_version()` và advisory khi pin driver, không kiểm tra riêng CLI. [Thông báo WAL-reset trong tài liệu SQLite](https://sqlite.org/wal.html#walresetbug).
- Không tạo transaction ghi dài khi import hoặc tạo báo cáo. Parse, kiểm tra và tính preview ngoài transaction; lặp lại kiểm tra phụ thuộc trạng thái bên trong transaction ghi.
- Dữ liệu nhiều bảng dùng cho một báo cáo được đọc trong snapshot nhất quán. Index theo doanh nghiệp/kỳ/tài khoản/đối tượng/ngày và khóa phân trang thực tế; đo bằng query plan.
- Không chia bút toán và số dư giữa các database riêng trong phiên bản đầu. Khi khối lượng ghi vượt khả năng đo được, đánh giá thay database hoặc phạm vi vận hành trước khi thêm nhiều writer.

## 5. API, định danh và quyền

- REST JSON, `/api/v1`; hợp đồng theo `api/README.md`. Chọn OpenAPI trước khi sinh client hoặc viết nhiều màn hình để hạn chế lệch DTO.
- ID bản ghi độc lập số chứng từ nghiệp vụ. Số chứng từ có phạm vi doanh nghiệp/năm/loại theo chính sách được duyệt; không tái sử dụng số đã hủy để che khoảng trống.
- Phạm vi dữ liệu luôn tường minh. Một pháp nhân ban đầu không đồng nghĩa bỏ kiểm tra company ID hoặc quyền đối tượng trong API.
- Session ngẫu nhiên phía server, DB chỉ giữ hash; cookie HttpOnly/Secure trên HTTPS/SameSite Strict, kiểm tra Origin cho thao tác ghi. Từ0.11.0, POST /auth/login nhận remember tùy chọn: true giữ30ngày, false/bỏ qua giữ12giờ; thời hạn DB và cookie lấy cùng quy tắc. Form đăng nhập chọn sẵn “Giữ đăng nhập30ngày”, có thể bỏ chọn. Không dùng JWT/refresh token/localStorage; request đọc không tự kéo dài phiên. Giới hạn là cố định từ lúc đăng nhập, đăng xuất/đổi mật khẩu/quyền/khôi phục vẫn thu hồi phiên. Phiên đã cấp trước nâng cấp giữ hạn cũ, cần đăng nhập lại một lần để chọn thời hạn mới. Kiểm thử thật SQLite có clock giả lập24giờ/30ngày, reopen, rollback và thu hồi phiên. Tham khảo thuộc tính cookie và giới hạn phiên: [OWASP Session Management](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html).
- Quyền xem lương, xem báo cáo, nhập nháp, ghi sổ, khóa kỳ và restore là các quyền phân biệt. Audit ghi actor, hành động, nguồn, version, request ID và thời điểm.
- Audit trong cùng SQLite hỗ trợ truy vết ứng dụng, không tự tạo khả năng chống sửa bởi quản trị có toàn quyền filesystem. Nếu cần mức chống sửa cao hơn, thiết kế lưu dấu độc lập theo yêu cầu cụ thể.

## 6. Frontend nhẹ và điều hướng nhanh

- Router điều hướng phía client; app shell giữ nguyên. Sáu phân hệ chứng từ dùng `ModuleLayout` và route con cho báo cáo; mua/bán có thêm công nợ, đối tượng và hàng hóa. Thanh tab/menu giữ phân hệ nguồn; chỉ mount nội dung route đang mở. Đường dẫn tab gần nhất giữ trong phiên layout; tab và bộ lọc báo cáo nằm trong URL để hỗ trợ reload/Back/Forward. Các route độc lập cũ vẫn hoạt động. Chia chunk theo phân hệ; prefetch route và query theo ý định thay vì nạp toàn bộ ứng dụng.
- TanStack Query giữ dữ liệu server; filter/sort/page/tab có thể phản ánh trong URL; form nháp dùng state riêng có giới hạn vòng đời.
- Từ0.9.0: tìm danh sách/sổ cập nhật input ngay, chỉ đổi bộ lọc API sau250ms hoặc Enter/blur; kết thúc IME mới tìm. Placeholder chỉ qua đổi page khi toàn bộ actor/quyền/company/epoch/kỳ/bộ lọc/pageSize/sort/revision giữ nguyên, có nhãn cập nhật và khóa thao tác trên trang cũ. Tải trước chunk theo hover/focus, không mount màn hình hoặc prefetch số liệu tài chính. Phiếu nghiệp vụ giữ form local khi gõ; router blocker chụp nháp vào bộ nhớ phiên khi rời màn hình, giữ ID/version và đường trở về danh sách đã lọc. Khi lệnh đang chạy, chặn chuyển route; refresh/đóng tab có cảnh báo. Ctrl/⌘ S chỉ lưu nháp, Ctrl/⌘ Enter chỉ xem trước, ghi sổ vẫn xác nhận riêng.
- Số dư và báo cáo hiển thị context kỳ/as-of/revision. Background refetch không làm mất nội dung đang nhập và không hiển thị số 0 thay cho trạng thái loading/lỗi.
- Sau mutation, invalidate đúng danh sách, chi tiết và các báo cáo bị tác động. Client của người khác hoặc tab khác dùng refetch/revision; chỉ thêm SSE nếu cần cập nhật chủ động và có bằng chứng.
- Danh sách lớn phân trang trên server; tổng toàn bộ tập lọc do API trả. Thêm virtualization sau khi xác định số hàng/render thực sự cần.
- Giao diện dùng CSS variables và component có semantic, focus, phím tắt rõ ràng. Một hệ thống UI thống nhất; không cài đồng thời nhiều UI kit.
- Ant Design đã được người dùng chọn cho UI. Form state dùng Ant Design Form, bảng mặc định dùng Ant Design Table; QueryClient quản lý dữ liệu API. Theme token và CSS Modules áp dụng màu sắc, khoảng cách và mật độ hiển thị chung.
- `MoneyInput` dùng chuỗi chính xác xuyên suốt form/API, bọc `InputNumber` với `stringMode`; mọi parser/formatter/giới hạn phải giữ độ chính xác. Kiểm tra blur, step, wheel và locale; backend luôn tính lại và không tin tổng tiền từ UI.
- Trước khi triển khai nhiều màn hình, kiểm tra một form chứng từ nhiều dòng với bàn phím, chọn tài khoản, tiền lớn, thêm/xóa dòng và chuyển tab giữ nháp; đo render và bundle trên dữ liệu đại diện. Lựa chọn Ant Design không tự bảo đảm tốc độ hoặc tính đúng nghiệp vụ.
- Production: Go phân biệt `/api/` và file static. Chỉ fallback `index.html` cho đường điều hướng HTML; lỗi API và asset thiếu không trả HTML 200. Asset có hash được cache dài; HTML kiểm tra phiên bản mới. Không dùng Vite dev server/preview làm server production.
- Chưa xác lập chỉ tiêu hiệu năng được nghiệm thu. Khi có màn hình đầu tiên, ghi baseline bundle, thời gian chuyển route có/không cache và độ trễ nhập bảng chứng từ trên cấu hình máy đại diện.

## 7. Giải pháp bổ sung đề xuất

Đây là các lựa chọn để cân nhắc khi khởi tạo; chưa có dependency nào được cài. Production dependency bổ sung tuân theo quy tắc chấp thuận trong AGENTS.md.

| Nhu cầu | Đề xuất | Điều kiện sử dụng |
|---|---|---|
| Go SQLite driver | `modernc.org/sqlite` | Pure Go, có API backup; pin bản nhúng SQLite đã sửa lỗi và kiểm tra trên hệ điều hành đích |
| SQL | SQL tường minh + `database/sql` | Ưu tiên đọc được transaction và truy vấn; cân nhắc sqlc khi số truy vấn đủ lớn và công cụ được chọn |
| Decimal backend | Bọc `math/big` | Chính xác và không thêm runtime dependency; chỉ thêm thư viện decimal sau khi so sánh rounding, overflow, serialization và maintenance |
| Nhập liệu kiểu Excel nâng cao | Đánh giá AG Grid khi yêu cầu vượt Form/Table hiện có | Chọn vùng, clipboard nâng cao và Excel export tích hợp thuộc Enterprise; xác minh license, đo hiệu năng và test editor chuỗi trước khi chọn |
| Kiểm thử | Go testing; Vitest/Testing Library; Playwright | Kiểm tra số liệu, SQLite thật và các luồng quan trọng; chọn/cài khi bắt đầu viết mã |
| Truy vết | `log/slog`, request ID, metrics ghi/backup/busy | Không log nội dung lương, toàn bộ chứng từ hoặc dữ liệu nhạy cảm |
| Job nền | Job bền vững trong SQLite; worker trong cùng ứng dụng | Trạng thái, retry có giới hạn, idempotency, không mất việc khi restart |
| Backup | Snapshot DB + blob bất biến + manifest/hash | Có restore drill, bản sao trên thiết bị/vị trí khác và chính sách lưu phù hợp |

Các khuyến nghị này nhằm giữ hệ thống nhỏ và có thể kiểm chứng; không phải tuyên bố công cụ nhanh hơn mọi lựa chọn khác.

## 8. Thứ tự xây dựng

1. Chốt policy số liệu còn mở; tạo kiểu tiền/decimal, test invariant, cấu hình SQLite, migration đầu và recovery adapter.
2. Định nghĩa API context/version/idempotency; xây một lát cắt từ nháp → ghi sổ → sổ cái → đối soát → backup/restore. Xác minh crash/retry trước khi nhân rộng module.
3. Tạo app shell Vite, router và QueryClient; triển khai đúng lát cắt trên frontend, gồm lỗi version, loading và nháp.
4. Mở rộng danh mục, thu/chi, kho, công nợ, sổ. Sau đó tài sản/lương/giá thành; cuối cùng hoàn thiện thuế/BCTC, kết chuyển và nghiệm thu đủ bảng đối chiếu.

## 9. Các quyết định nghiệp vụ cần xác nhận trước khi triển khai phần liên quan

- Đồng tiền sử dụng, giới hạn số, scale quantity/đơn giá/tỷ giá và policy làm tròn theo nghiệp vụ.
- Phương pháp tính giá xuất kho; cách xử lý âm kho, hàng trả lại, chi phí phát sinh muộn và hồi tố qua kỳ khóa.
- Cấu trúc số chứng từ, quyền ghi sổ/duyệt/khóa kỳ và quy trình sửa sai.
- Chính sách khấu hao/phân bổ đầu/cuối kỳ, tập hợp giá thành và phiên bản quy định thuế/bảo hiểm.
- Mục tiêu mất dữ liệu tối đa (RPO), thời gian khôi phục (RTO), lịch backup, nơi lưu bản sao và phân quyền restore.
- Mức giữ nguyên mẫu in và nhu cầu hóa đơn điện tử/ngân hàng/thuế/bảo hiểm điện tử.

Khi chưa được xác nhận, ghi rõ quyết định đang mở và xây phần không phụ thuộc quyết định đó. Không tự áp dụng giả định làm thay đổi số tiền hoặc sổ kế toán.

## 10. Mốc giao diện mẫu đã triển khai

Frontend local được triển khai trước backend để kiểm tra luồng nhập nhiều dòng và trải nghiệm điều hướng. `web/src/shared/data/workspace.ts` là adapter dữ liệu giả lập trong bộ nhớ; Query giữ cache theo danh tính/quyền mẫu, doanh nghiệp, epoch và kỳ. Biểu mẫu nháp giữ riêng ở cấp ứng dụng, được bảo toàn qua chuyển route trong phiên.

Các tổng hợp, lọc và phân trang hiện chạy trên bộ mẫu hữu hạn phía client. Khi kết nối API, chuyển các thao tác này sang server với cùng snapshot/phạm vi; không dùng adapter mẫu làm kho dữ liệu production. Bộ mẫu chỉ dùng tài khoản và chứng từ tổng hợp để kiểm tra UI, chưa thiết lập chế độ kế toán hoặc tham số thuế cho doanh nghiệp.

Giao diện chưa cung cấp lệnh ghi sổ thật; các trạng thái đã ghi sổ đến từ fixture. Quyền, kỳ, idempotency, transaction, cấp số và recovery vẫn cần triển khai theo thiết kế ở trên. Không persist cache hoặc nháp vào localStorage; xuất JSON hỗ trợ giữ một bản nháp trước khi kết thúc phiên.

## Tài liệu kỹ thuật đã đối chiếu

- [Go: tổ chức module](https://go.dev/doc/modules/layout) và [transaction với database/sql](https://go.dev/doc/database/execute-transactions).
- [SQLite: số thực gần đúng](https://sqlite.org/floatingpoint.html), [STRICT tables](https://sqlite.org/stricttables.html), [SUM và overflow](https://sqlite.org/lang_aggfunc.html).
- [modernc.org/sqlite: driver và backup](https://pkg.go.dev/modernc.org/sqlite).
- [TanStack Query: invalidation sau mutation](https://tanstack.com/query/latest/docs/framework/react/guides/invalidations-from-mutations) và [query cancellation](https://tanstack.com/query/latest/docs/framework/react/guides/query-cancellation).
- [Vite: build optimization](https://vite.dev/guide/features.html#build-optimizations) và [React Router: route object/lazy](https://reactrouter.com/start/data/route-object).
- [Ant Design Form](https://ant.design/components/form/), [Table](https://ant.design/components/table/) và [InputNumber stringMode](https://ant.design/components/input-number/).
- [AG Grid: Community và Enterprise](https://www.ag-grid.com/react-data-grid/community-vs-enterprise/).

Tài liệu Vite, TanStack Query và SQLite đã được tra bằng Context7; các chi tiết Go/driver/router và thông báo SQLite được đối chiếu thêm từ tài liệu chính thức. Khi chọn phiên bản cài đặt, tra lại tài liệu đúng phiên bản.

## Biểu mẫu nghiệp vụ và chiều ngân hàng/kho — schema 2

`/vouchers/new` và `/vouchers/:id` dùng Ant Form/Table cho thu/chi, chuyển nội bộ, mua/bán hàng/dịch vụ và kết chuyển. `operations` giữ payload/cơ sở tính; `ledger` tiếp tục là nguồn bút toán duy nhất. Nháp lưu trong `voucher_inputs` và các dòng ledger nháp; service tính kiểm tra qua SAVEPOINT rồi rollback mọi biến động trước khi chỉ lưu nháp. Ghi sổ phải chạy lại qua operations; API nhập tay không được sửa/ghi sổ trực tiếp nháp phân hệ. Trạng thái đang nhập giữ trong WorkspaceProvider theo danh tính/quyền/epoch, không persist trình duyệt.

`bank_accounts` liên kết số tài khoản với tài khoản sổ; `bank_movements` là chi tiết hai chiều, kể cả hai ngân hàng dùng cùng TK112. `warehouses` và hai bảng chiều gắn lô/biến động với kho; migration gắn toàn bộ tồn cũ vào `default`, không thay giá trị. Giá kho tính theo từng item–warehouse; API automation vẫn trả tồn tổng và API warehouses trả tồn chi tiết cùng snapshot. Các biến động đã ghi sổ bất biến; lô còn lại được thay đổi theo phương pháp giá kho đã xác nhận.

Chi phí mua hàng giữ liên kết documentId + chỉ số dòng nguồn bất biến trong payload operations; mỗi lần phân bổ kiểm tra phần còn lại bên trong write transaction. Hàng nhập nhận thêm giá trị, đồng thời ghi Có TK chi phí của nguồn, không ghi VAT/công nợ lần hai. Làm tròn/phân bổ dùng math/big; kết quả từng dòng giữ trong calculation. Kiểm tra backup/khởi động đối chiếu kho theo chiều và tổng giá trị với ledger, chi phí phân bổ với nguồn và ngân hàng với bút toán.

Bộ tài khoản tham chiếu được embed, đóng góp vào checksum migration 2. Chỉ bổ sung mã còn thiếu; chart tham chiếu không tự đổi phân nhóm tài khoản có lịch sử hoặc tạo rollup báo cáo bằng tiền tố. Không chỉnh schema.sql/migration 1 đã phát hành.


## Bản nháp BCTC — schema3

Các truy vấn và bản nháp BCTC hiện nằm trong `ledger/financial.go`, sử dụng cùng read snapshot và `ledger.Write` cho kiểm tra quyền/epoch/version, idempotency, audit và commit. Không tạo bản sao sổ hoặc ghi bút toán từ BCTC. `financial_reports.data` giữ input có template version; tổng dẫn xuất được tính lại, không chấp nhận client đặt tổng. Giữ nguyên định nghĩa `tt99-2026-v1` khi bổ sung mẫu tương lai để đọc lại báo cáo lịch sử.

Mẫu năm dương lịch TT99 cho hoạt động liên tục; chỉ tiêu nhập tay, không suy ra phân loại ngắn/dài hạn hoặc luồng tiền từ mã tài khoản. Source là bảng đối chiếu phát sinh/số dư theo khoảng ngày, giữ riêng hai bên theo đối tượng. B09 là nội dung bổ sung tự do; không phải bộ thuyết minh pháp định đầy đủ. UI thay tab income bằng financial, URL income cũ vẫn mở BCTC; API income giữ tương thích.

### Số dư đầu kỳ (0.5.0)

`operations/opening.go` điều phối nháp và xác nhận khởi tạo qua ledger.Write/SaveTx/PostTx, tái sử dụng giá kho và tài sản hiện có. Schema4 thêm singleton opening_balances (mốc, nguồn, payload, liên kết document) và opening_assets (số kỳ/lũy kế trước chuyển đổi); không tạo bản sao bảng số dư. Trigger bảo vệ nguồn đã xác nhận và chặn ghi sổ trước mốc; số dư luôn là các dòng nguồnopening trong cùng ledger. Báo cáo phân biệt opening với movement theo nguồn. Backup kiểm tra lại payload→ledger/ngân hàng/kho/tài sản; hash/checksum schema1–3 giữ nguyên, chỉ nâng bản chuẩn bị khi restore.

## Nhập XLSX/CSV — schema5 (0.6.0)

`platform/spreadsheet` đọc/chuẩn hóa raw XLSX bằng Excelize2.11.0 và CSV bằng thư viện chuẩn; không phụ thuộc HTTP/DB. `operations/excel_*` sở hữu schema cột, ghép dữ liệu, điều phối catalog/voucher/opening qua serviceTx hiện có. Ledger công khai lookup mã/nguồn và validator trong cùng transaction; không có bộ ghi sổ riêng cho Excel. Handler multipart parse ngoài write transaction. Không thêm thư viện Excel phía trình duyệt.

`excel_imports` lưu actor/epoch, SHA256 tệp, workbook đã đọc, thiết lập, trạng thái và kết quả. Bản sao raw đã đọc giữ trong DB/backup; không chạy macro/công thức. Preview dùng transaction rollback, commit dùng Ledger.Write cùng kiểm tra revision/idempotency và từng validator nghiệp vụ. Schema1–4/checksum cũ giữ nguyên; schema5 thêm một bảng và hai index. Xem [phạm vi/giới hạn import](excel-import.md).

## Quan hệ tài khoản cha–con — schema6 (0.7.0)

`account_hierarchy(account_id,revision,parent_id)` dùng ID ổn định, FK, khóa account/revision, CHECK tự tham chiếu, trigger chống vòng lặp và sửa/xóa lịch sử. View `account_current_parents` lấy bản mới nhất. Baseline revision0 theo `parentCode` trong bộ nguồn đã nhúng; cặp vận hành1121→112 được khai báo tường minh. Không đoán cha bằng tiền tố. Setup và endpoint nạp tham chiếu bổ sung liên kết còn thiếu, giữ liên kết đã lưu.

`ledger.SaveAccountTx` điều phối lưu danh mục và append quan hệ trong transaction/audit/idempotency hiện có; client gửi parentRevision để chống ghi đè quan hệ. Tối đa10cấp, cha phải tồn tại/đang sử dụng; không đổi cha nếu tài khoản hoặc nhánh con có bất kỳ chứng từ nào. Payload cũ bỏ parentId giữ nguyên quan hệ; trường revision0 bỏ khỏi JSON hash để giữ idempotency cũ. Workspace đọc danh mục/quan hệ trong cùng snapshot. Đây là phân cấp quản lý danh mục; báo cáo/ghi sổ vẫn dùng dòng trực tiếp theo account ID, chưa cộng gộp cây theo ngày hiệu lực.

Frontend dựng Table tree/TreeSelect từ parentId, đếm con trực tiếp, cấp và đường dẫn tên; tìm không dấu giữ các tổ tiên, mở/thu mọi nhánh, thêm con kế thừa nhóm/yêu cầu đối tượng khi tạo. Mã mới không bị ép theo tiền tố cha. Excel tài khoản thêm parentCode; cột không ghép giữ quan hệ cũ, mã cha phải có trước khi nhập con trong cùng tệp. CSV danh mục có mã cha.


## Điều chỉnh sau xác nhận — 0.8.0

Theo yêu cầu23/10/2026, operations quản lý nháp và lịch sử số dư đã xác nhận. Schema7 giữ opening_balances/opening_assets gốc, thêm opening_adjustments (snapshot trước/sau, lý do, actor/time, base/confirmed version, cặp đảo–thay thế), opening_current và current_assets. Ledger vẫn là nguồn số; các view không tạo sổ tài chính thứ hai. Assets/stock_lots là cache phân hệ được đối chiếu với snapshot và movement.

Xác nhận ghi đảo toàn bộ bản hiệu lực và ghi bản mới trong cùng transaction với ngân hàng/kho/tài sản/audit/idempotency. Hai chứng từ có tổng int64 riêng để không tràn khi cộng hai phiên bản lớn. Ledger có cổng prepare/post đảo đầu kỳ; chỉ cổng này dùng tham chiếu lịch sử ngừng hoạt động, DB buộc copy chính xác dòng gốc. Tài khoản/đối tượng bản mới vẫn kiểm tra hiệu lực. Source opening_adjustment/opening_adjustment_reversal thuộc dư đầu tại mọi truy vấn báo cáo và ngân hàng.

Thu/chi/chuyển tiền có replacementOf/replacementReason tùy chọn (omitempty giữ hash payload cũ). Preview và lưu nháp mô phỏng đảo+thay qua savepoint; ghi sổ mới đảo+ghi cùng transaction. Liên kết ở operations.payload bất biến; unique reversal_of/source và kiểm chứng backup ngăn một gốc bị thay nhiều lần. Sửa tiếp bản thay thế tạo chuỗi mới. Không áp dụng đường này cho mua/bán/kho có phụ thuộc.

Đặc tả và giới hạn: [opening-adjustments.md](opening-adjustments.md).


### Xuất báo cáo đa định dạng (0.10.0)

HTTP xuất dùng service đọc snapshot hiện có, sau đó đóng gói ở platform/spreadsheet và template HTML, không giữ transaction khi tạo XLSX. GET documents/reports/export thêm format csv/xlsx/print, mặc định CSV giữ tương thích; catalog/export kiểm tra snapshot revision; financial-reports/export kiểm tra ID/version bản lưu. Excelize2.11.0 đã có sẵn được tái sử dụng, không thêm dependency. Tiền số lớn/decimal và mã giữ chính xác dạng text; số nguyên tối đa15chữ số dùng int64, không có phép tính float. Không có công thức Excel dẫn xuất hay bản sao sổ. Chi tiết: [export-formats.md](export-formats.md).

## Nhật ký và khôi phục thao tác — schema8 (0.12.0)

`ledger.Write` mở audit_context sau kiểm tra quyền/epoch/idempotency; trigger chụp các dòng thực tế trước/sau trong cùng transaction. Tiền và giá trị SQLite được biểu diễn chuỗi; audit header/details chỉ hoàn tất cùng revision và command sau nghiệp vụ thành công. Savepoint preview không để lại lịch sử. Không lưu mật khẩu/hash, session hoặc token; chỉ ghi dấu có thay đổi thông tin đăng nhập. Nhật ký và snapshot đã commit không cho UPDATE/DELETE qua SQL thông thường. Bảo vệ này không thay thế kiểm soát filesystem của quản trị máy chủ.

Owner xem lịch sử qua `/audit/events` và `/audit/detail` có phân trang phía server, bộ lọc ngày Việt Nam/người/thao tác/tìm kiếm, tên người tại thời điểm thực hiện và nội dung trước/sau. `/audit` cũ giữ hợp đồng 200 sự kiện. Sự kiện cơ bản chưa có snapshot vẫn đọc được, không backfill giả dữ liệu.

`operations/audit.go` chỉ khôi phục tập thao tác được khai báo bằng các helper nghiệp vụ đang dùng: sửa nháp chứng từ/nghiệp vụ/đầu kỳ/điều chỉnh đầu kỳ, bản nháp BCTC và danh mục. Preview rollback toàn bộ; confirm kiểm tra lại current state, sự kiện mới hơn, revision và fingerprint, lý do bắt buộc, quyền owner và epoch. Khôi phục tạo nhật ký mới có restored_from/base_action để có thể đảo lại thao tác khôi phục. Nháp vừa tạo có thể bỏ và phục hồi cùng ID; tombstone version buộc bản phục hồi tăng version, ngăn client cũ ghi đè. Danh mục mới chỉ ngừng sử dụng. Chứng từ posted/đầu kỳ confirmed không sửa trực tiếp; giữ luồng đảo/điều chỉnh chuyên biệt. Chính sách, khóa kỳ, quyền và nhập hàng loạt không có hoàn tác tự động. Không tạo cơ chế restore SQL tùy ý.

## Tổng quan tài chính — 0.13.0

Metrics vẫn đọc cùng transaction của `ledger.Snapshot`, thêm moneyBalance/inventoryBalance/payable/unbalancedDraftCount dưới dạng chuỗi tiền hoặc số đếm. Money/inventory lấy tổng các dòng trực tiếp của nhóm tài khoản từ ledger; payable cộng dư Có dương từng đối tượng theo cùng cách tổng hợp của sổ công nợ. Nháp lệch tính bằng math/big trên toàn kỳ. Danh sách100chứng từ workspace tách opening/adjustment/reversal đầu kỳ giống bộ lọc Documents, không dùng làm nguồn tính tổng.

Dashboard dùng server metrics và chỉ có phép trình bày chuỗi/BigInt, demo giữ helper tính từ toàn bộ dữ liệu mẫu. Thiếu trường trên API cũ được thể hiện rõ; không lấy trang chứng từ để bù hoặc coi thiếu là0. Biểu đồ SVG và CSS Module riêng, không thêm thư viện biểu đồ/cache. Mã chứng từ dài được giới hạn trong ô; bảng gần đây chỉ hiển thị4dòng, link mở danh sách có phân trang toàn bộ.

## Danh mục hàng hóa và dịch vụ dùng chung (0.14.0)

Giữ một bảng items và ID chung cho mua/bán, với quy ước tracked=true cho hàng hóa quản lý kho và false cho dịch vụ; không thêm trường phân loại cạnh tranh hoặc migration. ItemEditor dùng chung ở danh mục và trong ItemSelect của phiếu. Lưu danh mục qua API hiện có, invalidate workspace rồi chọn phản hồi server vào dòng đang nhập; không reset hoặc lưu phiếu ngầm. Bộ lọc dùng metadata, không dựa vào tiền tố mã.

saveItemTx cho sửa tracked chỉ khi chưa có tham chiếu ở operations/voucher_inputs, lô/biến động kho hoặc payload tồn kho đầu kỳ/điều chỉnh. Kiểm tra và cập nhật trong cùng Ledger.Write, nên không đua với ghi sổ. Mã/đơn vị vẫn bất biến; audit trước/sau và khôi phục dùng validator này. Thay đổi không cần schema hoặc thay snapshot cũ.
