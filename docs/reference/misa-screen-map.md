# Đối chiếu màn hình AMIS — 22/10/2026

Đã mở bằng Edge các URL người dùng cung cấp, xem bố cục và thao tác mở biểu mẫu; không cất hoặc ghi sổ dữ liệu trên MISA. Dữ liệu kiểm thử của Oanh nằm trong dataset tạm riêng. Bảng này mô tả phạm vi đã triển khai, không khẳng định tương đương toàn bộ sản phẩm AMIS.

| Màn hình tham chiếu | Màn hình/chức năng Oanh |
|---|---|
| [CAReceiptPaymentList](https://actapp.misa.vn/app/CA/CAReceipt/CAReceiptPaymentList) | `/cash`: dư đầu/thu/chi/dư cuối, lọc/trạng thái/phân trang/xuất, dòng đang chọn và chi tiết bên dưới |
| [CAReceiptDetail](https://actapp.misa.vn/app/popup/CAReceiptDetail) | `/vouchers/new?kind=receipt&account=111`: dùng chung phiếu thu/chi; nhiều dòng, đối tượng, công nợ, metadata, nháp, xem trước, ghi sổ, in/đính kèm |
| [BADepositWithdrawList](https://actapp.misa.vn/app/BA/BADepositWithdraw/BADepositWithdrawList) | `/bank`: danh sách, lọc theo số tài khoản và tổng tiền riêng từng ngân hàng |
| [BADepositDetail](https://actapp.misa.vn/app/popup/BADepositDetail) | Thu/chi tiền gửi dùng cùng biểu mẫu; chọn số tài khoản và suy ra TK kế toán |
| [BAInternalTransfer](https://actapp.misa.vn/app/popup/BAInternalTransfer) | Chuyển nội bộ, cả hai số tài khoản khác nhau cùng TK112; ghi nhận hai biến động cùng transaction |
| [PUVoucherManager](https://actapp.misa.vn/app/PU/PUVoucher/PUVoucherManager) | `/purchases`: hàng hóa, dịch vụ, thanh toán và truy chứng từ |
| [PUVoucherDetail](https://actapp.misa.vn/app/popup/PUVoucherDetail) | Mua trong nước nhập kho: nhiều hàng/kho, VAT, chiết khấu, chi phí liên kết nguồn, chưa/thanh toán ngay |
| [PUServiceDetail](https://actapp.misa.vn/app/popup/PUServiceDetail) | Mua dịch vụ, chọn TK chi phí/dở dang, xác nhận khấu trừ VAT; đánh dấu nguồn chi phí mua hàng |
| [SAVoucher](https://actapp.misa.vn/app/SA/SAVoucher) | `/sales`: hàng hóa/dịch vụ, công nợ, tra cứu/xuất |
| [SAVoucherDetail](https://actapp.misa.vn/app/popup/SAVoucherDetail) | Bán hàng: xuất kho/giá vốn theo kho, chiết khấu/VAT, chưa/thu tiền ngay |
| [SAServiceDetail](https://actapp.misa.vn/app/popup/SAServiceDetail) | Bán dịch vụ, nhiều dòng, TK doanh thu, VAT/chiết khấu, phiếu thu liên kết |
| [GLVoucher](https://actapp.misa.vn/app/GL/GLVoucher) | `/documents`: danh sách và chứng từ nhiều dòng Nợ/Có |
| [ProfitAndLossTransfer](https://actapp.misa.vn/app/GL/ProfitAndLossTransfer) | `/documents?source=closing`: lọc nguồn kết chuyển |
| [GLVoucherDetail](https://actapp.misa.vn/app/popup/GLVoucherDetail) | Biểu mẫu tổng hợp toàn màn hình; giữ các dòng Nợ/Có độc lập để hỗ trợ nhiều bên |
| [ProfitAndLossTransferDetail](https://actapp.misa.vn/app/popup/ProfitAndLossTransferDetail) | `/vouchers/new?kind=closing`: xem trước, ghi sổ/khóa kỳ; mở lại bằng luồng đảo hiện có |

## Tài khoản tham chiếu

Nguồn: [Google Sheets người dùng chỉ định](https://docs.google.com/spreadsheets/d/1SGGiB8oowT89Pi0GPwvNoo-NDT_E4fl_B7gx__VW-ds/edit?gid=0#gid=0). Có 184 mã tài khoản, lưu tại `server/internal/accounting/chart.json` với URL, hash CSV gốc, dòng nguồn, phiên bản `user-sheet-2026-10-22` và mốc tham chiếu 01/01/2026. Mốc này là metadata phiên bản nhập, không tự thay chế độ/chính sách doanh nghiệp hoặc số liệu trước đó. Chuẩn hóa cha/con bị lệch cột trong sheet; giữ nguyên tên nguồn, gồm lỗi gõ. Migration và thao tác áp dụng chỉ thêm mã còn thiếu, không đổi tài khoản/lịch sử đã có. 1121 cũ được giữ để tương thích; dữ liệu mới chọn TK112 hoặc tài khoản tiền gửi đã khai báo.

621/622/623/627 và các tài khoản con được phân nhóm chi phí chờ tập hợp, tránh đưa thẳng vào 911. Mapping cha/con là dữ liệu tham chiếu, chưa thay cho cây tài khoản/báo cáo có phiên bản pháp định.

## Quy tắc tham khảo và giới hạn

- Chiết khấu: theo mặt hàng, theo % hóa đơn hoặc theo số tiền tổng hóa đơn; phân bổ theo số lượng, giá trị hoặc tự nhập. Nguồn: [hướng dẫn bán hàng](https://helpact.misa.vn/kb/nhap-lieu-nhu-the-nao-voi-cac-loai-chiet-khau-tren-bao-gia-don-dat-hang-chung-tu-ban-hang/), [hướng dẫn mua hàng](https://helpact.misa.vn/kb/html_14030500/). Chiết khấu cùng hóa đơn giảm giá tính thuế; giá nhập đã gồm VAT thì tiền chiết khấu cũng gồm VAT. Chưa có chứng từ giảm giá/chiết khấu sau hóa đơn hoặc hàng tặng có tổng thanh toán bằng 0.
- Chi phí mua hàng: chọn dòng dịch vụ đã ghi sổ làm nguồn, giới hạn số còn lại, rồi phân bổ vào hàng nhập theo số lượng, giá trị sau CK trước VAT, tỷ lệ tự nhập tổng 100% hoặc giá trị tự nhập. Nguồn: [hướng dẫn phân bổ AMIS](https://helpact.misa.vn/kb/html_14030600/). Chuyển giá trị từ TK của nguồn vào kho; không tạo công nợ/VAT lần hai. Không cho kết chuyển khi còn nguồn chi phí chưa phân bổ hết. Chưa xử lý điều chỉnh chi phí hồi tố cho hàng đã xuất hoặc hoàn nguyên phân bổ phân hệ.
- Tài liệu AMIS trên không đặc tả đầy đủ cách xử lý phần lẻ đồng. Oanh công khai quy tắc riêng: lấy phần nguyên, cấp đồng còn dư theo phần lẻ lớn nhất; bằng nhau theo thứ tự dòng. Tổng phân bổ luôn đúng tổng nguồn. Làm tròn giá/VAT tiếp tục theo policy doanh nghiệp đã xác nhận; không mặc định tham số pháp lý từ AMIS.
- Nhiều kho: nhập/xuất trực tiếp từ kho của dòng hàng, FIFO/bình quân độc lập từng hàng–kho; dữ liệu cũ thuộc Kho mặc định. Chưa có điều chuyển giữa kho, kiểm kê hoặc quy đổi đơn vị.
- Giao diện dùng bố cục/mật độ tương tự, thương hiệu Oanh và Ant Design. Inter v4.1 tự lưu trữ từ [nguồn chính thức](https://rsms.me/inter/) theo OFL 1.1; không sao chép logo, hình hoặc mã nguồn riêng của MISA.
- Chưa tích hợp phát hành hóa đơn điện tử, ngân hàng điện tử, nhập khẩu/hải quan/ngoại tệ, hợp đồng/đơn hàng có vòng đời riêng, BCTC/tờ khai pháp định hoặc mọi menu trong sản phẩm MISA. Trường số hợp đồng/đơn hàng hiện chỉ là tham chiếu.


## BCTC bổ sung ngày22/10/2026

Đã mở `CreateFinancialReportDetail` và luồng chọn kỳ trong Edge: danh sách năm/người lập/ngày/trạng thái; popup chọn B01/B02/B03, chỉ một phương pháp LCTT; chi tiết chỉ tiêu/mã/thuyết minh/hai cột, người ký và thao tác cuối. Mẫu doanh nghiệp AMIS đang xem hiển thị cấu trúc TT200; không sao chép mã cũ sang TT99.

Oanh thay tab income bằng financial, có mẫu TT99 năm hoạt động liên tục, công thức tổng và nhập tay, source sổ theo khoảng, lưu/version/kiểm tra/in/CSV. Chưa có tự động ánh xạ tài khoản, cấu hình công thức người dùng, B09 đầy đủ, XML hoặc gửi báo cáo. Nguồn pháp định/giới hạn tại `docs/accounting-correctness.md` mục12.
