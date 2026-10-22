# Trạng thái dự án

Cập nhật: 22/10/2026. Tài liệu ghi chức năng có mã nguồn và bằng chứng kiểm tra, không thay cho nghiệm thu kế toán.

## Phạm vi đã chốt

- Quy trình sau mỗi task có thay đổi: validation → commit → push nhánh/tag liên quan → release Docker trên server hiện tại → healthcheck. Người dùng đã ủy quyền mặc định, không cần xác nhận lại cho commit/push/release thông thường; quy tắc được lưu trong `AGENTS.md`.
- Người dùng yêu cầu ứng dụng kế toán bao phủ workbook, UI/UX hoàn chỉnh và nghiệp vụ tự động; chính sách thay đổi trong Cài đặt.
- Go + SQLite; React + TypeScript strict + Vite SPA, Ant Design Form/Table, React Router, TanStack Query; pnpm. Frontend và driver Go `modernc.org/sqlite` đã được chấp thuận cài đặt.
- UI ưu tiên mật độ thông tin: chữ chính 13px, chữ phụ 12px, input/button cao 32px; margin/padding nhỏ và đồng đều.
- Một pháp nhân, một máy chủ ghi trên ổ cục bộ, VND theo đơn vị đồng. Tiền và decimal qua API là chuỗi; int64 và math/big ở backend, chuỗi/BigInt ở frontend.

## Release Docker 0.2.0

- Người dùng yêu cầu commit và release trên server hiện tại bằng Docker; domain `oanh.toanerrors.com` sẽ được cấu hình riêng ở Cloudflare Tunnel.
- Bổ sung Dockerfile nhiều stage, Compose một container chạy user thường/read-only, healthcheck, restart policy và volume `oanh_data`; origin publish ở `127.0.0.1:8080`, bật cookie Secure cho HTTPS qua tunnel.
- Đã build image `oanh-crm:0.2.0`, chạy container `oanh-app-1` healthy ở `127.0.0.1:8080`; UID 65532, root filesystem read-only, volume `oanh_data`. Restart giữ nguyên active dataset; API/database, HTML/SPA và màn hình khởi tạo không lỗi JavaScript đã kiểm tra. Host/Origin của domain HTTPS qua proxy được chấp nhận, request cross-origin bị chặn.
- Build Docker chạy 15 test frontend và production build thành công; healthcheck Go đã qua compile/vet. Không tạo tài khoản hay dữ liệu mẫu trên volume release. Nhánh phát hành `release/v0.2.0`, tag `v0.2.0`; remote `origin` đã cấu hình tới `https://github.com/toanerrors/oanh-crm.git`. Chưa cấu hình route Cloudflare hoặc kiểm tra HTTPS công khai trong đợt release.
- Runbook: [deployment.md](deployment.md).

## Đã triển khai

### Ẩn tab tài khoản Google Sheet — 0.6.1

- Bỏ tab “Tài khoản từ Google Sheet” và phần bảng/ghi chú/nút nạp tham chiếu khỏi Danh mục; gỡ component/import không còn dùng. Giữ hai tab đối tượng và hệ thống tài khoản, dữ liệu tài khoản đã nhập và API nguồn ở backend.
- Validation: TypeScript strict, production build và diff check đạt. Không đổi backend/schema hoặc quy tắc số liệu; không chạy thêm Go/E2E cho thay đổi hiển thị này.

### Nhập XLSX/CSV theo luồng MISA — 0.6.0

- Trình nhập4bước: chọn tệp/sheet/dòng tiêu đề → ghép cột tự động hoặc thủ công → kiểm tra toàn bộ/lọc lỗi/tải XLSX lỗi → kết quả. Mẫu cơ bản/nâng cao Text, định dạng ngày/số tường minh, xác nhận cột bỏ qua. Có lối vào tổng quan/sidebar và toolbar các phân hệ/danh mục/kho/ngân hàng/số dư; modal giữ tab nguồn, mobile giữ thanh bước gọn.
-14loại: đối tượng/tài khoản/hàng/kho/ngân hàng/nhân viên; chứng từ khác cặp hoặc nhiều dòng; thu/chi tiền mặt/tiền gửi/chuyển nội bộ; mua/bán hàng hóa/dịch vụ; số dư tài khoản/đối tượng/ngân hàng/kho/TSCĐ/242. Tái sử dụng validator nghiệp vụ, hỗ trợ chiết khấu/phân bổ công nợ/nguồn chi phí mua hàng/nhiều kho. Chứng từ/số dư chỉ lưu nháp; không tự ghi sổ/xác nhận. Danh mục cập nhật theo mã giữ cột tùy chọn chưa ghép và khóa lịch sử hiện có.
- Excelize2.11.0 được người dùng phê duyệt. Parser raw string/exact decimal, hệ ngày1900/1904; chặn công thức ở cột ghép, lỗi ô, macro/liên kết ngoài, XML địa chỉ ô/dòng mơ hồ. XLS nhị phân chưa hỗ trợ: cần Save As XLSX. Giới hạn20MB/20sheet/128cột/5.000dòng dữ liệu/500bản ghi, cùng giới hạn bộ đệm và nghiệp vụ; không nhập báo cáo dẫn xuất/bảng lương hoặc nghiệp vụ chưa hỗ trợ. Phạm vi và cách dùng: [excel-import.md](excel-import.md).
- Schema5 lưu nguồn đã đọc/hash/cấu hình/kết quả theo actor và epoch; lịch sử50tệp, tối đa10tệp chờ/người. Preview rollback, commit toàn bộ cùng audit/idempotency và revision; lỗi bất kỳ dòng/commit không ghi một phần. Kiểm tra trùng nguồn xuyên kênh. Giữ API CSV cũ; backup/restore nhận schema1–5, giữ dữ liệu nhập đã lưu. Binary0.5.x không đọc schema5.
- Validation: Go race/vet cho spreadsheet/operations/ledger/sqlite/httpserver đạt; regression gồm tiền trên2^53, rollback lỗi dòng/trigger cuối giao dịch, retry/cạnh tranh commit, revision/epoch/quyền/actor, phân bổ công nợ/chi phí mua hàng, migration4→5, backup/restore/reopen.39test Node, TypeScript strict, production build đạt. E2E mới và cập nhật quyền, đọc thành công danh sách9test live; chưa chạy E2E tự động.
- Edge/dữ liệu giả lập: chọn sheet thứ2/tiêu đề dòng3, tự ghép tiếng Việt và ghép thủ công, báo lỗi dòng5/chặn nhập, tiền lớn nguyên vẹn, nhập2chứng từ thành nháp, tải lại/xem kết quả không nhập thêm, khôi phục cấu hình từ lịch sử, xác nhận bỏ qua cột và nhập mã đối tượng00009876, modal giữ Mua hàng;390px không tràn sau hiệu ứng, khôi phục viewport, console không lỗi. XLSX mẫu/báo lỗi được xác minh qua integration HTTP; chưa xác minh tệp tải xuống qua Edge vì công cụ chờ download timeout. Chưa benchmark giới hạn dữ liệu lớn; cảnh báo chunk Ant Design lớn vẫn còn.


