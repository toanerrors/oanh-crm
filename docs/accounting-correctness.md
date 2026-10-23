# Quy tắc số liệu và ghi sổ

Tài liệu này là tiêu chí thiết kế và kiểm thử. Chưa có engine ghi sổ hoặc policy thuế đang chạy. Invariant phải có test độc lập với implementation; kết quả Excel có lỗi không phải chuẩn tham chiếu.

## 1. Hợp đồng số liệu

| Giá trị | Biểu diễn | Quy tắc |
|---|---|---|
| Số tiền đã chốt | DB INTEGER / Go int64 theo minor units | Có currency/scale; VND dùng scale 0; kiểm tra giới hạn trước khi ghi |
| Tiền qua JSON | Chuỗi integer minor units | Không qua JSON number hoặc float ở bất kỳ bước trung gian nào |
| Quantity, unit price, rate | Decimal chính xác; lưu TEXT canonical hoặc scaled INTEGER có schema rõ | Có scale/range và đơn vị; parser tập trung, cấm ép TEXT sang REAL để tính |
| Tính trung gian | Integer lớn hoặc rational/decimal chính xác | Giữ độ chính xác đến bước làm tròn đã định nghĩa |
| Mã định danh | Chuỗi | Giữ số 0 đầu, không tự suy diễn mã thiếu từ dạng hiển thị Excel |
| Ngày chứng từ/kế toán | Ngày lịch `YYYY-MM-DD` | Không dịch ngày vì timezone; kiểm tra kỳ tương ứng |
| Audit timestamp | RFC3339 UTC | UI hiển thị `Asia/Ho_Chi_Minh` |

Ví dụ 16.321.887 đồng gửi API là `"16321887"`, currency `VND`, scale `0`. Quantity `"1525.41"` và unit price `"10700"` cho giá trị trước thuế `"16321887"` nếu policy của nghiệp vụ không có điều chỉnh khác. Không truyền qua `float64` hoặc JavaScript `number` để tạo ra kết quả này.

Không có một scale hoặc phương pháp làm tròn mặc định áp dụng cho mọi nghiệp vụ. Trước khi dùng phải định nghĩa: đơn vị, độ chính xác nhập/tính/lưu, cách làm tròn, cấp dòng/chứng từ/kỳ và cách phân bổ chênh lệch. Thay đổi policy tạo phiên bản mới, không tính lại âm thầm chứng từ cũ.

