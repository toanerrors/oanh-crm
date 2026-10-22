# Kiểm tra giao diện — 22/10/2026

**Kết quả: passed trong phạm vi bố cục tương đồng và các luồng đã triển khai.** Không phải chứng nhận sao chép từng pixel hoặc đầy đủ toàn bộ sản phẩm AMIS. Các giới hạn nghiệp vụ ở [ma trận màn hình](docs/reference/misa-screen-map.md).

## Nguồn và môi trường

- Tham chiếu: 15 URL AMIS trong [ma trận](docs/reference/misa-screen-map.md), truy cập phiên đã có bằng Edge; không ghi dữ liệu vào MISA.
- Bản kiểm thử: frontend production build + Go/SQLite3.53.4 tại `http://127.0.0.1:18280`, dataset giả lập riêng. Không dùng dữ liệu doanh nghiệp thật trong test hoặc Git.
- Cặp ảnh xem cùng công cụ: phiếu thu trắng, chứng từ tổng hợp trắng và danh sách tiền mặt. Bố cục desktop đối chiếu ở chiều rộng1874px, chiều cao1078/1079px do viewport Edge; kiểm tra Oanh bổ sung1440×900,820×1180 và390×844. Reset viewport khi xong.
- Ảnh kết quả lưu ngoài repository tại thư mục artifact của chat `/Users/toanerrors/.codex/visualizations/2026/10/22/01a1612e-b0c8-74f2-a1a6-7894fc049279/`: `oanh-cash-desktop.jpg`, `oanh-cash-mobile.jpg`, `oanh-receipt-desktop.jpg`, `oanh-receipt-mobile.jpg`, `oanh-purchase-mobile.jpg`, `oanh-general-desktop.jpg`. Ảnh mẫu trắng: `misa-receipt-reference.jpg`, `misa-general-reference.jpg`. Không đưa ảnh/số liệu danh sách thực của MISA vào Git.

## Các vòng chỉnh và đối chiếu

| Vòng | Quan sát | Điều chỉnh/kiểm tra lại |
|---|---|---|
| 1 | Danh sách nhiều khoảng trống trước bảng, thao tác tách nhiều hàng | Gom lọc/thao tác thành toolbar, tổng thu/chi gọn, bảng và chi tiết dưới; nền/sidebar và tab gần cấu trúc mẫu |
| 2 | Header phiếu thu cao, tỷ lệ cột đối tượng/ngày/tổng chưa tương đồng | Grid3:.72:1.25, thu gọn tham chiếu, control28px trong phiếu, label13px; header trắng, nền xám, bảng trắng/đầu xanh xám |
| 3 | Ô tài khoản quá rộng, font fallback, form tổng hợp khác phân cấp | Hiển thị mã tài khoản gọn, cột diễn giải linh hoạt; Inter tự lưu trữ; form tổng hợp dùng header đối tượng/ngày/tổng, bảng và thanh thao tác cuối |
| 4 | Modal trên mobile có margin ngoài, dữ liệu cũ thiếu dòng thu/chi ở mẫu mới | Margin0/height100dvh, cuộn trong body/footer luôn thấy; chuyển payload thu/chi cũ thành một dòng để hiển thị; kiểm tra mở từ danh sách thẳng biểu mẫu |
| 5 | Một số trường không đăng ký làm mất loại dịch vụ; dữ liệu biểu mẫu chung lẫn trường chuyển tiền | Đăng ký tradeMode và giữ toàn bộ nháp; chuẩn hóa payload theo loại trước tính; chạy lại Edge thành công |

## Font, khoảng cách, màu và tài sản

- Inter chính thức v4.1 (OFL1.1), chữ chính13px; control28px trong phiếu,32px ở shell. Không tải font ngoài khi chạy. Icon dùng hệ thống hiện có; không sao chép logo/hình/MISA AVA.
- Đã xem cặp ảnh và chỉnh header, tỷ lệ cột, tab, cột kế toán, vùng đính kèm và footer. Giữ Oanh, bốn tổng tiền thay vì ba ô của MISA, số chứng từ có năm và bước xem trước/ghi sổ riêng.
- Chứng từ tổng hợp giữ dòng tài khoản + Nợ/Có độc lập, đáp ứng nhiều Nợ/Có; không biến thành mô hình một cặp tài khoản. Các ngày thủ công dùng hợp đồng một ngày hiện có. Tham chiếu/chứng từ kèm theo nằm trong vùng mở rộng.
- Sai khác còn lại P3: nhãn/tab/hàng cao theo Ant Design, bố trí trường bổ sung và giới hạn tệp10MB thay vì5MB; không phải lỗi mất thao tác. Chưa có các trường/luồng không được backend hỗ trợ như hải quan/ngoại tệ/đơn vị quy đổi.