### Căn hàng bộ lọc công nợ — 0.5.11

- Nhãn Đến ngày đặt cạnh DatePicker trong một hàng flex, ô ngày rộng180px và cao32px như hai Select bên cạnh. Chỉ áp dụng bộ lọc công nợ theo chứng từ; giữ giá trị ngày/API và cơ chế lọc cũ.
- Validation: TypeScript strict/production build đạt; bổ sung kiểm tra hình học vào E2E phân hệ mua/bán, đọc thành công danh sách8 test live (chưa chạy E2E tự động). Edge: xác nhận trước sửa DatePicker lệch xuống10px, sau sửa cả3 control cùng tọa độ trên/dưới và cao32px ở mua/bán; mở lịch, đổi ngày lọc phản ánh đúng ngày ISO; mobile390px xuống hàng, không tràn ngang sau khi chuyển bố cục hoàn tất; khôi phục viewport, không có lỗi console.

### Chức năng nhanh và ghim trên sidebar — 0.5.10

- Rê chuột/focus vào phân hệ mở menu tối gồm nghiệp vụ, danh mục và báo cáo hiện có; dùng cùng danh sách khi sidebar224/210px hoặc thu gọn64px. Lối tắt báo cáo giữ đường dẫn/tab phân hệ nguồn; thao tác lập chứng từ theo quyền, kết chuyển mới chỉ dành cho owner. Mẫu chỉ hiển thị các lối tắt chạy trong mẫu.
- Nút sao ghim/bỏ ghim từng chức năng vào nhóm ĐÃ GHIM, có nhãn phân hệ để phân biệt. Chỉ lưu ID trong danh sách cho phép vào localStorage theo công ty/người dùng, không lưu dữ liệu nghiệp vụ; lọc ID lỗi/trùng/vượt quyền, đồng bộ sự kiện storage giữa tab và báo nếu trình duyệt không cho lưu. Mobile mở danh sách bằng nút cạnh phân hệ, dùng chung ghim với desktop; chỉnh nền Drawer tối để đủ tương phản.
- Menu mở bằng ArrowRight/ArrowDown, Tab vào nội dung, Escape đóng và trả focus; đóng khi điều hướng/đổi chế độ sidebar/breakpoint hoặc nhấn ngoài. Popup chỉ mount khi mở; giới hạn chiều cao, cho cuộn. Dùng lại handler chứng từ khác để giữ nháp.
- Validation:35 test Node gồm5 test lối tắt/quyền/mẫu/ID ghim/phạm vi người dùng; TypeScript strict/production build đạt. Cập nhật E2E sidebar và thêm2 kịch bản ghim/mobile, danh sách14 test đọc thành công; chưa chạy E2E tự động. Edge/backend dữ liệu giả lập: hover mở/rời chuột đóng, ArrowRight/Escape và focus, sidebar64px, ghim không chuyển trang/còn sau tải lại/bỏ ghim, mở báo cáo giữ tab Mua hàng, mở Thu tiền gửi đúng1121, nháp giữ diễn giải sau chuyển Bán hàng; mobile390px và popup820×600 không tràn, khôi phục viewport; không có lỗi console. Kiểm tra trực tiếp thu/mở của0.5.9 nay đã thực hiện khi kết nối Edge hoạt động trở lại. Chưa có benchmark hiệu năng chuyển động.

### Sidebar thu gọn/mở rộng — 0.5.9

- Nút icon ở trái header desktop thu sidebar về64px và mở lại224px (210px ở màn hình vừa); đồng bộ sidebar, vùng nội dung và tiến trình điều hướng bằng biến chiều rộng chung, transition200ms. Ẩn tên/nhóm/logo chữ khi thu gọn, giữ icon/trạng thái đang chọn/badge nháp và tên truy cập; tooltip tên mục khi hover/focus. Không mount lại Outlet hoặc đổi query/form khi bật tắt.
- Trạng thái giao diện giữ trong AppShell khi chuyển route; không lưu dữ liệu tài chính hoặc tùy chọn mới vào localStorage. Mobile giữ Drawer đầy đủ, ẩn nút thu gọn desktop; CSS thu gọn chỉ áp dụng từ761px. Dùng quy tắc giảm chuyển động sẵn có của ứng dụng.
- Validation:30 test Node, TypeScript strict/production build và diff check đạt; bổ sung E2E cho chiều rộng/tooltip/điều hướng/giảm chuyển động/mobile, kiểm tra danh sách12 test. Chưa chạy E2E tự động và chưa xác minh hiển thị/chuyển động trực tiếp: kết nối điều khiển Edge timeout trong lượt này. Việc tiếp theo: kiểm tra thu/mở, tooltip, nháp và breakpoint trên Edge khi kết nối hoạt động.

### DatePicker Ant Design cho ngày và tháng — 0.5.8

