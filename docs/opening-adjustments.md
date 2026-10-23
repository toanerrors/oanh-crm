# Điều chỉnh số dư đã xác nhận

Triển khai0.8.0 theo yêu cầu23/10/2026. Đây là mô tả hợp đồng và kiểm tra kỹ thuật; chưa thay cho nghiệm thu kế toán doanh nghiệp.

## Invariant và cách sửa

- Người dùng nhập **số dư đúng sau điều chỉnh**, không phải tự tính chênh lệch. Mốc chuyển đổi giữ nguyên. Lý do bắt buộc. Nháp lưu bền, có version, tiếp tục sửa hoặc bỏ trước xác nhận.
- Số đã xác nhận không sửa/xóa. Xác nhận điều chỉnh tạo hai chứng từ trong cùng transaction: đảo toàn bộ phiên bản số dư đang dùng và ghi phiên bản số dư mới. Mỗi chứng từ giữ giới hạn200dòng/int64; không cộng tiền của hai chứng từ vào một tổng int64. Tổng chênh lệch dùng số nguyên lớn.
- Nguồn `opening_adjustment` và `opening_adjustment_reversal` đều thuộc dư đầu, không thuộc phát sinh/NKC/dòng tiền trong kỳ. Các báo cáo cộng cùng nguồn ledger theo account/partner; ngân hàng giữ bankId. Không có số dư song song trong frontend.
- Lưu root ban đầu, phiên bản trước/sau, liên kết cặp chứng từ, lý do, người/thời điểm xác nhận. Lịch sử bất biến, xem được từng bản và phân trang. Chỉ chủ sở hữu điều chỉnh; người có quyền đọc xem lịch sử.
- Save/confirm/discard có epoch/idempotency/version, audit và revision. Confirm phải kiểm tra lại baseVersion, draftVersion, revision preview và điều kiện trong transaction ghi. Lỗi giữa bất kỳ bước nào rollback cả cặp và phân hệ.

## Phân hệ và kỳ

- Kỳ chuyển đổi phải mở; không áp dụng hồi tố khi có kỳ đã khóa từ mốc trở đi. Nháp vẫn sửa/lưu được, preview chỉ rõ kỳ cần xử lý.
- Chỉ kiểm tra các chi tiết kho/tài sản thực sự đổi. Dòng tiền/công nợ vẫn sửa được khi kho/tài sản khác đã có phát sinh.
- Kho đổi khi chưa có xuất kho/biến động âm/điều chỉnh giá trị sau mốc cho mặt hàng–kho đó: giữ movement gốc, ghi movement âm trên chứng từ đảo, dương trên chứng từ mới; lô đầu kỳ cũ về0, tạo lô mới. Các lô mua khác giữ nguyên. Lô đầu kỳ ưu tiên trước lô mua cùng ngày trong FIFO. Chặn thay đổi kho đã có xuất, chỉ rõ chứng từ liên quan; không tự tính lại giá vốn đã chốt.
- Tài sản thay số tiền/thời gian/loại hoặc bỏ khi chưa khấu hao/phân bổ. Tên có thể sửa độc lập; mã/ID đã có giữ ổn định. Giữ bản gốc của opening_assets; truy vấn dùng snapshot hiệu lực cho elapsed/lũy kế đầu. Assets là trạng thái dẫn xuất hiện hành, khôi phục được từ snapshot và các phát sinh; tài sản bỏ khỏi snapshot không xuất hiện trong danh sách đang dùng. Không sửa nguyên giá ngầm sau khi đã khấu hao.
- Các blocker trả mã, thông báo và liên kết chứng từ/kỳ; không báo thành công một phần hoặc tự thêm dòng cân đối.

## Lưu trữ và API

Schema7 thêm opening_adjustments: document_id (bản mới), opening_document_id (root), before_document_id, reversal_document_id, base_version, confirmed_version, reason, before_payload/after_payload, created/confirmed actor+time. Một nháp đang mở/root, phiên bản xác nhận duy nhất/liên tiếp, trigger bất biến sau xác nhận. View opening_current chọn snapshot xác nhận mới nhất, fallback root; current_assets loại tài sản đầu kỳ không còn trong snapshot.

GET opening-balances trả bản hiệu lực, balanceVersion, nháp điều chỉnh nếu có. POST opening-balances/adjustments lưu nháp. GET adjustments/{id} trả đối chiếu trước/sau và blockers trên snapshot hiện tại. POST adjustments/confirm xác nhận với id/version/revision. POST adjustments/discard chỉ bỏ nháp. GET adjustments phân trang lịch sử, có bản đầu tiên.

UI: nút Điều chỉnh số dư đầu kỳ mở ngay bản có thể nhập; Lưu nháp, Xem thay đổi & xác nhận, Lịch sử. Review hiển thị tổng trước/sau, chi tiết và blocker, xác nhận cuối. Không tự thay số dư thật khi triển khai.

## Case nghiệm thu

1.1triệu→5triệu→3triệu: original giữ nguyên, nguồn sổ đầu kỳ/carry/debt/bank/BCTC đúng, phát sinh không tăng giả; phiên bản liên tiếp.
2.Tiền trên2^53 và int64max giữ chuỗi chính xác. Hai bên đối tượng/ngân hàng không bù trừ sai.
3.Nháp/version/idempotency/retry/stale epoch/revision, hai phiên ghi đồng thời, rollback trigger lỗi cuối.
4.Kho thay trước xuất, cùng ngày mua/FIFO, kho đã xuất chặn đúng dòng nhưng tiền vẫn đổi; tài sản thêm/sửa/bỏ trước khấu hao, tên sau khấu hao và số liệu bị chặn khi phụ thuộc.
5.Kỳ khóa, account/partner ngừng dùng cho đảo lịch sử nhưng bản mới vẫn kiểm tra hiệu lực; xác nhận không thể đi qua API ledger chung.
6.Migration6→7 giữ dữ liệu; backup/restore/reopen/check đối soát phiên bản gốc, lịch sử, lô tồn, tài sản và ngân hàng.


## Cách dùng

Tại Cân đối phát sinh chọn **Điều chỉnh số dư đầu kỳ**. Sửa số đúng, nhập lý do và chọn **Xem thay đổi & xác nhận** (tự lưu nháp trước). Đối chiếu hai bảng trước/sau và chênh lệch; xử lý cảnh báo nếu có rồi **Xác nhận điều chỉnh**. **Lưu nháp** cho phép đóng và tiếp tục sau tải lại; **Bỏ nháp** không thay số đã xác nhận. Nút **Lịch sử số dư đầu kỳ** xem gốc và từng phiên bản, lý do/người/thời gian.

Với phiếu thu/chi/chuyển nội bộ đã ghi sổ: **Sửa / thay thế phiếu** điền sẵn nội dung, chọn ngày sửa hợp lệ và lý do, tính/xem trước, ghi sổ. Phiếu cũ được đảo và phiếu mới được ghi cùng lần xác nhận; từ gốc có **Mở phiếu đã sửa**. Nháp đã lưu được tiếp tục đúng ID khi mở lại qua nút tạo mới.
