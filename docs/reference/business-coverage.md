# Đối chiếu nghiệp vụ từ ảnh tham khảo

Ngày đối chiếu: 21/10/2026. Ảnh là tham chiếu chức năng, không phải nguồn tham số kế toán hay yêu cầu tích hợp một nhà cung cấp cụ thể. Có màn hình hoặc bút toán tổng hợp không đồng nghĩa đã có đầy đủ quy trình phân hệ.

| Nhóm nghiệp vụ trong ảnh | Hiện trạng có mã nguồn | Phần còn thiếu |
|---|---|---|
| Tiền mặt — thu, chi | Phiếu thu/chi tự động, xem trước, ghi sổ, lọc theo TK111; dư đầu, thu, chi và dư cuối kỳ từ cùng snapshot; xuất/in và đính kèm chứng từ | Kiểm kê theo mệnh giá, biên bản/chênh lệch kiểm kê và quy trình xử lý chênh lệch |
| Tiền gửi — thu, chi | Màn hình riêng TK1121; phiếu thu/chi và chuyển tiền nội bộ; số dư/phát sinh kỳ; đảo phiếu tiền có liên kết | Danh mục và sổ theo từng số tài khoản/ngân hàng; phí, ngoại tệ, lãi và tiền đang chuyển cần đặc tả riêng |
| Thu/trả tiền từ mua/bán | Phân bổ một phiếu cho nhiều chứng từ, thanh toán từng phần, chặn vượt/sai đối tượng/sai loại, trả lại công nợ khi đảo phiếu; hỗ trợ nguồn ghi tăng tài sản/242 có 331 | Gắn phân bổ cho các phiếu thủ công cũ; ứng trước, bù trừ, chiết khấu thanh toán và chênh lệch cần quy tắc riêng |
| Sao kê và đối chiếu ngân hàng | Có import CSV chứng từ qua kiểm tra và xem trước; có đính kèm bằng chứng | Chưa có parser sao kê, chống trùng giao dịch ngân hàng, ghép từng dòng sao kê, hồ sơ đối chiếu và hạch toán từ sao kê |
| Ngân hàng điện tử | Chưa có adapter | Cần xác định ngân hàng, hợp đồng API và quyền truy cập; chưa được coi là chỉ cần nhập thông tin cấu hình |
| Dự báo dòng tiền | Đã khai báo được hạn thanh toán và nhận diện chứng từ quá hạn | Chưa có dự báo theo ngày/kịch bản, kế hoạch chi hoặc đối chiếu dự báo với thực tế |
| Mua hàng/dịch vụ | Tính VAT/điều kiện khấu trừ; nhập kho hoặc chi phí dịch vụ; thông tin hóa đơn/hạn thanh toán; danh sách chứng từ và công nợ | Chi phí mua hàng phân bổ; nhận hàng và nhận hóa đơn ở các thời điểm khác nhau |
| Mua chi phí trả trước — ví dụ thuê văn phòng 242 | Luồng ghi tăng khoản phân bổ → 242/331, thanh toán nguồn đó qua phiếu tiền, phân bổ đường thẳng theo kỳ; không ghi nhận thêm chứng từ mua thứ hai | Chưa gộp VAT khấu trừ và ghi tăng nhiều khoản phân bổ vào cùng một chứng từ mua |
| Đơn mua hàng và hợp đồng mua | Chưa có quy trình riêng | Cần vòng đời phê duyệt/hủy, hạn mức, nhận hàng từng phần và liên kết chứng từ mua |
| Trả lại/giảm giá hàng mua | Chưa có nghiệp vụ riêng | Cần nguồn hóa đơn/dòng hàng, định giá trả lại, ảnh hưởng tồn kho/VAT/công nợ và xử lý hồi tố |
| Xử lý hóa đơn đầu vào | Có số/ký hiệu/ngày và kiểm tra trùng trong luồng tự động | Chưa có lấy hóa đơn điện tử, xác minh, đối chiếu hồ sơ và quy trình điều chỉnh/thay thế |
| Bán hàng dịch vụ/hàng hóa | Tính doanh thu, VAT, xuất kho/giá vốn; tra cứu nguồn; hóa đơn/hạn thanh toán; thu tiền theo chứng từ | Chưa phát hành hóa đơn điện tử, quản lý trạng thái phát hành hoặc điều chỉnh/trả lại/giảm giá bán |
| Tổng hợp — chứng từ nghiệp vụ khác | Nháp nhiều dòng, version, ghi sổ, chống trùng nguồn, đảo chứng từ thủ công, sổ NKC/sổ cái | Chưa có quy trình đề nghị/quyết toán tạm ứng nhân viên và phê duyệt |
| Kết chuyển lãi lỗ và khóa sổ | Kết chuyển qua 911/421, khóa kỳ; mở lại có đảo kết chuyển liên kết; điều hướng trực tiếp từ Kế toán tổng hợp | Chưa khóa riêng từng loại chứng từ hoặc lập lịch tự khóa |
| Báo cáo | Sổ chi tiết/sổ cái, NKC, cân đối phát sinh, công nợ hai bên theo đối tượng, doanh thu/chi phí; CSV và in | Chưa có công nợ nhân viên riêng, hồ sơ BCTC pháp định/B01a-DN/lưu chuyển tiền tệ và mapping được thẩm định |
| Tiện ích/gợi ý AI | Gợi ý danh mục và diễn giải từ dữ liệu có sẵn | Chưa có trợ lý AI, nhắc hạn nghiệp vụ, OCR hoặc tự phê duyệt; không coi các khối quảng cáo trong ảnh là chức năng đã có |