- Thay toàn bộ input ngày/tháng native bằng `DateInput` bọc Ant Design DatePicker trong chứng từ khác, mua/bán/thu chi, nghiệp vụ, chính sách, công nợ, số dư và ngày lập BCTC. Form và API tiếp tục dùng chuỗi `YYYY-MM-DD`/`YYYY-MM`; giao diện `DD/MM/YYYY`/`MM/YYYY`, lịch tiếng Việt, nhập bàn phím và xóa ngày. Truyền tiếp id/ref, trạng thái disabled và props của Form.
- Dùng Day.js1.11.23 đã có qua Ant Design, khai báo trực tiếp đúng phiên bản sau khi người dùng chấp thuận; không nâng package. Parse strict, không đổi ngày sai sang ngày khác hoặc chuyển ngày qua UTC. Theme DatePicker có cell36×24 để lịch gọn, vừa màn hình nhỏ dù controlHeightSM toàn ứng dụng là32.
- Validation:30 test Node gồm3 test ngày/tháng/ngày nhuận/định dạng sai/timezone; TypeScript strict và production build đạt. Cập nhật E2E tương tác lịch và định dạng ngày chính sách, kiểm tra danh sách11 test mẫu/8 test live; chưa chạy E2E tự động. Edge/dữ liệu giả lập: lịch tiếng Việt trong chứng từ/popup mua hàng, chọn ngày công nợ, nhập ngày nhuận/tháng, ngày sai không rollover, xóa/khôi phục ngày và nhập chuỗi canonical; kiểm tra popup không tràn ở360/390px, khôi phục viewport. Không ghi dữ liệu thử vào production.

### Nút icon và tooltip trên thanh công cụ — 0.5.7

- Thêm `ToolbarButton` dùng Ant Design Button/Tooltip: icon32×32px, nhãn tiếng Việt qua tooltip khi hover/focus và `aria-label`; wrapper giữ tooltip cho nút bị vô hiệu hóa. Giữ trạng thái disabled/loading, quyền và handler cũ.
- Áp dụng thanh công cụ báo cáo/số dư/BCTC, danh sách chứng từ/thu chi/mua bán, nhập CSV, danh mục/ngân hàng/kho, công nợ, hàng hóa/nhân viên, chính sách/người dùng/kỳ và nút tạo trên tổng quan. Nhóm icon chứng từ thành hàng riêng trên mobile; các nút lưu/xác nhận trong form vẫn có chữ.
- Validation:27 test Node và TypeScript strict/production build đạt; cập nhật E2E kiểm tra nút không có chữ, giữ accessible name và tooltip khi focus; kiểm tra danh sách11 test mẫu/8 test live, chưa chạy E2E tự động. Edge/dữ liệu giả lập: tooltip qua Tab, Enter mở đối tượng; mở đúng số dư/BCTC/nhập CSV/ngân hàng/kho và menu tạo chứng từ. Kiểm tra icon32×32 và không tràn trang ở390/820px; khôi phục viewport mặc định. Không ghi dữ liệu thử vào production.

### Hiển thị công ty trên header — 0.5.6

- Chuyển tên công ty, biểu tượng và mô tả khỏi sidebar sang bên trái header dùng chung; nhấn vẫn mở Thiết lập. Sidebar desktop và menu mobile không còn khối công ty lặp.
- Header giữ chiều cao52px, tên dài co gọn bằng dấu ba chấm và có tên đầy đủ qua tooltip gốc; giữ menu, tìm kiếm và tài khoản ở màn hình nhỏ. Không đổi API, dữ liệu hoặc quy tắc kế toán.
- Validation:27 test Node và TypeScript strict/production build đạt; cập nhật E2E hiện có cho vị trí/nút công ty và viewport, kiểm tra danh sách8 test; chưa chạy E2E tự động. Edge/backend dữ liệu giả lập: desktop và390px, nút công ty mở Thiết lập, menu mobile không lặp công ty, tìm kiếm và menu tài khoản mở đúng.

### Thanh công cụ cạnh tìm kiếm, bỏ tiêu đề trang lớn — 0.5.5

- `PageToolbar` dùng chung: bộ lọc/tìm kiếm bên trái, các nút chức năng căn phải. Chuyển thao tác báo cáo/số dư/in/CSV, BCTC, danh mục/đối tượng, công nợ và tài khoản tham chiếu vào hàng tìm kiếm. Các mục hàng hóa/nhân viên/chính sách/người dùng và tổng quan đặt nút ngay trên bảng tương ứng. Danh sách chứng từ giữ thanh công cụ hiện có; mẫu demo cũng đưa nút xuống hàng tìm kiếm.
- Tiêu đề/mô tả trang dùng chung không chiếm bố cục; giữ heading ngữ nghĩa cho trình đọc màn hình. Bỏ tiêu đề BCTC lặp phía trên bộ lọc. Thanh tab, phân trang, quyền, sự kiện mở form và query/API/số liệu giữ nguyên. Màn hình nhỏ cho nhóm nút xuống dòng và căn phải.
- Validation:27 test Node và TypeScript strict/production build đạt; bổ sung kiểm tra nhóm tìm kiếm/nút trong E2E hiện có và kiểm tra danh sách test. Edge/dữ liệu giả lập: báo cáo, BCTC, danh mục, công nợ, hàng hóa, chính sách/người dùng, tổng quan; mở đúng form số dư/BCTC/đối tượng, không lưu dữ liệu thử vào production; kiểm tra bố cục390px và khôi phục viewport mặc định. Chưa chạy E2E tự động.

### Bỏ ghi chú cuối bảng báo cáo — 0.5.4

- Gỡ khối footer có biểu tượng khiên và dòng “Cân đối Nợ/Có chỉ là một bước kiểm tra…” khỏi màn hình sổ/báo cáo, gồm các báo cáo nằm trong phân hệ. Khối ghi chú tương ứng ở công nợ cũng được gỡ. Giữ nguyên phân trang, bộ lọc, tổng số và truy vấn.
- Validation: TypeScript strict/production build đạt; bổ sung assertion vào E2E hiện có và kiểm tra danh sách test. Không chạy E2E tự động cho thay đổi văn bản này; kiểm tra hiển thị trên Edge sau release.

### Bỏ breadcrumb và footer phạm vi dữ liệu — 0.5.3

