# Backup, chuyển dữ liệu và restore SQLite

Backup/restore thủ công đã triển khai qua Cài đặt và API. Mục 0 mô tả chức năng hiện tại; các mục sau là quy tắc và tiêu chí vận hành, gồm những kiểm thử chưa hoàn tất.

## 0. Runbook hiện có

1. Đăng nhập bằng chủ sở hữu, mở **Cài đặt → Backup / khôi phục → Tạo và tải backup**. Server tạo snapshot bằng `VACUUM INTO`, fsync file, kiểm tra schema/checksum/integrity/FK/cân đối và khớp kho/tài sản với sổ; đóng ZIP và đọc kiểm tra lại trước khi trả file.
2. Gói ZIP chứa `manifest.json`, `accounting.sqlite` và các blob `attachments/<sha256>` được snapshot tham chiếu. Lưu thêm bản sao trên thiết bị độc lập. Gói có dữ liệu lương, tài khoản và hash mật khẩu; giữ quyền truy cập tương đương dữ liệu nguồn. Chưa có mã hóa archive trong ứng dụng.
3. Để chuyển máy, chạy cùng phiên bản ứng dụng trên máy đích, khởi tạo tài khoản chủ sở hữu và chọn **Kiểm tra gói khôi phục**. Upload tối đa 128 MiB, tổng giải nén tối đa 256 MiB; từ chối đường dẫn thoát thư mục, symlink, tên trùng, schema khác, thiếu/sai checksum hoặc đối soát không đạt.
4. Rà soát tên doanh nghiệp, thời điểm snapshot, số chứng từ và tổng Nợ/Có. Chỉ bấm **Khôi phục từ gói đã kiểm tra** khi muốn thay dataset đang dùng. Server kiểm tra lại file staging, tạo backup hiện tại, chuẩn bị dataset mới và đổi epoch, thu hồi phiên, đóng toàn bộ connection cũ trước khi chuyển con trỏ.
5. Sau khi thành công, đăng nhập bằng tài khoản có trong gói khôi phục. Tài khoản chỉ tồn tại ở máy đích trước restore không được chuyển vào gói. Kiểm tra sổ, kho, tài sản và file đính kèm. Client cũ bị từ chối bởi epoch hoặc phiên.

Thư mục dữ liệu:

```text
data/
├── active-dataset.json
├── application.lock
├── datasets/<id>/accounting.sqlite
├── datasets/<id>/attachments/<sha256>
├── backups/<id>.zip
├── staging/<id>/...
└── recovery/<id>.json
```

Khởi tạo mới chỉ công bố con trỏ sau khi database được tạo và kiểm tra. Khi con trỏ đã có nhưng DB bị thiếu/hỏng, ứng dụng dừng khởi động; không tạo database rỗng thay thế. Khôi phục giữ nguyên dataset cũ và ghi nhật ký chuẩn bị/kích hoạt/rollback ngoài dataset; nếu không mở được dataset mới thì thử quay lại con trỏ cũ. Giữ cả gói backup trước restore và thư mục dữ liệu cho đến khi đối soát hoàn tất.

Đã kiểm tra bằng SQLite file tạm: backup có file đính kèm, restore đúng snapshot, dữ liệu sau snapshot không còn, epoch đổi, lệnh cũ bị chặn; archive traversal và staging bị sửa bị từ chối. E2E kiểm tra upload/xem trước/xác nhận/đăng nhập lại và không còn dữ liệu/cache sau snapshot. Chưa thử cắt điện/hết đĩa tại từng điểm chuyển, rollback do lỗi filesystem giữa các bước, restore drill định kỳ hoặc RPO/RTO trên máy mục tiêu. Chưa có lịch backup, retention và dọn file staging/blob mồ côi tự động; cần quản lý dung lượng vận hành.

## 1. Giới hạn vận hành

