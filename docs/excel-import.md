# Nhập từ Excel — 0.6.0

## Cách dùng

Mở **Nhập từ Excel** ở sidebar hoặc icon nhập cạnh tìm kiếm trong phân hệ. Chọn loại dữ liệu; tải mẫu cơ bản/nâng cao nếu cần. Kéo thả XLSX/CSV, chọn sheet và dòng tiêu đề (1–50), kiểm tra dữ liệu mẫu. Bước Ghép dữ liệu gợi ý cột theo tiêu đề tiếng Việt/không dấu; tiêu đề trùng cần chọn thủ công. Chọn định dạng ngày và số đúng với tệp, ghép đủ cột bắt buộc và xác nhận cột muốn bỏ qua. Kiểm tra toàn bộ, tải XLSX danh sách lỗi, sửa tệp/ghép cột rồi kiểm tra lại. Chỉ nhấn Nhập dữ liệu khi không còn lỗi.

Tệp được lưu trong vùng chờ trên server; chưa tạo giao dịch khi tải lên. Khi đã kiểm tra, thiết lập ghép cột được lưu để tiếp tục từ lịch sử sau tải lại trang. Lịch sử hiển thị 50 tệp gần nhất của chính người tải; tối đa10 tệp chờ/người. Có thể bỏ tệp chờ. Chứng từ/số dư được nhập thành **nháp**; mở chứng từ hoặc biểu mẫu số dư để đối chiếu và ghi sổ/xác nhận theo quy trình hiện có. Kết quả thành công được lưu; không nhập lại vì lỗi tải danh sách.

## Phạm vi

| Nhóm | Loại dữ liệu |
|---|---|
| Danh mục | Khách hàng/NCC; tài khoản; hàng hóa/dịch vụ; kho; tài khoản ngân hàng; nhân viên |
| Chứng từ | Chứng từ khác dạng cặp Nợ/Có hoặc nhiều dòng; phiếu thu/chi tiền mặt/tiền gửi; chuyển tiền nội bộ; mua/bán hàng hóa hoặc dịch vụ |
| Số dư | Dư tài khoản theo đối tượng/ngân hàng; tồn kho theo kho; TSCĐ/CCDC/242 có nguyên giá và đã phân bổ |

Owner nhập mọi loại; accountant không nhập tài khoản, nhân viên và số dư; viewer không có quyền import. Tệp do owner tải vẫn chỉ owner đó đọc được nếu sau này bị hạ quyền. Không tự tạo danh mục bị thiếu. Mã tham chiếu dùng mã danh mục, tài khoản ngân hàng dùng số tài khoản, không dùng ID nội bộ.

Các dòng chung mã nguồn tạo một chứng từ và phải lặp thông tin chung nhất quán. Cặp Nợ/Có dùng chung số tiền/diễn giải; dạng nhiều dòng cho phép nhiều đối ứng nhưng tổng phải cân đối. Mã nguồn bắt buộc và phải thống nhất với nhập tay/CSV/nghiệp vụ tự động. Phân bổ công nợ dùng mã nguồn hoặc số chứng từ mua/bán đã ghi sổ, số tiền phân bổ trên mỗi dòng; để trống tài khoản đối ứng. Chi phí mua hàng dùng mã chứng từ mua đã ghi sổ đánh dấu chi phí và số dòng nguồn từ1; cách phân bổ/chiết khấu theo chính sách hiện có. Nhiều kho dùng mã kho từng dòng.

Danh mục mặc định thêm mới, báo trùng; chế độ cập nhật chỉ sửa mã đã có và vẫn chịu hạn chế lịch sử. Các cột tùy chọn chưa ghép giữ giá trị cũ khi cập nhật; ghép cột có ô trống áp dụng giá trị trống/mặc định của trường. Số dư chỉ thêm chi tiết mới vào nháp trước lần ghi sổ đầu tiên, cùng ngày chuyển đổi; báo trùng tài khoản/đối tượng/ngân hàng, hàng/kho, mã tài sản. Không cộng lại số dư đã có hoặc sửa bộ đã xác nhận.