- Bỏ breadcrumb “Không gian làm việc → phân hệ” ở header và dòng footer “Dữ liệu doanh nghiệp/mẫu · Kỳ · Phiên bản · Đơn vị: VND” trên các trang dùng chung. Gỡ component/import/CSS không còn sử dụng; giữ nút mở menu mobile, tìm kiếm, tài khoản và các tab phân hệ.
- Người dùng đính chính yêu cầu: giữ phân trang. Đã hoàn tác toàn bộ thay đổi bỏ phân trang trước khi commit; backend/API và các bảng giữ cơ chế phân trang của0.5.2. Không đổi schema, dữ liệu hoặc quy tắc kế toán.
- Validation:27 test Node, TypeScript strict/production build và kiểm tra danh sách E2E. Edge/backend thật trên dữ liệu giả lập: Cân đối phát sinh không còn breadcrumb/footer, thanh phân trang185 dòng vẫn chuyển trang2; menu390px mở/đóng được. E2E được bổ sung kiểm tra không còn breadcrumb/footer và vẫn có phân trang; chưa chạy E2E tự động, đã kiểm tra trực tiếp bằng Edge.

### Giữ tab và báo cáo trong phân hệ — 0.5.2

- Thêm layout/route con chung cho Mua hàng, Bán hàng, Tiền mặt, Tiền gửi, Hàng tồn kho và Kế toán tổng hợp. Tab Báo cáo giữ thanh tab, menu và breadcrumb của phân hệ; các tab Nhật ký/Sổ cái/Cân đối/BCTC chuyển nội dung ngay tại đường dẫn con, không dẫn ra trang báo cáo độc lập.
- Mua/bán mở Công nợ theo chứng từ, Nhà cung cấp/Khách hàng, Hàng hóa ngay trong phân hệ. Loại mua/bán theo route, danh mục lọc đúng vai trò gồm đối tượng có cả hai vai trò. Liên kết tổng hợp công nợ cũng giữ phân hệ. Các route độc lập cũ được giữ.
- Layout nhớ đường dẫn từng tab trong phiên; bộ lọc chứng từ và tab/bộ lọc báo cáo quay lại theo URL. Chỉ mount route đang xem, giữ Query/API/phạm vi và số liệu server hiện có; không đổi schema hoặc quy tắc kế toán.
- Validation:27 test Node, TypeScript strict và production build. Edge/backend thật trên dữ liệu giả lập: mua/bán qua công nợ/danh mục/hàng hóa/báo cáo, BCTC↔Cân đối, menu/breadcrumb, bộ lọc và tab khi quay lại/reload, Back/Forward; báo cáo trong cả sáu phân hệ và tab Kết chuyển. Bổ sung E2E cho luồng mua/bán; không chạy Playwright CLI điều khiển browser trong lượt kiểm tra Edge.

### Chứng từ khác: nhập số tiền và diễn giải một lần — 0.5.1

- Mặc định mỗi dòng có TK/đối tượng Nợ và Có riêng, một số tiền và một diễn giải chung. Nhập/sửa số tiền hoặc diễn giải cập nhật đúng hai dòng ledger của cặp; thêm/xóa cả cặp. Diễn giải dòng có thể để trống để dùng diễn giải chứng từ khi xem sổ.
- Chế độ nhiều Nợ/Có giữ mỗi dòng nguồn một bên, một số tiền và một diễn giải. Nháp cũ chỉ dùng giao diện cặp khi các dòng đối ứng theo thứ tự hai bên có cùng số tiền/diễn giải; còn lại mở chi tiết và chặn chuyển chế độ gây mất thông tin. Giữ thứ tự dòng, đối tượng, tài khoản, API, schema4 và kiểm tra server.
- Validation: TypeScript strict/build,24 test Node (cặp tiền lớn, khác diễn giải, nhiều đối ứng, Có trước Nợ). Edge/backend thật trên dữ liệu giả lập: một lần nhập9.007.199.254.740.993 tạo đúng hai dòng cùng tiền/diễn giải, giữ đối tượng riêng; thêm/xóa cặp, chuyển chế độ/route giữ nháp; mở/lưu nháp1 Nợ/3 Có giữ nguyên toàn bộ dòng và thứ tự. Nhập1.5 báo lỗi, không làm tròn; bố cục mobile390px giữ nút lưu và bảng cuộn ngang. Đã cập nhật E2E selectors, không chạy Playwright CLI trong lượt kiểm tra Edge.

### Nhập số dư đầu kỳ — 0.5.0

- Thêm thao tác ngay tại Cân đối phát sinh, form Ant Design nhập Nợ/Có, nguồn đối chiếu và ngày đầu tháng chuyển đổi; lưu/mở lại/bỏ nháp, mở kỳ còn thiếu và xác nhận riêng. Chủ sở hữu khởi tạo trước lần ghi sổ đầu tiên, số xác nhận bất biến. Không thêm số dư thật khi release.
- Chi tiết tài khoản/đối tượng/ngân hàng, hàng hóa156 theo hàng–kho và TSCĐ211/214, trả trước242; tạo đồng thời nguồn ledger, ngân hàng, lô tồn/biến động, tài sản và số kỳ đã phân bổ. Mọi tiền giữ chuỗi/int64/big.Int/BigInt; không tự cân đối chênh lệch.
- Nguồnopening vào dư đầu ngay kỳ chuyển đổi, tách phát sinh/NKC/thu chi và danh sách chứng từ thông thường; chuyển tiếp kỳ sau từ cùng sổ. Context có mốc dữ liệu. Báo cáo/source BCTC trước mốc bị từ chối; công nợ giữ riêng hai bên theo đối tượng. Cột tiền lớn không ngắt dòng.
- Schema4 bổ sung metadata chuyển đổi và lịch sử phân bổ ban đầu; checksum/hash1–3 giữ nguyên. Backup/restore kiểm tra payload với ledger, ngân hàng, kho và tài sản. Đã kiểm tra migration từ3 có BCTC cũ, backup/restore bộ đã xác nhận, phát sinh bán hàng/khấu hao sau chuyển đổi, rollback/retry/cạnh tranh ghi và stale version/epoch/quyền.
- Validation: test Go cho ledger/operations/sqlite/httpserver; race cho các test Opening/Migration/Report/Posting và vet bốn package; TypeScript strict, production build,21 test Node. Edge dữ liệu giả lập: nhập9.007.199.254.740.993, lưu/mở lại sau reload, mở kỳ, xác nhận/khóa, đối chiếu dư đầu/phát sinh0 và bố cục form1440/390px; không dùng dữ liệu thật làm test. Không chạy Playwright CLI song song Edge.
- Giới hạn: chỉ khởi tạo trước chứng từ đầu tiên, chưa điều chỉnh số dư đã xác nhận, chưa có hóa đơn công nợ chuyển đổi để phân bổ thu/trả theo hóa đơn. Kho/tài sản ngoài156/211/214/242 cần phân hệ chi tiết tương ứng, hiện bị chặn thay vì nhận số tổng không đối soát được. Chưa benchmark dữ liệu lớn.