- DB đặt trên filesystem cục bộ; một tiến trình được quyền ghi. Không đặt DB đang mở trong thư mục NFS/SMB hoặc đồng bộ cloud.
- WAL là một phần trạng thái của DB đang hoạt động. Không backup bằng cách copy riêng `.sqlite`/`.db`, không xóa `-wal` hoặc `-shm` thủ công để làm file có vẻ sạch. [SQLite: WAL file](https://sqlite.org/wal.html#the_wal_file).
- Snapshot DB dùng Online Backup API qua driver adapter; `VACUUM INTO` là phương án có thể cân nhắc khi kiểm tra đúng semantics và durability. Cả hai cần xử lý lỗi, hoàn tất ghi bền vững và kiểm tra output trước khi công bố backup. [SQLite Online Backup](https://sqlite.org/backup.html), [VACUUM INTO](https://sqlite.org/lang_vacuum.html#vacuum_with_an_into_clause).
- Backup checksum xác minh byte và phát hiện hỏng dữ liệu; không thay cho xác thực nguồn hoặc mã hóa. SQLite thông thường không tự mã hóa file backup.

## 2. Định dạng gói backup

Một gói logic gồm:

```text
backup-<id>/
├── manifest.json
├── accounting.sqlite
└── attachments/<content-hash>...
```

Manifest tối thiểu có format version, app/schema version và migration checksum, SQLite version, company ID, dataset ID/epoch tại thời điểm snapshot, thời điểm tạo UTC, file hash/size, số lượng bản ghi kiểm tra và các tổng đối soát chính xác tính từ snapshot. Không ghi mật khẩu, khóa giải mã hoặc secret ngoài nhu cầu khôi phục đã được thiết kế.

DB snapshot có thể chứa session hoặc thông tin xác thực thuộc bộ dữ liệu. Gói backup phải được kiểm soát quyền như dữ liệu nguồn; sau restore phải vô hiệu hóa phiên cũ theo chính sách kích hoạt dataset mới.

## 3. Quy tắc attachment trước khi backup

- Blob lưu bất biến theo content hash, tên hiển thị nằm trong DB. Không sửa nội dung blob đã được tham chiếu.
- Trước khi commit một tham chiếu mới: ghi file tạm, kiểm tra hash/kích thước, đồng bộ dữ liệu theo nền tảng, rename vào vị trí cuối và bảo đảm độ bền thư mục theo nền tảng; sau đó commit DB. File mồ côi có thể được dọn sau; không để bản ghi đã commit trỏ đến file chưa bền vững.
- Trong thời gian tạo backup, tạm hoãn garbage collection của blob để các file mà snapshot tham chiếu không bị xóa. Không cần giữ write transaction trong toàn bộ thời gian sao chép.

## 4. Luồng backup

1. Kiểm tra quyền, disk space và trạng thái job; tạo thư mục staging với tên do server sinh, chưa hiển thị là backup hoàn tất.
2. Tạo snapshot DB nhất quán bằng API được chọn; hoàn tất/đóng destination đúng tài liệu driver. Có deadline và xử lý busy/retry có giới hạn.
3. Mở snapshot riêng, chạy `PRAGMA integrity_check` và `PRAGMA foreign_key_check`; xác minh schema, đếm bản ghi và tính tổng kiểm tra từ chính snapshot.
4. Liệt kê attachment từ snapshot rồi sao chép blob bất biến tương ứng. File thiếu hoặc hash sai làm backup thất bại; không bỏ qua.
5. Tạo manifest, xác minh lại file hash/size và độ bền output. Nếu dùng archive, đóng archive thành công và đọc kiểm tra trước khi đánh dấu hoàn tất.
6. Công bố gói bằng thao tác atomic thích hợp trên cùng filesystem; ghi trạng thái job/audit. Chỉ sau đó mới áp retention cho các backup cũ.
7. Tạo bản sao trên thiết bị/vị trí độc lập với ổ chứa DB theo chính sách đã chốt; mã hóa khi cần và quản lý khóa tách khỏi archive.

Backup lưu ngay trên cùng ổ hỗ trợ thao tác restore, nhưng không đủ để khôi phục khi mất ổ đó. Lịch backup và số bản giữ phải theo RPO/dung lượng thực tế, không tự coi một lịch cố định là đáp ứng mọi doanh nghiệp.

## 5. Luồng restore và chuyển máy

Restore ghi đè dữ liệu đang dùng là thao tác cần được ủy quyền cho đúng gói và đúng môi trường. Việc chuẩn bị, xác minh gói và dry run trên dataset staging có thể làm trước.

1. Nhận gói vào staging; kiểm tra nguồn, format, kích thước, file path, symlink và archive path traversal. Không mở archive vào thư mục dữ liệu đang hoạt động.
2. Xác minh hash/size, phiên bản và mọi attachment. Từ chối schema mới hơn phiên bản ứng dụng có thể đọc. Nếu cần nâng schema cũ, migration trên bản sao staging sau khi giữ nguyên gói gốc.
3. Kiểm tra DB bằng `integrity_check`, `foreign_key_check` và đối soát nghiệp vụ: cân đối Nợ/Có, số dư phân hệ, tổng/checksum nghiệp vụ trong manifest. Hash byte của DB có thể đổi sau migration; giữ bằng chứng và so sánh tổng logic thích hợp.
4. Hiển thị kết quả dry run: doanh nghiệp, thời điểm snapshot, kỳ, bản ghi, attachment và dữ liệu sẽ thay. Chỉ kích hoạt sau khi có ủy quyền restore phù hợp.
5. Vào maintenance mode; chặn request ghi mới, dừng worker và đợi request/job đang chạy kết thúc. Tạo và kiểm tra backup của dataset hiện tại trước khi thay; nếu không tạo được thì không tiếp tục đường restore thông thường.
6. Đóng toàn bộ read/write connection và file handle của dataset cũ. Chuẩn bị dataset mới trong thư mục riêng; không trộn DB mới với sidecar WAL của dataset cũ.
7. Đặt `dataset_epoch` mới và vô hiệu hóa phiên/lease công việc cũ theo thiết kế. Trong maintenance, cập nhật con trỏ active dataset theo quy trình atomic/durable được kiểm tra trên hệ điều hành đích. Giữ nguyên dataset cũ để quay lại.
8. Mở lại DB mới bằng cấu hình chuẩn; kiểm tra metadata, schema và các đối soát sau mở. Nếu thất bại, giữ maintenance và quay lại dataset cũ; không mở traffic trên dữ liệu chưa xác minh.
9. Cho client đăng nhập/tải lại context khi cần, hủy cache và mutation thuộc epoch cũ. Khi hợp lệ mới mở ghi và khởi động worker với lease của dataset mới.
10. Ghi audit kích hoạt/rollback ở nơi còn tồn tại qua thao tác thay dataset; kiểm tra bằng thử nghiệm sau crash giữa các bước chuyển đổi.

`Idempotency-Key` không thay thế dataset epoch. Một tab mở trước restore có thể mang dữ liệu/khóa đã mất sau khi quay về snapshot cũ; server phải từ chối lệnh gửi bằng epoch cũ thay vì ghi lại trên dữ liệu mới.

## 6. Tiêu chí nghiệm thu recovery

- Backup trong lúc ứng dụng có giao dịch ghi; restore sang thư mục/máy tạm cho số liệu và attachment khớp snapshot.
- Gói lỗi, file thiếu, checksum sai, schema không tương thích, zip path traversal hoặc hết đĩa đều bị từ chối và không làm thay đổi dataset đang dùng.
- Mất tiến trình giữa các bước không để con trỏ active trỏ đến dataset chưa xác minh; hệ thống khởi động lại xác định được dataset hợp lệ hoặc giữ maintenance.
- Đường rollback về dataset cũ chạy được; các handle cũ đã đóng, không còn worker cũ ghi vào dataset mới.
- Test `integrity_check` và `foreign_key_check` không được thay cho kiểm tra cân đối sổ và khớp phân hệ.
- Có restore drill định kỳ và sau thay đổi schema/driver quan trọng. Ghi thời gian backup/restore, kích thước, thời điểm bản sao hợp lệ gần nhất và kết quả kiểm tra.
- RPO/RTO và retention vẫn là quyết định cần xác nhận; chưa có đo đạc thì không tuyên bố đáp ứng mục tiêu khôi phục.

## Tương thích schema 1 và 2

Release 0.3.0 thêm migration 2 có checksum bao gồm SQL và bộ tài khoản tham chiếu. `Check` chấp nhận đúng fingerprint/checksum của schema 1 hoặc 2. Gói backup schema 1 được kiểm tra ở schema gốc; khi activate, bản dataset chuẩn bị được migration lên schema mới **trước** khi đổi pointer. Gói nguồn vẫn nguyên vẹn. Migration giữ tài khoản/bút toán cũ và đưa tồn kho cũ vào Kho mặc định; epoch và phiên đăng nhập được đổi theo quy trình restore hiện có.

Kiểm tra bổ sung: ngân hàng khớp ledger, biến động và lô theo từng hàng–kho, nguồn chi phí không bị phân bổ vượt/trùng/sai ngày và tổng phân bổ bằng tổng nguồn. Nháp chỉ có dòng ledger nháp và payload, không có biến động phân hệ. Chứng từ đã post không được sửa payload nháp hoặc biến động ngân hàng/kho.

Trước nâng production lên schema 2 phải có backup đã xác minh. Có thể dừng container, sao chép toàn bộ `/data` khi writer đã đóng, kiểm tra SQLite bản sao chỉ đọc, đóng gói và checksum. Lưu ngoài repository, quyền riêng tư; không ghi DB/attachments vào Git. Binary 0.2.0 không đọc schema 2; rollback dữ liệu cần runbook và xác nhận, không chép đè volume tùy tiện.


## Schema3 — bản nháp BCTC

Backup bao gồm bảng `financial_reports` với input/version/ngày/người cập nhật. Migration3 chỉ thêm bảng/index, không đổi tiền hoặc bút toán. Đã bổ sung kiểm thử migration1/2→3 và backup/restore bản nháp có tiền trên2^53, giữ version/nội dung, đổi epoch và chặn lệnh phiên cũ. Template `tt99-2026-v1` phải giữ khả năng đọc lại, không sửa định nghĩa đã phát hành.

### Schema4 — số dư đầu kỳ

Thêm metadata chuyển đổi và lịch sử phân bổ tài sản trước chuyển đổi. Backup/Check đối chiếu số dư với ledger, chi tiết ngân hàng, stock movement và tài sản; chặn lịch sử giao nhau với mốc. Giữ nguyên hash/checksum migration1–3, nhận gói1/2/3/4 và chỉ nâng bản chuẩn bị. Đã kiểm tra migration/backup/restore từ schema3 có BCTC cũ, số dư đã xác nhận và lô tồn/tài sản sau phát sinh. Image trước0.5.0 không đọc schema4; không rollback binary cũ trực tiếp trên volume đã nâng.

## Schema5 — vùng chờ nhập Excel

Backup chứa bảng `excel_imports`: dữ liệu đã đọc từ tệp, cấu hình ghép cột, hash, actor và kết quả nhập. Không cần file tạm của upload để phục hồi. Tệp riêng theo actor; owner bị hạ quyền không đọc lại tệp đã tải với quyền owner. Sau restore, tệp chờ của epoch cũ bị chặn và phải tải lại; lịch sử đã nhập vẫn đọc theo phiên/epoch hiện hành, không cho nhập lại. Không lưu tệp tài chính vào Git hoặc trình duyệt.

Ứng dụng đọc/chuẩn bị nâng backup schema1–4 và nhận schema5; checksum/schema fingerprint lịch sử giữ nguyên. Image0.5.x không đọc schema5. Release0.6.0 phải có backup nguội toàn bộ volume và kiểm tra trên bản sao trước migration; không hạ schema hoặc chạy binary cũ trên volume đã nâng. Kiểm thử backup/restore/reopen xác minh số tiền lớn và nguồn nhập vẫn được giữ nguyên.
