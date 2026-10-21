# Hướng dẫn backend Go

Áp dụng cùng `../AGENTS.md`. Đọc các quy tắc số liệu và backup khi nhiệm vụ tác động dữ liệu.

## Tổ chức mã

- Một Go module trong `server/`; entry point tại `cmd/server/`. Dependency được nối tường minh ở phần khởi tạo ứng dụng.
- Module nghiệp vụ trong `internal/modules/<module>/`, gồm model/quy tắc, service, repository interface và handler khi thực sự cần.
- SQLite adapter của module nằm trong module đó. Phần kết nối, migration và transaction dùng chung ở `internal/platform/sqlite/`.
- `internal/accounting/` dành cho value type và quy tắc tính chính xác được nhiều module dùng. Không biến thành package `utils` chứa mọi thứ.
- Module `ledger` sở hữu bút toán đã ghi sổ và các truy vấn sổ. Phân hệ khác gọi service/port công khai; không sửa bảng ledger từ handler hoặc repository riêng.
- Interface nhỏ đặt tại nơi sử dụng. Không tạo một interface cho mỗi struct hoặc một generic repository cho toàn hệ thống.
- Handler decode/validate DTO và mapping lỗi HTTP. Service xử lý nghiệp vụ và transaction. Repository thực thi SQL; không quyết định chính sách thuế hoặc tự commit.

## Tiền, ngày và lỗi

- Tạo `Money`, `Quantity`, `UnitPrice`, `Rate` hoặc kiểu tương đương có constructor kiểm tra. Kiểu amount luôn biết currency/scale của mình.
- Ưu tiên `math/big.Int`/`math/big.Rat` cho phép tính trung gian chính xác nếu chưa chấp thuận dependency decimal. Bọc kiểu mutable của `math/big` để không chia sẻ buffer gây sửa giá trị ngoài ý muốn.
- Không tạo giá trị chính xác từ float. Parser chỉ nhận chuỗi chuẩn, kiểm tra dấu, scale, độ dài và range trước khi tính; giới hạn đầu vào để tránh phép tính quá lớn.
- Ghi DB bằng `int64` đã kiểm tra hoặc chuỗi thập phân đã chuẩn hóa. Không bind `json.Number` qua `float64` hoặc scan số tiền vào `any` rồi ép kiểu tùy tiện.
- Không dùng `TOTAL()`, `AVG()` hoặc `CAST AS REAL` cho báo cáo tài chính. `SUM()` chỉ trên cột INTEGER và xử lý overflow; các tổng vượt giới hạn dùng phép cộng chính xác có kiểm tra ngoài SQL.
- Không dùng `time.Time` mang timezone để thay thế ngầm ngày chứng từ. Phân biệt ngày lịch và timestamp audit.
- Trả lỗi có mã ổn định như `UNBALANCED_ENTRY`, `PERIOD_CLOSED`, `VERSION_CONFLICT`, `IDEMPOTENCY_CONFLICT`, `DATASET_CHANGED`, `AMOUNT_OVERFLOW`; kèm field/message khi phù hợp.
- Wrap lỗi nội bộ bằng `%w`, không trả SQL/path/secret cho client; không nuốt lỗi commit hoặc biến lỗi thành số 0.

## Transaction và SQLite

- Transaction do application service sở hữu; truyền transaction context/handle tường minh cho các repository tham gia. Không mở transaction lồng nhau ngầm.
- Sau `BeginTx`, các thao tác thuộc nghiệp vụ dùng đúng `sql.Tx`; không gọi ngược `sql.DB` cho cùng transaction. Dùng `defer Rollback`, chỉ trả thành công sau `Commit`.
- Một write pool với `SetMaxOpenConns(1)`; read pool giới hạn riêng. Giới hạn idle connection phù hợp, không phát sinh connection mới ngoài cấu hình chuẩn.
- Cấu hình mọi connection bằng DSN/hook được driver hỗ trợ và xác minh; kiểm tra `foreign_keys`, `synchronous`, timeout trên connection thực tế.
- Nếu cần `BEGIN IMMEDIATE`, chọn cách cấu hình được driver tài liệu hóa; `BeginTx` không tự bảo đảm chế độ này. Không trộn `BEGIN` thủ công với `sql.Tx`.
- Đọc kiểm tra kỳ, version, quyền và idempotency trong transaction ghi; số chứng từ cấp bằng counter và UNIQUE constraint có phạm vi rõ ràng, không dùng `MAX()+1` ngoài transaction.
- Retry `SQLITE_BUSY` có giới hạn và deadline; rollback rồi retry toàn transaction an toàn/idempotent. Không lặp vô hạn hoặc retry từng câu SQL làm mất ý nghĩa nghiệp vụ.
- Kết thúc `Rows`/statement, xử lý `rows.Err()`; tránh giữ read transaction lâu gây phình WAL.
- Migration chạy tuần tự trước khi nhận traffic; không tự reset DB khi migration thất bại. Kết nối/migration lỗi thì dừng khởi động ghi dữ liệu.

## HTTP và vận hành

- REST JSON dưới `/api/v1`; context/request timeout, giới hạn body, graceful shutdown, structured logging có request ID.
- Tham số SQL phải bind. Sort/field động qua allowlist; không nối đầu vào người dùng vào SQL.
- Phân quyền doanh nghiệp/phân hệ và chứng từ ở server. UI ẩn nút không phải biện pháp kiểm soát quyền.
- Job dài lưu trạng thái bền vững và có khả năng chạy lại; transaction nghiệp vụ tạo job/outbox khi cần. Không chỉ chạy goroutine rồi coi là đã lưu công việc.
- Import và báo cáo có giới hạn hàng/kích thước/thời gian. Không giữ write transaction trong lúc parse Excel, render PDF hoặc upload.
- Export giữ số tiền lớn và mã định danh chính xác; chống formula injection khi xuất dữ liệu người dùng sang CSV/Excel.
- Đọc `docs/backup-restore.md` trước khi làm bất kỳ chức năng backup/restore; giữ phần phụ thuộc API riêng của SQLite driver trong adapter backup.

## Kiểm tra khi có mã nguồn

- Unit test quy tắc tiền và nghiệp vụ; integration test trên SQLite file tạm, WAL và cấu hình giống production.
- Dùng file DB thật cho test reopen/backup/restore/concurrency; không chỉ dùng `:memory:`. Không mock đường ghi sổ quan trọng để thay cho integration test.
- Chạy `gofmt` trên file sửa; `go test` và `go vet` cho package ảnh hưởng; dùng `go test -race` cho logic cạnh tranh khi môi trường hỗ trợ.
- Case bắt buộc: rollback khi lỗi giữa chừng, double-submit, cùng key khác payload, cấp số cạnh tranh, khóa kỳ cạnh tranh với ghi sổ, stale version, amount overflow và đọc lại sau restart.
- Chưa có `go.mod` thì chưa có lệnh test backend chạy được; báo đúng giới hạn này.