### Form chứng từ khác: hai bên Nợ/Có —0.4.1

- Chia chi tiết hạch toán thành hai nhóm đối xứng: TK Nợ/Có, đối tượng Nợ/Có, diễn giải Nợ/Có và số tiền Nợ/Có trên cùng hàng hiển thị. Có thêm/xóa riêng từng bên hoặc thêm cả hai; không báo cân đối khi chưa nhập tiền.
- Đây là cách trình bày form; giữ nguyên mảng dòng ledger, thứ tự nguồn, API, schema3 và quy tắc kiểm tra server. Không tự ghép bút toán đối ứng, tự điền tiền bên còn lại hoặc ép chứng từ nhiều Nợ/nhiều Có thành một cặp.
- Validation: TypeScript strict/build và18 test Node đạt. Edge với backend thật/dữ liệu giả lập: nhập hai TK/đối tượng/diễn giải riêng, tiền9007199254740993, thêm/xóa từng bên, lưu và kiểm tra API; mở/lưu lại nháp cũ1 Nợ/3 Có giữ đủ dữ liệu/thứ tự. Kiểm tra1440px và390px; mobile chỉ bảng cuộn ngang, nút lưu còn hiển thị. Đã cập nhật selectors E2E; chưa chạy lại Playwright CLI trong lượt này.

### Bản nháp BCTC thay tab doanh thu/chi phí — 0.4.0

- Thay tab trong ảnh bằng danh sách/lập BCTC theo bố cục tham chiếu AMIS; URL `tab=income` cũ mở BCTC, giữ NKC/sổ cái/cân đối phát sinh và API income tương thích.
- Bộ mẫu TT99 `tt99-2026-v1`: B01 (126 chỉ tiêu), B02 (21), B03 trực tiếp (27) hoặc gián tiếp (37), năm dương lịch từ2026, hoạt động liên tục. Chọn kỳ/báo cáo, nhập chi tiết hai cột, xem công thức/tổng tự tính, người ký/địa chỉ/thuyết minh bổ sung, kiểm tra, lưu/mở lại nháp, in và CSV.
- Server tính lại bằng big.Int, giữ trống khi thiếu thành phần, kiểm tra280=440 và liên báo cáo; tiền lớn/số âm chính xác, EPS tối đa2 chữ số lẻ. Source là bảng đối chiếu sổ toàn năm trong cùng snapshot, giữ dư hai bên theo đối tượng; không cộng riêng trang/100 chứng từ workspace.
- Schema3 chỉ thêm bảng bản nháp/index. Save dùng version, epoch, quyền, idempotency/audit/commit chung; có test đồng thời/retry/rollback/reopen, migration1/2→3, backup/restore nháp và chặn phiên cũ. Không đổi bút toán/policy/chart để tạo báo cáo.
- Validation: Go race cho ledger/sqlite/httpserver/operations, vet cho ledger/sqlite/httpserver; TypeScript strict/build và16 test Node đạt. OpenAPI53 operation khớp router/schema refs. Edge kiểm tra nhập tiền trên2^53, giữ qua chuyển tab và reload, công thức, lưu/mở lại, source toàn năm, EPS−0,25, bản in và bố cục1440/390px; thanh nút mobile đã sửa để nằm trong viewport. Dữ liệu QA hoàn toàn giả lập.
- Giới hạn: đây là bản nháp nhập tay, chưa có mapping tài khoản→BCTC đã thẩm định, chưa tự lấy số liệu/phân loại ngắn–dài hạn hoặc luồng tiền; B09 mới là nội dung bổ sung, chưa đủ các bảng pháp định. Chưa có TT133/TT200, mẫu giữa niên độ/không hoạt động liên tục, XML/nộp thuế/kiểm toán/phê duyệt. Không gọi phép kiểm tra số học là nghiệm thu pháp định.


### Biểu mẫu tham chiếu AMIS ngày 22/10/2026 — release 0.3.0

- Đã khảo sát 15 URL trong Edge, gồm các liên kết GL trong ảnh. Bố cục danh sách có toolbar/tổng/chi tiết dưới, biểu mẫu toàn màn hình có đối tượng/ngày/tổng, bảng nhiều dòng, vùng đính kèm và thanh thao tác cuối. Thương hiệu Oanh, Inter tự lưu trữ có OFL; Ant Design Form/Table hiện có.
- Phiếu thu/chi dùng chung, thu/chi ngân hàng dùng chung, chuyển nội bộ, mua/bán hàng hoặc dịch vụ; thông tin hóa đơn/metadata, nháp nghiệp vụ có version, xem trước/ghi sổ, bản đã post chỉ xem, in snapshot. Mua/bán thanh toán ngay tạo phiếu tiền liên kết atomic. Dữ liệu đang nhập giữ trong phiên, không persist trình duyệt.
- Bộ tham chiếu 184 tài khoản từ Google Sheet, có phiên bản/hash/dòng nguồn/cha con chuẩn hóa. Chỉ thêm mã thiếu, giữ tài khoản lịch sử; 1121 cũ vẫn dùng được. 621/622/623/627 chờ tập hợp, không tự kết chuyển vào911.
- Chiết khấu theo dòng/% hóa đơn/tổng tiền; chi phí mua hàng liên kết dòng dịch vụ đã ghi sổ, phân bổ theo lượng/giá trị/% tự nhập/giá trị tự nhập, chặn vượt hoặc trùng nguồn. Công khai quy tắc phần dư đồng; không suy đoán đó là thuật toán nội bộ AMIS.
- Danh mục ngân hàng/số dư riêng và nhiều kho; giá vốn FIFO/bình quân theo hàng–kho, lịch sử kho cũ vào Kho mặc định. Schema2 có kiểm tra migration/checksum/fingerprint; đọc backup schema1, migration bản chuẩn bị trước đổi pointer; đối chiếu chiều kho/ngân hàng/chi phí khi mở hoặc phục hồi.
- Validation: Go race/vet cho operations/ledger/identity/sqlite/httpserver; TypeScript strict/production build và15 test Node. Edge đã kiểm tra nháp, reload, tiền trên2^53/in, chuyển cùngTK112, bán dịch vụ CK/thu ngay, mua phân bổ phí, kết chuyển/khóa kỳ; kiểm tra bố cục trong design-qa.md. Bộ test mới có rollback, stale version, retry, reopen, backup/restore schema1→2, CK/làm tròn/phần dư và tách giá vốn hai kho. Không chạy lại Playwright CLI khi đang kiểm tra bằng Edge.
- Không tuyên bố đã sao chép đầy đủ mọi tính năng AMIS. Phạm vi và phần chưa triển khai được ghi rõ tại [ma trận 15 màn hình](reference/misa-screen-map.md); còn các nghiệp vụ trả lại/giảm giá sau hóa đơn, kho điều chỉnh/điều chuyển, nhập khẩu/ngoại tệ, tích hợp ngân hàng/HĐĐT và biểu mẫu pháp định. Chi phí nguồn chưa phân bổ hết chặn kết chuyển; chưa hỗ trợ phân bổ hồi tố sau xuất kho.