## Kiểm soát mới của luồng tiền

- Số tiền nguyên VND dương; tài khoản tiền 111/1121. Tiền/decimal qua API là chuỗi. Không tự thêm dòng cân đối, phí hoặc chênh lệch.
- Thanh toán đúng chứng từ nguồn, cùng đối tượng và loại công nợ, không vượt phần chưa phân bổ; tổng các phần bằng số tiền phiếu. Kiểm tra lại trong transaction ghi, gồm các lần thanh toán đã commit trước đó.
- Chứng từ mua/bán thủ công, bút toán giảm công nợ chưa phân bổ hoặc thanh toán tương lai làm chặn phiếu tự động; cần đối soát, không coi là đã có liên kết hóa đơn. Nguồn phân bổ mới phải được quản lý bởi nghiệp vụ mua/bán/ghi tăng tài sản để không bị đảo riêng ngoài luồng. Danh sách công nợ theo chứng từ không thay cho sổ công nợ của mọi loại chứng từ.
- Đảo toàn bộ phiếu tiền có nguồn và ngày hợp lệ; phân bổ được trả lại từ ngày đảo. Chứng từ gốc và báo cáo trước ngày đảo giữ được lịch sử.
- Không thêm schema/migration; nguồn phân bổ và thông tin hóa đơn lưu bất biến trong payload nghiệp vụ cùng chứng từ, audit và idempotency. Chưa benchmark tập lớn; truy vấn hiện đọc các nguồn trong cùng snapshot.

## Thông tin chưa xác định

Người dùng chưa biết ngân hàng, nhà cung cấp hóa đơn điện tử và chế độ BCTC. Không tự chọn hoặc đổi bộ tham số thuế/chế độ kế toán hiện tại. Các phần tích hợp và báo cáo pháp định ở bảng trên vẫn là phần chưa triển khai, cần xác định nguồn tài liệu và nghiệm thu kế toán. Các phần chưa tích hợp như kiểm kê, đơn/hợp đồng, sao kê, trả lại/giảm giá và tạm ứng cũng còn phải triển khai; yêu cầu bổ sung đầy đủ chưa được hoàn tất trong mốc này.