## Tệp và tính chính xác

- XLSX hoặc CSV UTF-8 dấu phẩy, tối đa20MB. XLS nhị phân cũ phải Save As XLSX; không đọc XLS/XLSM, tệp mã hóa, macro hoặc liên kết workbook ngoài.
- Tối đa20sheet; 5.050dòng gồm vùng tiêu đề, 128cột/sheet; toàn workbook tối đa100.000ô và4MB dữ liệu JSON đọc được. Mỗi lần nhập tối đa5.000dòng và500chứng từ/bản ghi; số dư còn chịu giới hạn150chi tiết của nghiệp vụ hiện có. ZIP giải nén tối đa64MB,32MB/entry.
- Không chạy công thức; cột đã ghép chứa công thức hoặc lỗi Excel bị báo lỗi. Paste Values trong bản sao nguồn sau khi tự đối chiếu. Không nhập báo cáo dẫn xuất như giao dịch.
- Giữ mã số thuế/điện thoại/số tài khoản/số hóa đơn dạng Text; ô định danh dạng số bị từ chối để không đoán số0 đầu. Mẫu đặt cột Text. Giá trị Excel đã bị làm tròn trước khi tải lên không thể phục hồi.
- Tiền/quantity/rate đọc từ chuỗi raw, chuẩn hóa bằng math/big và kiểm tra theo nghiệp vụ; không qua float. Số serial ngày hỗ trợ1900/1904, không nhận ngày giả29/02/1900 hoặc phần giờ. Text ngày theo DD/MM/YYYY hoặc YYYY-MM-DD đã chọn. Không tự đoán dấu nghìn/thập phân.
- Trình kiểm tra xử lý toàn bộ, hiển thị tối đa1.000 lỗi và tổng lỗi thực tế. Bất kỳ dòng lỗi nào cũng chặn toàn bộ gói. Xem trước không lưu danh mục/chứng từ, không cấp số. Nhập kiểm tra lại quyền/epoch/revision/nguồn trong cùng transaction và commit toàn bộ cùng audit/idempotency. Tổng kiểm tra chỉ gồm dòng hợp lệ.

Không nhập bảng lương/chấm công, hóa đơn chuyển đổi công nợ, BCTC dẫn xuất hoặc những nghiệp vụ chưa có quy tắc trong ứng dụng. Không tích hợp AVA hay tự chuyển mọi mẫu Excel bên ngoài; cần ghép đúng cột và dùng chính sách đã xác nhận.

## Tham chiếu đã kiểm tra ngày22/10/2026

- Luồng MISA trên [dashboard](https://actapp.misa.vn/app/Overview/dashboard), menu Nhập từ Excel của tiền gửi: chọn tệp → ghép dữ liệu → kiểm tra → kết quả, sheet/dòng tiêu đề, mẫu cơ bản/nâng cao. Chỉ xem giao diện, không nhập dữ liệu thật vào MISA.
- [Hướng dẫn nhập/xuất Excel MISA](https://helpact.misa.vn/kb/cac-cau-hoi-thuong-gap-khi-xuat-khau-nhap-khau-du-lieu-excel/).
- [Excelize2.11.0](https://github.com/qax-os/excelize/releases/tag/v2.11.0), [OpenReader/RawCellValue/giới hạn giải nén](https://xuri.me/excelize/en/workbook.html). Dependency được người dùng phê duyệt, parser chạy backend.

Từ0.7.0, mẫu tài khoản có Mã tài khoản cha (parentCode). Nhập cha trước con trong cùng tệp; để trống ô đã ghép để tạo cấp1, không ghép cột để giữ cha hiện có khi cập nhật. Vòng lặp/nhánh có chứng từ/cha không tồn tại bị báo lỗi và chặn toàn bộ gói.