### Cải thiện UI/UX ngày 21/10/2026

- Bỏ footer nội dung và footer sidebar; đồng bộ Input, Select, InputNumber và Button ở chiều cao 40px qua theme Ant Design. Tăng cỡ chữ, độ tương phản, khoảng cách và chiều rộng điều hướng; thanh trên giữ khi cuộn, có khoảng chừa cho focus bàn phím.
- Gợi ý diễn giải và đơn vị tính từ dữ liệu đã tải, vẫn cho nhập nội dung mới. Tìm danh mục theo mã/tên có hoặc không dấu; nhãn đối tượng hiển thị cả mã và tên. Bố cục dòng mua/bán và bộ lọc tự xuống dòng trên màn hình nhỏ.
- Kiểm tra: TypeScript strict, production build, 15 test Node, 8 E2E mẫu và 6 E2E backend thật đạt; có kiểm tra chiều cao 40px, gợi ý không dấu, nhập tự do/giữ nháp và form mua hàng ở 1440/820/390px. Kiểm tra thêm 20 cặp route/viewport từ 375–1440px, reduced motion và skip link bàn phím không có lỗi JavaScript hoặc tràn ngang. Chưa benchmark hiệu năng mới; cảnh báo chunk Ant Design khoảng 700 kB vẫn còn.

### Hoàn thiện trải nghiệm tra cứu và nhập liệu

- Sidebar xanh đậm, trạng thái điều hướng rõ hơn, thẻ doanh thu có điểm nhấn và thao tác nhanh nằm đầu trang. Nhãn ghi sổ/nháp có độ tương phản chữ lần lượt 6,35:1 và 5,45:1 trên nền thực tế.
- Tìm chứng từ có gợi ý không dấu từ dữ liệu gần đây trong kỳ, chọn bằng bàn phím hoặc mở kết quả toàn kỳ. Màn hình nhỏ dùng hộp tìm kiếm riêng; danh sách chứng từ chuyển thành thẻ đủ đối tượng, trạng thái, ngày và số tiền, giữ phân trang và sắp xếp theo ngày/tiền. Nút lưu nháp giữ trong tầm nhìn khi nhập nhiều dòng.
- Validation: TypeScript strict, production build, 15 test Node, 11 E2E mẫu và 6 E2E backend thật đạt. Kiểm tra 24 cặp route/viewport ở 375–1440px, popup khi giảm chuyển động, đổi thiết lập chuyển động không mất nháp; không có tràn ngang, lỗi JavaScript hoặc lỗi console trong lượt kiểm tra bổ sung. Chưa benchmark hiệu năng mới; cảnh báo chunk lớn vẫn còn.

### Thu gọn giao diện theo mật độ thông tin

- Thống nhất chữ chính 13px, chữ phụ 12px, tiêu đề trang 20px và Input/Select/InputNumber/Button cao 32px. Giảm khoảng cách khung, thẻ, bảng, tab và biểu mẫu; thao tác nhanh nằm cùng hàng trên desktop. Giữ gợi ý nhập liệu và thanh lưu nháp khi cuộn.
- Tại viewport 1440×900 trên dữ liệu mẫu: đủ 8 dòng chứng từ cùng tổng tiền trong màn hình, thay cho 6 dòng trọn vẹn trước đó; chiều cao trang tổng quan giảm từ khoảng 1.565px xuống 1.113px. Đây là phép đo bố cục, chưa phải benchmark hiệu năng.
- Sửa CSS giảm chuyển động để tắt transition thay vì tạo transition ngắn cho mọi thuộc tính; menu sắp xếp không bị lệch vị trí ngoài viewport. Kiểm tra 28 cặp route/viewport từ 375–1920px không tràn ngang hoặc lỗi JavaScript/console; form ở 1440/820/390px giữ chiều cao control 32px.
- Validation: TypeScript strict, production build, 15 test Node, 11 E2E mẫu và 6 E2E backend thật đạt; chạy lại riêng 2 E2E tìm kiếm sau điều chỉnh padding popup. Cảnh báo chunk Ant Design khoảng 700 kB vẫn còn.

### Thu/chi và công nợ theo chứng từ — đối chiếu ảnh ngày 21/10/2026

