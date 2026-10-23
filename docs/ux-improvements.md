# Cải thiện thao tác và phản hồi — 0.9.1

Ngày kiểm tra:23/10/2026. Phạm vi: tìm chứng từ/chức năng, danh sách và sổ sách, phiếu nghiệp vụ, điều hướng phân hệ trên Edge. Người dùng yêu cầu thao tác nhanh và giữ công việc đang làm; giữ bộ UI và bố cục hiện có, không thay quy tắc kế toán.

## Bằng chứng trước sửa

1. Mở Tiền gửi trên production: toolbar và tab phân hệ có sẵn, trạng thái trống rõ ràng. Ảnh đã lưu/kiểm tra:01-bank-before.png.
2. Mở phiếu thu tiền gửi trống: biểu mẫu tải được chính sách hiệu lực; ảnh02-voucher-before.png. Bỏ capture đầu còn đang chuyển màn hình, chỉ dùng capture sau khi form hoàn tất. Đọc mã thấy onValuesChange đẩy toàn bộ phiếu lên WorkspaceProvider mỗi lần gõ và Đóng quay về đường dẫn cố định. Đây là bằng chứng mã nguồn, chưa đo số lần React render hay độ trễ nhập trước sửa.
3. Gõ liên tiếp7ký tự `uxprobe` trong danh sách: ghi nhận7request GET documents với q lần lượt u/ux/uxp/uxpr/uxpro/uxprob/uxprobe qua CDP Network. Ảnh03-search-before.png. Không lưu chứng từ hoặc thay đổi số liệu production.

Ba điểm ưu tiên: request theo từng phím; mất ngữ cảnh danh sách khi đóng phiếu/Back; thanh tìm kiếm chỉ dựa trên chứng từ gần đây, thiếu truy cập nhanh chức năng.

## Đã triển khai

- Input danh sách/sổ phản hồi ngay, debounce250ms cho bộ lọc; Enter/blur tìm ngay, xóa tìm kiếm áp dụng ngay, giữ an toàn khi nhập IME và hủy timer khi đổi phạm vi/unmount.
- Ctrl/⌘ K mở tìm nhanh; tên chức năng tìm không dấu, ưu tiên khớp tên rồi chức năng ghim trong cùng mức phù hợp, lọc theo quyền hiện có. ↑↓/Enter chọn, Esc đóng. Tìm chứng từ từ API toàn kỳ thay vì chỉ100bản gần nhất; request nhận AbortSignal, không tìm mỗi phím hoặc tải toàn bộ dữ liệu.
- Lưu page/pageSize/type/status/sort/direction trong URL, giữ bộ lọc khi mở/đóng và lưu phiếu; đổi kỳ về trang đầu. Tải trước chunk từ sidebar/tab/chức năng khi hover hoặc focus, bỏ qua kết nối2G/Data Saver; chỉ mount route đang dùng.
- Giữ trang trước trong cùng phạm vi dữ liệu khi đổi page, hiện nhãn đang cập nhật, khóa mở/xuất trên trang cũ; đổi bộ lọc/kỳ/revision/quyền/epoch không dùng trang cũ như dữ liệu mới. Có phản hồi lúc tải chi tiết và tải chính sách; lỗi tải danh sách/sổ có nút thử lại đúng truy vấn.
- Phiếu giữ Form state local khi gõ; rời route/Back chụp nội dung vào nháp bộ nhớ phiên với ID/version. Khi lệnh đang xử lý thì giữ màn hình để chờ kết quả; refresh/đóng tab có cảnh báo. Không persist nội dung tài chính vào localStorage.
- Bản0.9.1 bổ sung cảnh báo reload/đóng tab ngay cả khi form đã đóng về danh sách và phiếu còn trong bộ nhớ phiên; không ghi nội dung nháp xuống trình duyệt.
- Ctrl/⌘ S lưu nháp, Ctrl/⌘ Enter tính/xem trước; có khóa gửi lặp trong khi validate/gửi. Lỗi form tự cuộn và focus trường đầu tiên. Ghi sổ vẫn cần xác nhận riêng. Tôn trọng reduced motion cho sidebar/tiến độ.