## Kiểm tra tương tác và dữ liệu

- Lập phiếu thu hai dòng trên2^53, cất/reload/xem trước/ghi sổ/in: giữ toàn bộ chữ số, tổng Nợ/Có chính xác.
- Chuyển100 giữa hai số tài khoản cùng112: ghi sổ thành công; ngân hàng A còn9.999.900, B có100, tổng tiền gửi10.000.000.
- Bán dịch vụ1.001, CK10%, VAT10%, thu ngay: CK100/base901/VAT90/tổng991; tạo BH-2026-000007 và PT-2026-000008 cùng transaction.
- Nhập hàng1.000, VAT100, chọn nguồn phí101: Nợ156=1.101, Nợ1331=100, Có331=1.100, Có6427=101. Cất/reload/ghi sổ, dữ liệu phí giữ nguyên; giá trị thanh toán1.100 khác tổng Nợ1.201 được hiển thị đúng ngữ nghĩa.
- Kết chuyển cuối kỳ bằng Edge: post, khóa kỳ và mở lại bản đã post chỉ xem. Các test Go xác minh đảo/mở kỳ, các trường hợp lỗi, ba phương pháp phí tự động và giá kho theo hai kho.
- Xem chứng từ cũ PT-2026-000001 mở trực tiếp biểu mẫu; dòng250.000 hiển thị đầy đủ. Nháp chưa lưu có xuất JSON/giữ trong phiên; không persist vào localStorage.
- Mobile: không tràn ngang trang ở390px và820px; bảng cuộn ngang riêng. Modal390×844 vừa viewport; nút Đóng trong footer có đáy834px. Field tiền đã post có màu đọc được rgb(66,82,72), opacity1.
- Console ở tab kiểm tra cuối: không có error/warn. Không chạy Playwright CLI ở lượt này; thao tác UI thực hiện bằng Edge.

Không còn lỗi P0/P1/P2 quan sát được trong các màn hình/luồng trên. Chưa benchmark tải lớn hoặc nghiệm thu số liệu sản xuất. Release/healthcheck được xác minh ở bước phát hành Docker.


## BCTC —22/10/2026

- Tham chiếu Edge: luồng chọn kỳ và `CreateFinancialReportDetail`, bố cục tab/heading/bảng chỉ tiêu/mã/thuyết minh/hai cột/chữ ký/footer. Mẫu cũ của tài khoản tham chiếu được đối chiếu riêng với biểu mẫu TT99 năm.
- QA bằng hai dataset giả lập, không dùng dữ liệu production: tiền9007199254740993 giữ qua tab/lưu/reload; doanh thu1000−giảm trừ100=900; bộ số kiểm tra B01 tài sản1500/nguồn1500, B02 trước thuế600/thuế100/sau thuế500; B03 gián tiếp600−100+đầu kỳ1000=cuối1500; EPS−0,25 giữ chính xác. Kiểm tra backend độc lập các phép tính, thiếu dữ liệu và retry/concurrency/rollback/recovery.
- Màn hình1440×900 và390×844: bảng cuộn ngang riêng, footer đủ nút, nội dung modal cuộn dọc; rút mô tả vào hướng dẫn mở rộng. Source sổ toàn năm đọc snapshot, giữ hai bên đối tượng. Bản in HTML chứa dấu nháp, phiên bản và mục còn thiếu; script in cùng origin phù hợp CSP.
- Ảnh QA ngoài Git: `bctc-desktop-final.jpg`, `bctc-mobile-final.jpg` trong thư mục visualization của chat. Chưa benchmark tập BCTC lớn hoặc nghiệm thu biểu mẫu pháp định; không gắn nhãn hoàn tất B09/XML.