- Tách Tiền mặt (111) và Tiền gửi (1121); thêm phiếu thu/chi tự động, chuyển tiền nội bộ và đảo phiếu tiền có nguồn. Dư đầu/thu/chi/dư cuối kỳ đọc cùng snapshot từ bút toán đã ghi sổ, độc lập bộ lọc và phân trang danh sách.
- Phân bổ một phiếu cho nhiều chứng từ mua/bán hoặc ghi tăng tài sản/242 có 331; hỗ trợ thanh toán từng phần, chặn vượt/sai đối tượng/sai loại và tổng phân bổ khác số phiếu. Đảo phiếu trả lại phân bổ theo ngày đảo, giữ lịch sử trước đó. Bút toán công nợ thủ công chưa phân bổ hoặc thanh toán sau ngày tra cứu được đánh dấu cần đối soát và chặn thanh toán tự động.
- Bổ sung số/ký hiệu/ngày hóa đơn và hạn thanh toán cho mua/bán, kiểm tra trùng trong nghiệp vụ tự động theo bên phát hành/ký hiệu/số. Có màn hình công nợ theo chứng từ, hạn quá hạn, tổng toàn bộ tập lọc và truy nguồn phân bổ. Không tích hợp phát hành/tra cứu hóa đơn điện tử.
- Giữ schema/migration hiện tại, không thêm dependency; payload nghiệp vụ, chứng từ, số phiếu, audit và chống trùng cùng transaction. Kiểm tra mở dữ liệu/backup đối chiếu phân bổ với phiếu tiền, đối tượng, nguồn và công nợ; từ chối payload sai tổng hoặc khác sổ. Thêm lối vào ghi tăng chi phí trả trước từ Mua hàng, kết chuyển và quản lý kỳ từ Kế toán tổng hợp.
- Validation: Go race/vet cho `operations`, `ledger`, `httpserver`; TypeScript strict, production build, 15 test Node, 7 E2E backend thật và 11 E2E mẫu đã đạt. Có kiểm thử thanh toán đồng thời, rollback, retry, số tiền trên 2^53, mở lại SQLite, thanh toán 242 và biên kỳ. Ảnh form và luồng mới kiểm tra ở desktop/mobile; chưa benchmark tập công nợ lớn. Cảnh báo chunk Ant Design lớn vẫn còn.
- Yêu cầu bổ sung đầy đủ **chưa hoàn tất**. Ma trận theo ảnh và phần còn thiếu tại [reference/business-coverage.md](reference/business-coverage.md): kiểm kê tiền, sao kê/đối chiếu, ngân hàng điện tử, dự báo, đơn/hợp đồng mua, trả lại/giảm giá, quyết toán tạm ứng, khóa theo loại chứng từ và BCTC pháp định. Người dùng chưa biết ngân hàng, nhà cung cấp hóa đơn điện tử hoặc chế độ BCTC; không tự chọn hoặc đổi chính sách hiện tại.

### Các phân hệ hiện có

| Phần | Chức năng có mã nguồn |
|---|---|
| Nền tảng | Go HTTP/API và file tĩnh cùng origin; SQLite STRICT, migration/checksum, WAL/FULL, per-connection FK/timeout, một writer và read pool 3; khóa tiến trình Unix |
| Truy cập | Khởi tạo chủ sở hữu, cookie phiên, hash mật khẩu, khóa đăng nhập tạm thời, owner/accountant/viewer, quản lý người dùng và thu hồi phiên, nhật ký |
| Cấu hình | Phiên bản tăng theo ngày hiệu lực; khai báo chế độ kế toán, VAT, FIFO/bình quân, scale, làm tròn, ngày/giờ công, giảm trừ, biểu TNCN, tỷ lệ BH/TNDN, đường thẳng và phần dư tháng cuối; xác nhận trước dùng |
| Danh mục/kỳ | Đối tượng, tài khoản, hàng/dịch vụ, nhân viên; giữ mã/lịch sử, ngừng sử dụng; mở/khóa kỳ; mở lại có đảo kết chuyển liên kết |
| Sổ kế toán | Nháp nhiều dòng, version, ghi sổ/cấp số/audit/nguồn/idempotency/epoch cùng transaction; chứng từ đã ghi sổ bất biến; đảo chứng từ thủ công |
| Mua/bán/kho | VAT mua/bán theo cấu hình, xác nhận khấu trừ đầu vào từng dòng, FIFO hoặc bình quân sau mỗi lần nhập, chống âm kho/lùi ngày/trộn phương pháp và scale khi còn tồn |
| Lương | Ngày công tổng hợp, phụ cấp, làm thêm, bảo hiểm theo cơ sở đã nhập, giảm trừ và TNCN lũy tiến theo cấu hình; lưu cơ sở lương/BH/người phụ thuộc lịch sử |
| Tài sản/242 | Ghi tăng TSCĐ hoặc khoản CCDC/242; đường thẳng theo tháng, kiểm tra thứ tự kỳ, phần dư tháng cuối |
| Giá thành/thuế/kết chuyển | Kết chuyển giá thành không vượt chi phí dở dang 154; TNDN tạm tính từ doanh thu/chi phí và điều chỉnh đã xác nhận, chặn khoảng chồng lấn; kết chuyển doanh thu/chi phí qua 911/421 rồi khóa kỳ |
| Tra cứu/báo cáo | Dashboard server, chứng từ theo kỳ/lọc/sort/phân trang/tổng toàn bộ tập lọc; NKC, sổ cái, cân đối phát sinh, dư Nợ/Có theo đối tượng, chi tiết doanh thu/chi phí; truy chứng từ và cơ sở tính/chính sách |
| File | CSV toàn bộ tập lọc và chống formula injection; bản in chứng từ/sổ; file đính kèm theo hash; import CSV qua preview, xác thực cả gói rồi lưu nháp atomic |
| Khôi phục | VACUUM INTO snapshot, ZIP/manifest/checksum, schema fingerprint/integrity/FK/cân đối/khớp kho/tài sản; staging xác minh, backup trước restore, giữ dataset cũ, atomic pointer, epoch mới, thu hồi phiên, nhật ký ngoài dataset |
| UI/UX | Giao diện desktop/mobile, lazy route, trạng thái tải/lỗi/rỗng, form nháp giữ khi chuyển trang, cảnh báo trước thoát, tiền lớn chính xác; API thật mặc định, mẫu chỉ khi bật env tường minh |

Tài khoản tự động dùng các vai trò hiện có và tài khoản tương thích trong bộ tham chiếu; chưa có mapping nghiệp vụ/báo cáo có phiên bản cho mọi chế độ. Khai báo chế độ kế toán trong policy không tự đổi chart/mẫu báo cáo. Thông số thuế/bảo hiểm là dữ liệu doanh nghiệp xác nhận, không phải bộ tham số pháp định đã được hệ thống cập nhật tự động. Không dùng kết quả lỗi hoặc tham số năm 2022 từ workbook làm chuẩn.

## Validation đã chạy