## Kiểm tra sau sửa

- Cùng7ký tự, thao tác gõ liên tiếp trên Edge: chỉ1request q=uxprobe sau khi ngừng gõ. Đây là đo số request cho chuỗi kiểm thử này, không phải benchmark mọi tốc độ gõ/mạng hoặc FPS. Ảnh04-search-after.png.
- Dữ liệu giả lập riêng: Back/tiếp tục nháp giữ diễn giải và chuỗi9007199254740993; đóng phiếu bán hàng trả về đúng page2/pageSize20/sort amount desc. Lưu nháp5.000.000 bằng phím tắt; API xác minh status draft, version1, amount5000000 sau cả xem trước bằng phím tắt. Không phát sinh ghi sổ từ kiểm thử này.
- Tìm `chi tien` ưu tiên Chi tiền gửi; dùng ↑↓/Enter mở đúng form. Chức năng ghim xuất hiện đầu danh sách khi chưa gõ. Tìm BH-2026-000001 trong105phiếu bán hàng tìm/mở được bản cũ nằm ngoài100chứng từ workspace. Ảnh05–07 đối chiếu tìm nhanh.
- Giả lập độ trễ3giây: đổi page3→4 giữ trang cũ với nhãn cập nhật và nút mở/xuất disabled. Đổi tháng10→11 loại dữ liệu cũ ngay, không dùng placeholder khác kỳ; khi đổi kỳ, bỏ page trong URL. Back khi preview đang chờ vẫn ở phiếu; sau khi hoàn tất Đóng về đúng danh sách. Mạng đã khôi phục. Ảnh08-page-loading.png.
- Báo cáo vẫn ở /sales/reports và giữ tab phân hệ; Enter tìm131 tạo1request phù hợp, xóa q cập nhật ngay.
-390px: tìm nhanh, mở phiếu, lỗi validation focus id description, root scrollWidth/clientWidth đều390; đã kiểm tra ảnh09-mobile-palette.png và10-mobile-validation.png, khôi phục viewport. Console error không có trong phiên kiểm tra.
-49test Node đạt, TypeScript strict và production build đạt. Bổ sung E2E luồng trên; Playwright parse/list11test live đạt, chưa chạy E2E tự động. Không thay backend/schema/API hoặc quy tắc tính tiền.

Ảnh đối chiếu được giữ trong phiên tác vụ tại thư mục oanh-ux của23/10/2026; không đưa ảnh có giao diện/dữ liệu doanh nghiệp vào Git. Screenshot kiểm thử dùng dữ liệu giả lập khi có số liệu.

## Giới hạn và việc tiếp theo

Bản0.9.1 đã ghi nhận sự kiện beforeunload khi tải lại từ danh sách có phiếu trong bộ nhớ. Công cụ Edge tự xử lý luồng tải lại; chưa xác minh thao tác hủy hộp thoại giữ phiên qua công cụ này.

Chưa đo FPS/độ trễ nhập với phiếu hàng trăm dòng hoặc dataset lớn, chưa nghiệm thu mọi bộ gõ IME/hệ điều hành. Bundle Ant Design lớn vẫn phát cảnh báo build; không thêm dependency hoặc chia lại vendor chỉ dựa trên suy đoán. Cần benchmark trên máy và bộ chứng từ đại diện trước khi kết luận tốc độ tối đa; đợt này xác minh các thao tác và số request cụ thể ở trên.

Tham chiếu kỹ thuật: [React Router useBlocker](https://reactrouter.com/api/hooks/useBlocker), [TanStack Query placeholder data](https://tanstack.com/query/latest/docs/framework/react/guides/placeholder-query-data), [Ant Design Form](https://ant.design/components/form/).
