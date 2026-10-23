# Xuất Excel, CSV và bản in/PDF — 0.10.0

Menu **Xuất tệp** tại danh sách chứng từ (tiền/mua/bán/kho/tổng hợp), sổ sách/công nợ theo đối tượng, danh mục tài khoản/đối tượng và trình sửa BCTC có ba lựa chọn:

- **Excel (.xlsx):** bảng có tiêu đề doanh nghiệp, kỳ/phạm vi, phiên bản dữ liệu, hàng tiêu đề cố định, bộ lọc cột, dòng tổng từ máy chủ, độ rộng cột và cấu hình A4 ngang. Sheet Thông tin giữ bộ lọc và nguồn; BCTC có sheet riêng cho từng biểu mẫu đã chọn, cùng thuyết minh/mục cần kiểm tra. Mở trực tiếp tại sheet dữ liệu đầu tiên.
- **CSV (.csv):** UTF-8 có BOM, dùng khi trao đổi dữ liệu. Giữ endpoint CSV mặc định và quy tắc chống formula injection cũ. Chọn Excel khi cần giữ kiểu văn bản của mã/điện thoại có số0đầu khi mở bằng ứng dụng bảng tính.
- **Bản in / Lưu PDF:** mở bản in A4 ở tab riêng, giữ tab phân hệ nguồn. Bấm In/Lưu PDF rồi chọn lưu PDF trong hộp thoại trình duyệt. Không có dịch vụ tạo PDF trực tiếp trên server hoặc giả đổi đuôi HTML thành PDF.

Các định dạng xuất **toàn bộ tập lọc**, không chỉ trang đang xem. Sổ/chứng từ lấy dữ liệu và tổng từ cùng read snapshot; kiểm tra revision nếu được gửi. Danh mục lọc không dấu, vai trò và mã cha theo ID đã lưu; kiểm tra revision của snapshot. BCTC chỉ xuất bản đã lưu đúng ID/version; giữ nhãn Bản nháp, ô chưa nhập, nguồn tham chiếu, thuyết minh và người ký. Form còn sửa/chưa lưu thì nút Xuất bị khóa. Mọi GET xuất đều xác thực như nguồn đọc hiện có; không thêm quyền hoặc tạo bút toán/audit ghi.

## Độ chính xác và giới hạn

Tiền nguyên tối đa15chữ số được ghi bằng int64 vào ô số. Số lớn hơn15chữ số và số thập phân giữ nguyên chuỗi trong ô văn bản màu nâu; không ép qua float. Mã, số tài khoản, điện thoại và chuỗi người dùng luôn là ô văn bản. Excel không chứa macro, hyperlink hoặc công thức từ nội dung nhập; dòng tổng là giá trị đã tính trên server. Sheet Thông tin giải thích cách trình bày này để tránh cộng thiếu các ô dạng văn bản trong Excel.

CSV giữ giới hạn100.000dòng. Excel và bản in mới giới hạn10.000dòng/bảng để giới hạn bộ nhớ/file tạm và kích thước bản in; vượt giới hạn trả EXPORT_LIMIT, cần thu hẹp bộ lọc hoặc dùng CSV. XLSX dùng stream writer và đóng file tạm; kết thúc snapshot trước khi định dạng/đóng gói. Chưa benchmark giới hạn lớn nhất trong container production.

## Bằng chứng kiểm tra

- Go regression: XLSX mở lại sau ghi/đóng, giữ00009876, số nguyên lớn hơn2^53 và int64/30chữ số, số âm, decimal, ô trống khác0; công thức giả vẫn là text. Kiểm tra cố định tiêu đề, filter, style in, giới hạn và cancellation.
- HTTP/SQLite thật: vượt trang đang xem vẫn đủ dòng, cùng số tiền/tổng và revision, ba MIME/extension đúng, HTML escape, CSV cũ, unauthenticated401, viewer được đọc theo quyền hiện có, revision/BCTC version cũ409, các GET không đổi revision. BCTC nhiều sheet/giá trị lớn và vai trò/tìm không dấu có regression.
- Frontend: menu có mô tả định dạng, trạng thái tải/lỗi, focus bàn phím; khi lỗi tải lại workspace để có thể xuất lại với revision mới. Giữ demo CSV như trước. Bổ sung E2E cho menu/download/bản in, chỉ parse/list; chưa chạy E2E tự động.
- Go test/race/vet đạt cho ba package spreadsheet/httpserver/ledger;49test Node, TypeScript strict, production build và OpenAPI parse đạt; parse/list12test live và14test demo.
- Edge: menu bàn phím tự focus Excel; bản in đủ105dòng/tổng105.000 từ trang2, giữ tab nguồn; sửa tràn ngang tại390px với mã nháp và số tiền lớn. Chưa thao tác lưu PDF thực tế; ảnh đối chiếu được lưu trong phiên tác vụ, không đưa dữ liệu/ảnh doanh nghiệp vào Git. Tệp XLSX đã được kiểm chứng qua HTTP và đọc lại bằng Excelize; đường dẫn tệp tải xuống qua công cụ Edge chưa được xác minh do công cụ chờ download timeout.

Tham chiếu API: [Excelize streaming](https://xuri.me/excelize/en/stream.html), [ô dữ liệu](https://xuri.me/excelize/en/cell.html), [Ant Design Dropdown](https://ant.design/components/dropdown/).