- Go: unit và integration trên SQLite file thật; `go test -race ./internal/... ./cmd/server` và `go vet ./internal/... ./cmd/server` đã đạt. Driver pin v1.60.1, runtime SQLite 3.53.4 trên Linux amd64.
- Các case: tiền trên 2^53/int64/làm tròn; reopen; posted immutability; rollback cả cấp số/audit/idempotency; gửi lại cùng intent và cạnh tranh ghi/khóa kỳ; stale version/epoch, trùng nguồn giữa nhập tay/CSV, cấu hình mọi read connection, từ chối tiến trình thứ hai.
- Nghiệp vụ: FIFO/bình quân với kết quả độc lập, nhiều dòng cùng hàng và âm kho rollback; policy tương lai không đổi giá tồn cũ; lương giả lập gross 2.600, thuế 185, net 2.315; cơ sở tính giữ sau đổi nhân viên; phân bổ 100 qua ba tháng 33/33/34; chi phí dở dang còn 400 không bị kết chuyển; TNDN giả lập 140, kết quả sau thuế 460, mở lại/kết chuyển lại không nhân đôi số.
- Báo cáo/import/recovery: tổng toàn bộ tập lọc và phân trang, cân đối hai bên, CSV bảo vệ cả metadata/header; preview không ghi và lỗi gói rollback; backup/restore có attachment, kiểm tra traversal/tampered staging, epoch mới và dữ liệu sau snapshot bị loại.
- Frontend: TypeScript strict và 15 test Node đạt. Playwright 7 luồng dữ liệu mẫu và 5 luồng backend thật đã đạt; bao phủ ghi nhận bán hàng, xem cơ sở tính, retry sau commit không trùng, tiền lớn, đổi cấu hình từ UI, backup và restore/đăng nhập lại/bỏ cache cũ, mobile 390px.
- Build frontend đạt, route tách chunk. Chunk chia sẻ Ant Design khoảng 700 kB / 227 kB gzip, Vite còn cảnh báo chunk trên 500 kB; chưa benchmark hiệu năng trên máy/dữ liệu mục tiêu.
- Ảnh desktop 1440px và mobile 390px đã được kiểm tra; bảng có vùng cuộn riêng. Kiểm tra console ở luồng khởi tạo/bán hàng không có lỗi JavaScript.
- OpenAPI ghi 36 operation thực tế; đã đối chiếu đủ method/path với router, kiểm tra các schema reference, Unicode NFC và liên kết tài liệu. Validation tiếng Việt trên 62 file: 0 lỗi, 2 cảnh báo định dạng ở fixture CSV cố ý giữ chuỗi số canonical. Lượt cuối Go race/vet, typecheck/build và 5 E2E backend thật đạt; hồi quy 7 E2E mẫu cũng đã được chạy.

## Chưa hoàn tất

- Chưa bao phủ toàn bộ nghiệp vụ của 84 sheet. Chưa có nhập số dư chuyển đổi có mốc/cutoff riêng, chart cha/con có phiên bản, quy đổi đơn vị giữa kho, kiểm kê/điều chuyển/trả lại/điều chỉnh giá hồi tố hoặc đảo nghiệp vụ phân hệ tổng quát.
- Đã có thu/chi và phân bổ thanh toán theo chứng từ; còn thiếu gắn phân bổ cho chứng từ tiền thủ công cũ, tuổi nợ theo nhóm/tổng hợp, ứng trước/bù trừ/chiết khấu sau hóa đơn, đối soát sao kê ngân hàng hoặc tích hợp hóa đơn điện tử. Đã có chiều số tài khoản/ngân hàng cho giao dịch mới; dữ liệu tiền cũ chưa gắn số tài khoản vẫn cần đối soát riêng.
- Lương nhận ngày công tổng hợp; chưa có bảng chấm công theo ngày, trần/điều kiện BH tự động, quy tắc miễn làm thêm theo từng đối tượng hoặc quyết toán TNCN năm. Chưa có thanh lý/điều chỉnh tài sản hay phân bổ giá thành nhiều dịch vụ theo tiêu thức.
- Chưa có bộ BCTC pháp định, lưu chuyển tiền tệ, mẫu tờ khai/XML và gửi cơ quan thuế, phiên bản mapping chế độ/tài khoản/báo cáo đã thẩm định. TNDN hiện là phép tạm tính một khoảng không chồng lấn, chưa xử lý nộp bổ sung/quyết toán và điều chỉnh tự động các khoảng đã ghi nhận.
- Chưa có parser XLSX, nhập workbook liên kết ngoài, lịch tác vụ định kỳ, lịch/retention backup hoặc dọn staging/blob mồ côi tự động.
- Chưa nghiệm thu production, tải lớn, bảo mật độc lập, Windows, mất điện/hết đĩa ở từng điểm chuyển dataset hoặc restore drill định kỳ. Không dùng kết quả test dữ liệu giả lập làm bảo đảm tuân thủ pháp lý hoặc số liệu doanh nghiệp thực tế.

## Dữ kiện và vận hành

- Workbook: 84 sheet, 60.135 ô công thức, 7.299 giá trị lỗi lưu sẵn, hai workbook liên kết chưa cung cấp; nhiều sheet là hướng dẫn/ví dụ/nhập tay. Phân tích/hash và ánh xạ tại `reference/`; chưa chạy macro hay tính lại workbook bằng Excel.
- Mã nguồn đợt 0.3.0 trên nhánh `codex/misa-voucher-workspace`; mốc trước là tag `v0.2.0`; repository `toanerrors/oanh-crm` qua remote `origin`. Không đưa DB, backup, mật khẩu hoặc dữ liệu tài chính thực vào mã nguồn.
- Test/E2E dùng thư mục tạm và dữ liệu giả lập. Hướng dẫn chạy, khởi tạo và giới hạn triển khai ở README; chi tiết API ở `api/openapi.yaml`; runbook recovery ở `backup-restore.md`.

## Bước tiếp theo

1. Nghiệm thu tập chứng từ/kết quả độc lập cùng kế toán phụ trách cho từng cấu hình thực tế.
2. Hoàn thiện chart/mapping báo cáo có phiên bản, số dư chuyển đổi và các nghiệp vụ điều chỉnh/trả lại; sau đó triển khai hồ sơ BCTC/thuế đúng kỳ.
3. Bổ sung nghiệp vụ công nợ, ngân hàng, chấm công và phân bổ chi tiết còn thiếu theo ma trận workbook; đo hiệu năng và thử khôi phục trên máy mục tiêu trước triển khai.