`SUM()` trên INTEGER có thể overflow; `TOTAL()` luôn trả float. Không sửa lỗi overflow bằng cách cast REAL hoặc bỏ qua lỗi. Dùng giới hạn tổng đã kiểm chứng hoặc phép cộng chính xác phù hợp. [Tài liệu SQLite aggregate](https://sqlite.org/lang_aggfunc.html).

## 2. Các invariant phải giữ

1. Mỗi chứng từ đã ghi sổ có tổng Nợ bằng tổng Có chính xác theo đồng tiền hạch toán; từng dòng không đồng thời có Nợ và Có dương. Số tiền dòng là không âm; đảo bằng bút toán đối ứng/loại nghiệp vụ được định nghĩa.
2. Tài khoản tồn tại, còn hiệu lực và được phép ghi trực tiếp; tài khoản cần đối tượng phải có đối tượng đúng loại. Không ghi đồng thời vào tài khoản tổng hợp và con cho cùng khoản tiền.
3. Chứng từ nháp không tác động sổ, tồn hoặc số dư đã ghi nhận. Chuyển nháp → ghi sổ chỉ được thực hiện một lần với version hợp lệ.
4. Ghi sổ, cấp số, chi tiết phân hệ, audit và idempotency cùng commit hoặc cùng rollback. Một lỗi sau khi ghi header không được để lại biến động thiếu dòng.
5. Nguồn chứng từ và bút toán liên kết được; tổng hợp có thể dựng lại. Không chấp nhận một trường balance chỉnh tay để làm sổ khớp.
6. Kỳ khóa từ chối ghi/đảo/hồi tố trái quy tắc; cạnh tranh giữa khóa kỳ và ghi sổ phải có kết quả tuần tự hợp lệ.
7. Thanh toán phân bổ không vượt số tiền khả dụng hoặc công nợ theo chính sách; ứng trước và số dư hai bên được giữ rõ, không bù trừ sai đối tượng.
8. Kho giữ công thức tồn đầu + nhập − xuất = tồn cuối cho số lượng và giá trị theo policy. Giá tạm tính, điều chỉnh cuối kỳ và hàng trả lại phải có nguồn liên kết và trạng thái rõ.
9. Khấu hao/phân bổ lũy kế không vượt giá trị được phân bổ; tháng cuối xử lý phần dư chính xác và chạy lại không ghi trùng.
10. Tiền phân bổ theo các dòng phải cộng đúng tổng tiền phân bổ. Thuật toán chia dư có thứ tự ưu tiên xác định, bao phủ số âm/đảo nếu nghiệp vụ hỗ trợ.
11. Tổng báo cáo, dòng chi tiết và metadata dùng cùng phạm vi và snapshot. So sánh kỳ trước lấy đúng chế độ/policy và số dư tương ứng.
12. `null`, thiếu dữ liệu, lỗi, số 0 và số âm là trạng thái khác nhau. Thiếu tỷ giá, policy hoặc mã đối tượng phải trả lỗi/đưa vào vùng kiểm tra, không mặc định bằng 0.

## 3. Đồng tiền và thay đổi policy

- Bản đầu có thể chỉ bật VND, nhưng schema/hợp đồng phải biết đồng tiền và đơn vị biểu diễn. Hỗ trợ ngoại tệ chỉ triển khai sau khi chốt nguồn tỷ giá và các bước quy đổi/đánh giá.
- Nếu có ngoại tệ, giữ cả nguyên tệ, tỷ giá chính xác, số tiền quy đổi và policy/version; không suy ngược nguyên tệ từ số đã làm tròn.
- Thuế suất, giảm trừ, tỷ lệ bảo hiểm, điều kiện khấu trừ và mẫu báo cáo có khoảng hiệu lực không chồng lấn trái quy tắc. Chứng từ/kỳ chạy lưu phiên bản đã dùng.
- Thuế, kế toán và cách trình bày báo cáo là các quy tắc liên quan nhưng không đồng nhất. Khoản không được trừ khi tính TNDN vẫn phải được ghi nhận kế toán theo nghiệp vụ đúng.
- Mọi thay đổi làm khác kết quả đã ghi sổ cần migration/điều chỉnh có thể xem trước và đối soát; không chạy UPDATE hàng loạt tự động để áp policy mới cho dữ liệu cũ.

## 4. Nhập dữ liệu từ workbook

- Dùng bảng đối chiếu 84 sheet để nhận diện dữ liệu gốc, dữ liệu dẫn xuất, ví dụ và hướng dẫn. Chỉ nhập các tập đầu vào được xác nhận.
- Giữ batch ID, file hash, sheet, dòng nguồn, raw value và lỗi chuẩn hóa trong vùng kiểm tra có kiểm soát truy cập. Không đưa dữ liệu raw nhạy cảm vào log.
- Ngày Excel phải xét date system và cách lưu thực tế; MST/số tài khoản dạng numeric có thể đã mất số 0, không tự thêm khi thiếu bằng chứng.
- Ô công thức có giá trị lưu sẵn có thể cũ hoặc lỗi; không coi là dữ liệu gốc đã xác thực. Liên kết ngoài thiếu phải được ghi nhận và xử lý riêng.
- Import phải có bản xem trước số dòng, lỗi, trùng và tổng kiểm tra; schema/field mapping có phiên bản. Không thực thi VBA hoặc công thức không tin cậy.
- Chọn nhập lịch sử hoặc số dư chuyển đổi cho phạm vi tương ứng để tránh cộng hai lần. Nếu cho phép partial import, phải có chính sách batch và trạng thái đối soát rõ; mặc định một batch nghiệp vụ được ghi nhận nguyên tử trong giới hạn kích thước.

## 5. Bộ kiểm thử tối thiểu cho phần tương ứng

| Nhóm | Case cần có |
|---|---|
| Số chính xác | `0.1 + 0.2 = 0.3` trong kiểu exact, tiền > `2^53`, gần giới hạn int64, phép nhân trung gian vượt int64, số quá scale, giá trị âm/0 hợp lệ theo ngữ cảnh |
| Làm tròn/phân bổ | Tình huống nửa đơn vị theo từng policy, chia 100 đồng cho ba dòng, tháng cuối, đảo/điều chỉnh và tổng phần dư |
| Ghi sổ | Cân đối/không cân đối, tài khoản hoặc đối tượng sai, nháp không tác động sổ, rollback giữa chừng, sửa chứng từ đã ghi sổ bị từ chối |
| Cạnh tranh | Hai người ghi cùng nháp, cấp số đồng thời, khóa kỳ và ghi sổ cạnh tranh, cùng idempotency key khác payload, SQLite busy có deadline |
| Mạng/restart | Timeout sau commit, gửi lại cùng key, reopen DB giữ kết quả, job chạy lại không tạo trùng |
| Báo cáo | Chi tiết khớp tổng, nhiều trang không mất tổng, snapshot nhất quán, số dư đầu/cuối kỳ, phân loại LCTT, số dư đối tượng hai bên |
| Khôi phục | Backup khi có ghi, đủ attachment, restore staging, sai checksum/schema, lỗi giữa chừng, client/job cũ bị chặn bằng epoch |

Chỉ áp dụng test cho chức năng đang triển khai. Thay đổi logic tài chính cần regression test có kết quả kỳ vọng tính độc lập; không viết test chỉ lặp lại đúng thuật toán trong implementation.

## 6. Danh mục và chứng từ nguồn

Các yêu cầu sau là quyết định thiết kế bổ sung của dự án, để cụ thể hóa cách kiểm soát dữ liệu; không phải diễn giải điều khoản pháp luật.

- **Định danh:** tách ID kỹ thuật, mã danh mục và số chứng từ. UNIQUE có phạm vi đã định nghĩa; chuẩn hóa mã bằng quy tắc tường minh. Không suy ra loại đối tượng từ tiền tố KH/NCC hoặc dùng số dòng trong Excel/UI làm ID hay số chứng từ.
- **Tài khoản:** có ID, mã, tên, chế độ/phiên bản, tài khoản cha, tính chất số dư và quyền ghi trực tiếp. Thuộc tính dư Nợ/Có/hai bên phục vụ kiểm tra và trình bày, không phải lý do tự đảo dấu hoặc xóa số dư bất thường. Tài khoản tổng hợp lấy dữ liệu từ cây tài khoản của phiên bản liên quan.
- **Đối tác:** lưu vai trò khách hàng/nhà cung cấp rõ ràng; một đối tác có thể có nhiều vai trò. Không tự gộp các bản ghi chỉ vì tên gần giống hoặc MST trùng; việc hợp nhất có bằng chứng, mapping và kiểm tra công nợ liên quan.
- **Lịch sử danh mục:** mã đã được tham chiếu không được tái sử dụng cho thực thể khác; ngừng sử dụng thay vì xóa. Đổi tên/nhóm/giá mặc định không thay dữ liệu hoặc bản in lịch sử; dùng snapshot hoặc tham chiếu phiên bản để tái lập nội dung đã ghi nhận.
- **Nhiều dòng:** mô hình chứng từ hỗ trợ nhiều dòng Nợ/Có và nhiều đối tượng theo nghiệp vụ. Khi trình bày tài khoản đối ứng, giữ quan hệ từ bút toán nguồn; không join hai tập Nợ/Có thành tích chéo làm nhân số tiền.
- **Trùng nghiệp vụ:** phân biệt gửi lại cùng request với hai request/import khác nhau cùng phản ánh một sự kiện. Bổ sung ràng buộc nguồn theo loại sự kiện/nguồn/phiên bản được ghi nhận; cùng sự kiện không được tạo bút toán qua cả import và luồng nhập tay.
- **Đối chiếu hóa đơn:** định nghĩa khóa theo loại chứng từ, bên phát hành, ký hiệu/số và các trường cần thiết của định dạng áp dụng. Chỉ một số hóa đơn không đủ kết luận trùng; cùng hóa đơn có thể liên quan nhiều lần giao hàng/thanh toán hợp lệ. Khi dữ liệu chưa đủ xác định, đưa vào danh sách kiểm tra thay vì tự gộp hoặc bỏ qua.
- **Gợi ý và dữ liệu dẫn xuất:** mẫu hạch toán có phiên bản, nguồn và điều kiện áp dụng; chỉ tạo nháp để kiểm tra. Không tự ghi sổ từ gợi ý/AI. Server kiểm tra danh mục/hiệu lực, allowlist trường được sửa và tính lại trường dẫn xuất; dropdown hoặc ô readonly không thay thế kiểm soát phía server.

## 7. Số dư đầu kỳ và truy vấn sổ

### 7.1. Số dư chuyển đổi

- Mỗi lần khởi tạo số dư ghi rõ doanh nghiệp, đồng tiền, mốc chuyển đổi, nguồn và trạng thái xác nhận. Có chi tiết đối tác cho công nợ, ngân hàng cho tiền gửi, hàng/kho cho tồn và tài sản/khoản phân bổ khi cần.
- Số dư được xác nhận là một phần dữ liệu kế toán có kiểm soát và audit. Phân loại nó riêng khỏi phát sinh thông thường; không vừa nhập số dư tại mốc chuyển đổi vừa cộng lại toàn bộ lịch sử trước mốc đó.
- Chuyển tháng/năm không tự reset toàn bộ tài khoản. Chuyển số dư, kết chuyển doanh thu/chi phí và điều chỉnh theo chính sách kỳ đã chốt; mỗi sự kiện có nguồn và không chạy trùng.

### 7.2. Phạm vi và thứ tự đọc

- Trước khi tính sổ, xác định tuple phạm vi: doanh nghiệp, `dataset_epoch`, chế độ/phiên bản tài khoản, ngày kế toán, trạng thái đã ghi sổ, đồng tiền và các chiều đối tượng/kho/ngân hàng cần thiết.
- Khoảng thời gian nội bộ dùng đầu bao gồm, cuối không bao gồm `[from_date, to_date_exclusive)`. Với tháng 01/2026, phạm vi là từ `2026-01-01` đến trước `2026-02-01`. Không lọc bằng chuỗi tháng/năm hoặc lấy ngày tạo request thay ngày kế toán.
- Số dư đầu khoảng bằng số dư chuyển đổi hợp lệ cộng biến động trước ngày bắt đầu; phát sinh khoảng chỉ gồm dòng trong khoảng. Giữ phân loại opening/closing/adjustment để không cộng trùng bút toán chuyển đổi hoặc kết chuyển.
- Các phần của một báo cáo phải dùng cùng snapshot và cutoff revision. Xuất/in giữ metadata phạm vi và nguồn; không lấy bản nháp, dữ liệu của kỳ khác hoặc cache khác revision để lấp số thiếu.
- Sổ có thứ tự ổn định theo ngày kế toán, thứ tự ghi sổ bất biến và dòng bút toán. Phân trang phải giữ tiêu chí sort và phạm vi snapshot tương thích, tránh thiếu/trùng dòng khi có ghi mới.
- Sổ cái là view/truy vấn có tham số tài khoản. Rollup cộng đúng tập tài khoản được phép ghi trực tiếp theo cây tài khoản đã chọn; không dùng `LIKE '131%'` làm bằng chứng quan hệ cha/con hoặc cộng cả hàng cha đã tổng hợp với hàng con.
- Tài khoản/đối tượng không có giao dịch trong kỳ vẫn có thể có số dư đầu/cuối. Khi báo cáo yêu cầu hiển thị, truy vấn phải giữ các dòng này, không chỉ lấy tập ID xuất hiện trong phát sinh kỳ.

### 7.3. Công thức kiểm tra

Trong cùng đồng tiền, đơn vị và chiều chi tiết được đối chiếu:

```text
opening_net = opening_debit - opening_credit
movement_net = period_debit - period_credit
closing_net = opening_net + movement_net
```

Việc chuyển net thành cột dư Nợ/Có phải theo yêu cầu trình bày. Với số dư hai bên theo đối tượng, xác định số dư từng đối tượng trước rồi tổng hợp riêng Nợ và Có; không net toàn tài khoản trước khi phân loại.

Ví dụ kiểm tra thiết kế cho TK131, đơn vị đồng: A dư Nợ 100, B dư Có 40; kỳ này A phát sinh Có 20, B phát sinh Nợ 10. Cuối kỳ A dư Nợ 80, B dư Có 30. Tổng net là Nợ 50, nhưng không thay được thông tin Nợ 80/Có 30 trong báo cáo cần giữ hai bên. Kết quả này là bộ số giả lập để nghiệm thu, không phải số liệu doanh nghiệp.

## 8. Đối soát nhiều cấp và case bắt buộc

| Cấp kiểm tra | Điều kiện và cách xử lý |
|---|---|
| Cân đối tổng thể | Với bộ số dư mở cân đối và bút toán cân đối, tổng dư đầu Nợ/Có, tổng phát sinh Nợ/Có và tổng dư cuối Nợ/Có phải khớp theo phạm vi hợp lệ; không cộng lặp các cấp tài khoản |
| NKC và sổ cái | Cùng tập dòng đã ghi sổ và phạm vi phải cho cùng tổng phát sinh; mỗi dòng được tính đúng một lần, kể cả chứng từ nhiều dòng |
| Chi tiết và phân hệ | Sổ công nợ/tiền/kho/tài sản khớp tài khoản liên quan theo mapping, loại biến động và policy; chênh lệch hợp lệ phải có cầu nối giải thích, không sửa balance để làm khớp |
| Liên kỳ | Dư cuối khoảng trước là cơ sở dư đầu khoảng sau sau khi xét đúng bút toán kết chuyển/điều chỉnh; thay đổi so với bản đã chốt phải có dấu vết và nguyên nhân |
| Chứng từ và bằng chứng ngoài | Đối chiếu phiếu/hóa đơn, sao kê, kiểm kê và bảng tính định kỳ theo từng loại; cân đối Nợ/Có không chứng minh chứng từ có thật hoặc hạch toán đúng |

Mỗi đối soát cần giữ phạm vi, revision, tổng hai phía, chênh lệch, danh sách nguồn lệch và trạng thái xử lý. Khóa kỳ chỉ được thông qua khi các điều kiện bắt buộc theo policy đạt; trường hợp cần người xác nhận phải ghi quyền, lý do và bằng chứng, không tự đánh dấu đã khớp.

Thiếu tài khoản, mapping hoặc lỗi tính toán phải được báo rõ và chặn bước dùng kết quả không hợp lệ. Chỉ đổi tập rỗng thành số 0 khi truy vấn đã xác nhận tập đó hợp lệ; không mô phỏng `IFERROR(..., 0)` cho mọi lỗi.

Các case bổ sung cho bộ kiểm thử mục 5:

- Hai doanh nghiệp hoặc hai tài khoản có số tiền/ngày giống nhau vẫn được phân tách; tháng/năm đúng nhưng khác ngày kế toán không bị tính nhầm.
- Giao dịch ngay trước/ngay tại biên kỳ, năm nhuận, chứng từ nháp và chứng từ có liên kết đảo; xác định tập đưa vào sổ theo chính sách, không tự bỏ cả chứng từ gốc và chứng từ đảo.
- Tài khoản có số dư nhưng không phát sinh; opening không bị cộng hai lần; số dư hai bên TK131 theo ví dụ và đối chiếu tương tự cho TK331.
- Chứng từ nhiều Nợ/nhiều Có không bị nhân tiền; rollup tài khoản cha chỉ cộng đúng các dòng nguồn, không tính lặp cấp con.
- Hai idempotency key khác nhau nhưng cùng sự kiện nguồn không tạo trùng; hóa đơn khác bên phát hành nhưng cùng số không bị từ chối chỉ vì số trùng.
- Đổi tên/ngừng dùng danh mục không làm mất bản in lịch sử; đổi giá danh mục không tính lại chứng từ cũ; lỗi lookup không trở thành số 0.

## 9. Nguồn tham chiếu và giới hạn áp dụng

Đã đọc ngày 21/10/2026: [AccNet — Hướng dẫn làm sổ kế toán trên Excel](https://accnet.vn/huong-dan-lam-so-ke-toan-tren-excel). Bài nêu quy trình danh mục → chứng từ → nhật ký → sổ cái → báo cáo, cùng rủi ro lọc thiếu điều kiện và nhập trùng. Các mục 6–8 là đặc tả kỹ thuật do dự án bổ sung, cần nghiệm thu độc lập; không phải quy định pháp luật từ bài.

Không chuyển công thức Excel thành chính sách cấp số, không coi hàm xử lý lỗi là xác nhận dữ liệu đúng hoặc lấy ngưỡng chứng từ trong bài làm giới hạn hệ thống. Năng lực và độ chính xác phải được kiểm tra trên implementation thực tế.

Bài nhắc TT133/TT200; dự án không tự đổi chế độ kế toán theo một bài hướng dẫn. Bộ Tài chính xác nhận TT99/2025/TT-BTC áp dụng cho năm tài chính bắt đầu từ hoặc sau 01/01/2026 và thay thế TT200, với ngoại lệ về kế toán cổ phần hóa doanh nghiệp nhà nước. Cần chọn chế độ phù hợp phạm vi pháp lý và kỳ của doanh nghiệp, đồng thời giữ dữ liệu lịch sử. [Nguồn Bộ Tài chính](https://www.mof.gov.vn/tin-tuc-tai-chinh/tin-chinh-sach-tai-chinh/tap-huan-trien-khai-thong-tu-so-992025tt-btc).


## 10. Quy tắc hiện thực và bằng chứng ngày 21/10/2026

- Tiền chốt VND nguyên đồng, mỗi tổng chứng từ trong int64; tổng báo cáo dùng big.Int. Decimal có tối đa 15 chữ số nguyên/6 chữ số lẻ; lượng được chuyển về INTEGER theo scale 0–6 và từ chối phần lẻ vượt scale. Half-up/half-even/down áp dụng từng dòng theo policy xác nhận; không dùng float hoặc epsilon.
- Nguồn nghiệp vụ công khai được chuẩn hóa theo type; `manual` là alias của type, không phải namespace riêng cho kênh nhập. Chứng từ nguồn của kết chuyển/đảo/lương/tài sản chỉ do service tạo. Kiểm tra trùng nguồn giữa nháp nhập tay và CSV có integration test.
- VAT mua/bán tính lại theo đơn giá trước/sau thuế và tỷ lệ cấu hình; mua khấu trừ bắt buộc xác nhận điều kiện từng dòng, VAT không đủ điều kiện vào giá vốn/chi phí. Đây là phép tính theo tham số, chưa phải engine xác định điều kiện thuế từ hồ sơ pháp lý.
- FIFO lấy lô theo ngày/posting order; bình quân gom giá trị/số lượng sau lần nhập. Giá vốn phần xuất làm tròn theo policy, xuất hết lấy toàn bộ giá trị còn lại để không bỏ phần dư. Chặn âm kho, lùi ngày và thay phương pháp/scale trên tồn còn lại; cả policy lên lịch tương lai được kiểm tra tại thời điểm phát sinh.
- Lương tháng lấy cơ sở/ngày chuẩn/ngày công/phụ cấp/làm thêm, cơ sở BH đã xác nhận và biểu thuế theo policy. Cơ sở lương/BH/người phụ thuộc cùng input và policy được lưu khi commit, không tính lại lịch sử theo danh mục mới. Chưa tự xác định trần, đối tượng miễn giảm hoặc quyết toán năm.
- Khấu hao/242 chia phần giá trị cần phân bổ cho số tháng nguyên, phần dư vào tháng cuối; chạy từng kỳ liên tiếp. Case độc lập 100/3 cho 33, 33, 34. Chưa áp dụng cơ chế điều chỉnh/thanh lý giữa chừng.
- 154 thuộc nhóm chi phí dở dang, không phải chi phí kết chuyển kết quả. Giá thành hoàn thành 154 → 632 không vượt dư 154 đã ghi nhận; test nguồn 1.000, hoàn thành 600 còn 400 sau đóng kỳ. Chưa phân bổ theo nhiều đối tượng giá thành.
- TNDN tạm tính từ lợi nhuận trước thuế trên sổ, điều chỉnh tăng/giảm và lỗ chuyển đã nhập; không tự xác định tính hợp lệ của từng khoản. Chặn khoảng đã ghi nhận chồng lấn. Case giả lập doanh thu 1.000, chi phí 400, điều chỉnh tăng 100, tỷ lệ 20% cho thuế 140 và kết quả 460; các tỷ lệ này chỉ là dữ liệu kiểm thử.
- Kết chuyển cuối tháng qua 911/421 đồng thời khóa kỳ. Mở lại kỳ tạo đảo liên kết, giữ nguyên chứng từ gốc; đảo kết chuyển không được cộng vào doanh thu/chi phí hoạt động. Test mở lại/kết chuyển lại giữ nguyên kết quả 460, tài khoản 511/911 hết số dư.
- Snapshot sổ giữ cùng revision cho dòng/trang/tổng; tiền tổng không cộng từ riêng trang hiển thị. Recovery đối chiếu thêm lô/biến động kho và giá trị tài sản với ledger, không chỉ kiểm tra tổng Nợ/Có. Test/recovery dùng dữ liệu giả lập, chưa thay cho nghiệm thu bộ chứng từ của doanh nghiệp.

## 10. Nháp nghiệp vụ, chiết khấu, chi phí mua hàng và nhiều kho

Tự tính tiền khi nhập mua/bán dùng snapshot chỉ đọc và cùng hàm tính từng dòng/chiết khấu với ghi sổ. Không yêu cầu đủ thông tin đối tượng/hóa đơn/ngân hàng, không tạo chứng từ, biến động, audit hoặc tăng revision. Kết quả không thay bước kiểm tra ghi sổ; frontend không hiển thị kết quả thuộc nội dung cũ khi đang chờ tính lại. VAT mua đủ điều kiện khấu trừ hiển thị 1331; trường hợp không đủ điều kiện giữ quy tắc tính vào giá trị hàng mua/chi phí hiện có.

- Lưu/xem trước nháp không thay counters, operations, ngân hàng, tồn kho, tài sản hoặc trạng thái kỳ; nguồn nháp được giữ chống trùng. Version cũ bị từ chối; API nhập tay không bỏ qua service nghiệp vụ để post nháp phân hệ.
- Chiết khấu theo từng dòng hoặc % hóa đơn lấy giá trị dòng đã làm tròn theo policy, kiểm tra 0–100%, không vượt giá trị dòng. Chiết khấu tổng phải được phân bổ đủ. Giá chưa VAT: trừ CK rồi tính VAT. Giá gồm VAT: trừ CK khỏi giá thanh toán rồi tách VAT. Tổng bằng tổng các dòng chốt; thu/chi ngay đúng số này trong cùng transaction.
- Phân bổ đồng: phần nguyên của tỷ lệ chính xác, cấp số đồng dư theo phần lẻ giảm dần, cùng phần lẻ theo thứ tự dòng. Không epsilon, float hoặc ép TEXT qua SQL REAL. Chặn phương pháp phân bổ khiến CK một dòng vượt thành tiền; người lập chuyển sang tự nhập hoặc đổi tiêu thức.
- Chi phí mua hàng có nguồn dịch vụ đã post, ngày nguồn không sau hàng nhập, mỗi dòng nguồn không lặp trong một chứng từ; tổng sử dụng mọi chứng từ không vượt giá trị nguồn không gồm VAT đủ điều kiện khấu trừ. Tổng phân bổ vào từng hàng bằng tổng chọn từ nguồn. Tỷ lệ tự nhập cộng chính xác 100%. Giá trị phân bổ tự động dùng thành tiền sau CK trước VAT. Công nợ và VAT của nguồn được giữ riêng; bút toán nhập hàng Nợ156 thêm chi phí/Có TK nguồn tương ứng. Kết chuyển chặn khi còn nguồn chi phí chưa phân bổ hết.
- Giá vốn FIFO/bình quân, kiểm tra âm kho, scale, ngày cuối biến động và phần dư khi xuất hết đều theo item–warehouse. Không dùng tồn của kho khác để bù thiếu. Tồn cũ chuyển sang Kho mặc định với cùng số lượng/giá trị.
- Chuyển nội bộ cùng TK112 chỉ hợp lệ khi hai ID ngân hàng khác nhau; tổng sổ tiền gửi không đổi, chi tiết ngân hàng chuyển đủ hai phía. Đảo phiếu hoán đổi biến động gốc, giữ khả năng đối chiếu dù ngân hàng đã ngừng sử dụng.

Kết quả độc lập đã kiểm tra: giá 1.001, CK10%, VAT10% → CK100/base901/VAT90/tổng991; giá gồm VAT1.100 trừ CK100 → base909/VAT91/tổng1.000. Nguồn phí101 phân bổ tỷ trọng1:2 → 34/67. Hàng1.000 + phí101, VAT hàng100 đủ khấu trừ → tồn1.101, phải trả hàng1.100, giảm TK phí101; công nợ phí111 riêng. Hai kho cùng hàng giá100 và1.000 giữ giá vốn riêng ở cả FIFO/bình quân. Regression còn bao phủ tiền trên2^53, nháp/rollback/retry, reopen và backup/restore schema1→2.


## 12. Bản nháp BCTC TT99

Mẫu `tt99-2026-v1` đối chiếu Phụ lụcIV Mục1, B01 năm hoạt động liên tục, B02 và B03 trực tiếp/gián tiếp. [Công báo Chính phủ](https://congbao.chinhphu.vn/van-ban/thong-tu-so-99-2025-tt-btc-46529.htm); [bản phụ lục biểu mẫu do MISA lưu trữ](https://www.meinvoice.vn/wp-content/uploads/2025/10/Phu-luc-4-bieu-mau-bao-cao-tai-chinh.pdf), đọc22/10/2026. Năm bắt đầu từ2026, năm dương lịch; năm so sánh liền trước. Không tự đổi policy/chart của doanh nghiệp.

- Dòng chi tiết nhập tay sau đối chiếu; không có mapping tài khoản→chỉ tiêu đã thẩm định. B01 cột so sánh là dư đầu kỳ; B02/B03 là năm trước. Không bù trừ số dư đối tượng hoặc mặc định mọi dư131/331 là ngắn hạn.
- Ô trống giữ trống và tạo mục cần kiểm tra; chỉ tiêu tổng trống khi thiếu bất kỳ thành phần. Các phép cộng/trừ dùng big.Int/BigInt, không epsilon; server tính lại khi lưu. Chỉ tiêu (*) và dòng tiền chi trình bày âm, không tự đổi dấu. Tiền VND nguyên đồng; EPS nhập chính xác tối đa2 chữ số lẻ, không tự tính từ số cổ phiếu.
- Kiểm tra280=440 mỗi cột, B03.70=B01.110, B03 gián tiếp01=B02.50. Chỉ là kiểm tra số học nội bộ, không chứng nhận số liệu khớp sổ hoặc hồ sơ pháp định hoàn tất. B09 mới có vùng thuyết minh bổ sung, chưa có các bảng chi tiết pháp định.
- Source đọc cùng snapshot, trước ngày from là dư đầu, from≤ngày≤to là phát sinh; include cả kết chuyển/đảo kết chuyển như cân đối sổ. Đọc bút toán trực tiếp, cộng số dư từng đối tượng trước khi tổng hai bên. Không lấy riêng100 chứng từ workspace hoặc cộng riêng trang đang hiển thị.
- BCTC lưu ở trạng thái nháp, có version/nguồn tham chiếu; lỗi ghi/audit rollback cả dữ liệu và chống trùng. Không thay sổ hoặc trạng thái kỳ. Thay đổi sổ sau đó không tự sửa số đã nhập trong nháp; kế toán phải đối chiếu lại.

## 13. Số dư chuyển đổi ban đầu (0.5.0)

- Một bộ số dư cho pháp nhân/dataset, mốc là ngày đầu tháng. Nhập số cuối ngày trước mốc từ sổ/biên bản đã đối chiếu. Khởi tạo trước lần ghi sổ đầu tiên; không cộng số dư thêm vào bộ đã có lịch sử. Ngày chứng từ sau xác nhận không được trước mốc. Kỳ chuyển đổi phải mở, tổng Nợ=Có>0, không tự thêm dòng cân đối.
- Nháp có version, lưu bền và có thể bỏ khi chưa xác nhận. Chỉ chủ sở hữu lưu/xác nhận/bỏ nháp; đọc cho vai trò có phiên. Lệnh dùng epoch/idempotency/hash/audit và cùng transaction với ledger, ngân hàng, stock lot/movement và tài sản. Số đã xác nhận bất biến; không cho sửa, xóa hoặc đảo bằng chứng từ thủ công. Từ0.8.0 có luồng điều chỉnh riêng, giữ bất biến bản gốc; xem mục16.
- Tài khoản thường nhập từng dòng dương một bên Nợ hoặc Có, chi tiết đối tượng bắt buộc theo danh mục; tiền gửi chi tiết bankAccountId khớp tài khoản kế toán. Không bù trừ giữa các đối tượng. Doanh thu/chi phí/911 cần kết chuyển trên sổ nguồn trước khi chuyển số dư; không nhập chúng làm dư đầu kỳ.
- Kho hiện hỗ trợ hàng hóa156 theo item–warehouse, số lượng đúng scale và giá trị VND đã đối chiếu; ghi đồng thời lô tồn và biến động ban đầu theo chính sách đã xác nhận. TSCĐ211/214 và trả trước242 nhập nguyên giá, lũy kế, thu hồi, kỳ bắt đầu và tổng tháng. Số kỳ đã phân bổ trước chuyển đổi được lưu bất biến; kỳ sau tiếp tục đường thẳng trên nguyên giá gốc, phần dư kỳ cuối. Tài khoản kho/tài sản khác chưa có phân hệ chi tiết bị chặn, không nhận số tổng thay chi tiết.
- Nguồn `opening` thuộc dư đầu ngay tại kỳ chuyển đổi, không thuộc phát sinh, NKC, thu/chi tháng hoặc doanh thu/chi phí. Các kỳ sau lấy số đã xác nhận cùng phát sinh trước kỳ, không đặt lại số dư. BCTC source và báo cáo trước mốc từ chối thay vì trả0 cho lịch sử chưa nhập. Source BCTC chia hai bên theo đối tượng chỉ với tài khoản yêu cầu đối tượng.
- Công nợ đầu kỳ có chi tiết đối tượng, chưa có hóa đơn chuyển đổi và phân bổ thu/trả theo hóa đơn cho nguồn này. Phiếu hạch toán đối ứng131/331 thủ công được giữ trong sổ và cần đối soát, không tự gán hóa đơn nguồn.


## 14. Nhập chứng từ theo cặp Nợ/Có (0.5.1)

Form chứng từ khác có cặp TK/đối tượng Nợ và Có riêng, số tiền/diễn giải dùng chung một lần nhập. Tác động đúng hai dòng nguồn của cặp, giữ chuỗi tiền chính xác và thứ tự nguồn. Đây là cách nhập một cặp, không phải tự bù chênh lệch cho chứng từ nhiều đối ứng. Nháp nhiều Nợ/Có, khác số tiền hoặc khác diễn giải giữa hai bên giữ chế độ chi tiết một bên mỗi dòng; chỉ chuyển sang cặp khi số tiền/diễn giải từng cặp trùng chính xác. Dòng không có diễn giải dùng diễn giải chứng từ khi đọc sổ. Không đổi dữ liệu chứng từ đã ghi sổ, API, schema hoặc bước kiểm tra ghi sổ phía server.

## 15. Nhập Excel (0.6.0)

Invariant: upload/configuration không tạo bút toán; preview rollback toàn bộ; commit hoặc ghi toàn bộ danh mục/nháp+kết quả+audit/idempotency hoặc không ghi dòng nào. Không tự ghi sổ/xác nhận số dư. Dòng cùng nguồn phải nhất quán; cùng sự kiện nguồn không được nhập lại qua kiểu cặp/nhiều dòng/CSV/nhập tay. Cột bỏ qua phải được người dùng xác nhận. Danh mục cập nhật giữ cột tùy chọn chưa ghép, không tự đổi lịch sử. Số dư chỉ bổ sung chi tiết chưa có; không net công nợ hai bên.

Parser giữ raw string, kiểm tra tiền/decimal exact, định dạng số tường minh, ngày1900/1904; từ chối công thức ở cột ghép, lỗi ô và địa chỉ XML trùng/sai thứ tự. Kiểm thử SQLite thật bao phủ tiền trên2^53, lỗi một phần/trigger lỗi cuối transaction, retry và cạnh tranh commit, stale revision/epoch, quyền, tham chiếu phân bổ công nợ/chi phí mua hàng, backup/restore/reopen. Chi tiết và giới hạn tại [excel-import.md](excel-import.md).

## 16. Tài khoản con (0.7.0)

Cây danh mục dựa trên quan hệ ID đã lưu, không suy ra từ mã. Phiên bản quan hệ theo revision, baseline0 khi chuyển đổi; dữ liệu chứng từ/đối tượng/ngân hàng/kho và tên snapshot không đổi. Chặn vòng lặp/tự làm cha/vượt10cấp/cha không hiệu lực, stale parentRevision; nhánh có chứng từ giữ nguyên cha. Một lần lưu gồm tài khoản, quan hệ, revision, audit và khóa retry cùng commit. Nhập Excel dùng cùng validator; không ghép parentCode không xóa quan hệ. Metadata hierarchy hiện chưa được dùng rollup báo cáo hoặc thay quy tắc tài khoản ghi sổ.

SQLite thật kiểm tra seed theo ID khác mã, bảo toàn tên, migration5→6, cycle ở service/DB, nhiều cấp, cạnh tranh reparent, retry, khóa nhánh có bút toán lớn hơn2^53, backup/restore và reopen. Bộ cây frontend kiểm tra không đoán tiền tố, tìm con giữ tổ tiên, cấp và giới hạn nhánh được chọn làm cha.


## 16. Sửa số dư và thay thế phiếu tiền (0.8.0)

- Người dùng nhập số đúng sau điều chỉnh, không tự tính bút toán chênh lệch. Mốc chuyển đổi cố định; lý do bắt buộc. Nháp lưu bền/version; xác nhận kiểm tra lại revision/baseVersion/epoch/quyền và idempotency. Lỗi bất kỳ bước nào rollback toàn bộ cặp đảo–thay và chi tiết phân hệ.
- opening/opening_adjustment/opening_adjustment_reversal cùng được phân loại dư đầu, không cộng vào phát sinh/NKC/dòng tiền kỳ. Không net giữa các đối tượng/ngân hàng. Bản hiệu lực và carry được dựng từ toàn bộ ledger; tổng dùng big.Int. Gốc và lịch sử vẫn đối chiếu theo snapshot.
- Xác nhận hồi tố chỉ khi kỳ chuyển đổi mở và không còn kỳ đã khóa từ mốc trở đi. Sửa tiền độc lập với chi tiết kho/tài sản giữ nguyên. Kho đổi khi chưa có xuất/biến động âm/điều chỉnh giá trị liên quan; lô cũ về0, giữ movement gốc và thêm đảo–thay. Lô đầu kỳ ưu tiên trước lô mua cùng ngày. Không tự tính lại giá vốn đã xuất.
- Tài sản thêm/sửa số liệu/bỏ trước khi phân bổ; giữ mã/ID lịch sử. Tên có thể đổi sau phân bổ; giữ nguyên lũy kế hiện hành. Bỏ tài sản đầu kỳ không xóa nguồn, chỉ loại khỏi view hiệu lực. Khấu hao đọc elapsed đầu từ snapshot mới, cộng liên tiếp phát sinh sau mốc.
- Thu/chi/chuyển tiền: replacementOf cùng loại, lý do bắt buộc, ngày không trước gốc và phải ở kỳ mở. Đảo toàn bộ gốc rồi ghi bản mới cùng transaction; bản cũ, nguồn, ngân hàng/phân bổ công nợ vẫn truy được. Đây là phát sinh theo ngày sửa được chọn, khác với restatement số dư đầu kỳ. Một gốc chỉ thay thế một lần; sửa tiếp trên bản mới. Nháp/preview không tác động sổ.
- Mua/bán/tính giá vốn/lương/thuế có quy tắc điều chỉnh riêng; chưa triển khai thay thế trực tiếp qua đường sửa phiếu tiền. Không bật sửa/xóa tại chỗ trên chứng từ đã ghi sổ.


## 18. Xuất số liệu đa định dạng (0.10.0)

Invariant: xuất đọc toàn bộ tập lọc cùng snapshot với tổng; GET không ghi số liệu hoặc tăng revision. Revision/version cũ bị từ chối thay vì xuất dữ liệu khác bản đang xem. Mã luôn text; số nguyên tối đa15chữ số dùng int64, lớn hơn/decimal dùng text, không qua float; ô trống giữ trống. Formula injection không tạo công thức/link/macro trong XLSX; HTML dùng html/template và CSV giữ sanitizer. BCTC giữ trạng thái nháp/biểu mẫu/phiên bản nguồn/chỉ tiêu/thuyết minh theo bản lưu, không nhập lại các tổng từ client. Regression HTTP/SQLite và mở lại XLSX bao phủ2^53, int64,30chữ số, số âm/decimal/0/ô trống, phân trang, quyền, phiên bản và không đổi revision. Không thay quy tắc kế toán hoặc báo cáo pháp định.

## Khôi phục thao tác qua nhật ký (0.12.0)

Invariant: preview không thay revision/số liệu/audit; confirm lưu nghiệp vụ, audit trước/sau và idempotency trong một transaction. Thao tác thất bại không để lại snapshot/context. Sự kiện chỉ khôi phục nếu dữ liệu liên quan còn đúng trạng thái sau thao tác, chưa có thay đổi mới hơn và fingerprint/revision khớp. Giữ chính xác chuỗi tiền tới int64, kiểm tra lại tài khoản/đối tượng/kỳ/quyền bằng validator hiện có.

Chứng từ đã ghi sổ và số dư đã xác nhận bất biến; khôi phục không gọi đường ghi sổ ngầm. Dùng điều chỉnh/đảo có liên kết để sửa số liệu đã xác nhận. Khôi phục nháp bị bỏ phải tăng version vượt mọi bản đã tồn tại; client trước khi bỏ bị từ chối. Khôi phục danh mục mới chuyển active=false, không xóa/reuse mã lịch sử. Audit chỉ giúp truy vết thay đổi đã commit, không chứng minh nghiệp vụ ban đầu đúng tài khoản hoặc kỳ.

Regression bao gồm tiền lớn hơn 2^53, undo/redo, preview read-only, request gửi lại, lỗi ghi audit cuối transaction, hai người khôi phục đồng thời, client cũ, posted guard, owner/epoch, đầu kỳ/điều chỉnh nháp, BCTC và backup/restore. Nhật ký cơ bản thiếu snapshot không đủ bằng chứng khôi phục tự động.

## Tổng quan và chỉ số toàn kỳ (0.13.0)

Tổng quan giữ cùng nguồn/snapshot với workspace. Tổng tiền và tồn kho là tổng Nợ−Có trực tiếp theo nhóm tài khoản tới cuối kỳ, gồm lịch sử số dư/điều chỉnh. Không cộng giá trị cha đã tổng hợp với con, không suy diễn tồn kho vật chất. Phải trả là tổng dư Có của từng đối tượng, không bù với ứng trước đối tượng khác; phải thu giữ quy tắc sổ công nợ hiện có. Doanh thu/chi phí không cộng số dư đầu kỳ hoặc bút toán kết chuyển/đảo kết chuyển. Chênh lệch hiển thị chưa thay báo cáo kết quả kinh doanh.

Số nháp lệch đọc tất cả dòng nháp trong kỳ bằng số nguyên chính xác, không dựa vào100chứng từ workspace. Các chứng từ đầu kỳ/điều chỉnh/đảo đầu kỳ không vào số đếm hoạt động hoặc danh sách gần đây, vẫn tham gia số dư. Regression kiểm tra tổng tiền18446744073709551534đồng,127chứng từ kỳ/121nháp với bản lệch ngoài100dòng gần nhất; nguồn kho và điều chỉnh qua service thật, dữ liệu giữ sau reopen. Biểu đồ giữ đường0 cho số âm; số tiền ở chỉ số và nút tháng giữ chuỗi đầy đủ.

## Phân loại hàng hóa / dịch vụ dùng chung (0.14.0)

Trong phạm vi hiện tại, hàng hóa tracked=true có nhập/xuất và chi tiết kho; dịch vụ tracked=false dùng cho mua/bán dịch vụ, không yêu cầu kho và không tạo stock movement. Một ID danh mục dùng cho cả hai chiều mua/bán. UI chọn đúng loại và server kiểm tra lại TradeMode; không suy loại từ mã hoặc tên.

Dòng mua hàng được chọn tài khoản chi phí hoặc dở dang để mua dùng ngay, không tạo tồn kho. Chỉ dòng hàng hóa hạch toán156 tạo lô/biến động kho; bỏ trống tài khoản giữ mặc định156 cho hàng hóa và642 cho dịch vụ. Server không ghi đè tài khoản đã chọn; từ chối tài khoản sai nhóm/ngừng dùng và phân bổ chi phí nhập kho vào dòng không nhập kho. Nháp, retry, rollback và reopen phải giữ đúng tài khoản và đối soát kho–sổ. Chưa mở tài khoản tài sản/242 hoặc tài khoản kho khác156 qua đường mua hàng này.

Đổi loại chỉ áp dụng cho mã chưa được dùng, kiểm tra cả chứng từ nháp, nghiệp vụ đã ghi, lô/biến động và tồn kho trong số dư/điều chỉnh đầu kỳ trong cùng transaction cập nhật. Không sửa đơn vị/mã hoặc đổi cách đọc lịch sử. Regression SQLite kiểm tra mua/bán cùng mã, dịch vụ không có kho, sai loại, khóa tham chiếu, retry, audit undo/redo, backup/stage/reopen và cạnh tranh đổi loại với ghi sổ; chỉ một phía được thành công.


## 17. Sửa danh mục đã lưu

Invariant: lưu danh mục không mặc định khóa sửa. Chưa có tham chiếu được sửa mã và thông tin, chống mã trùng; ID ổn định. Có tham chiếu thì giữ trường định danh/mapping: ngân hàng giữ số và tài khoản kế toán, kho giữ mã, hàng giữ mã/đơn vị/loại, nhân viên giữ mã, tài khoản giữ mã/nhóm/yêu cầu đối tượng/tên, đối tượng giữ mã/tên/vai trò. Các mô tả an toàn, liên hệ và trạng thái vẫn sửa được theo từng danh mục. Nhân viên sửa lương/cơ sở BH cho lần tính sau, không tính lại chứng từ đã ghi.

Kiểm tra tham chiếu không giới hạn kỳ/trang và thực hiện lại trong transaction save; gồm nháp và chi tiết số dư/điều chỉnh đầu kỳ. Tên ngân hàng được lưu riêng trên chứng từ, không đồng bộ ngược khi đổi tên danh mục. Đối soát tiếp tục kiểm tra số tài khoản, mapping, biến động và sổ. Hoàn tác tạo audit mới và dùng lại validator, không bỏ qua tham chiếu đã xuất hiện sau thao tác cũ. Không sửa trực tiếp số liệu posted/confirmed hoặc xóa thật danh mục.
