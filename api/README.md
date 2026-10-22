# Hợp đồng API đã triển khai

Đặc tả: [OpenAPI 3.1](openapi.yaml). Base path `/api/v1`, cùng origin frontend. Endpoint và DTO được viết bằng Go; chưa dùng code generation.

## Dữ liệu và xác thực

- JSON UTF-8 dùng camelCase. Tiền VND là chuỗi số nguyên không âm trong int64; tổng báo cáo là chuỗi số nguyên chính xác có thể vượt int64. Decimal là chuỗi dùng dấu chấm, tối đa 15 chữ số nguyên và 6 chữ số lẻ; số lượng còn phải khớp scale của chính sách. Không nhận JSON number cho tiền/decimal.
- Context gồm `companyId`, `companyName`, `epoch`, `revision`, `currency=VND`, `scale=0`, `regime`; danh sách/sổ thêm `period`. `regime` trong context là thông tin khởi tạo; phiên bản chính sách giữ riêng chế độ áp dụng cho nghiệp vụ tự động.
- Ngày kế toán `YYYY-MM-DD`; timestamp UTC RFC3339. Cookie phiên `oanh_session`, HttpOnly, SameSite Strict, hạn 12 giờ; Secure khi bật cấu hình HTTPS. Không lưu token ở localStorage.
- Chủ sở hữu quản lý cấu hình, tài khoản, nhân sự, người dùng, kỳ, audit và recovery. Kế toán lập chứng từ/nghiệp vụ và xem cơ sở tính. Tài khoản chỉ xem tra cứu sổ và file; không xem dữ liệu cấu hình/lương chi tiết từ endpoint automation/calculation.
- JSON body tối đa 1 MiB, từ chối trường lạ và dữ liệu JSON nối tiếp. Sort chỉ `date`/`amount`, direction `asc`/`desc`; pageSize tối đa 100, page tối đa 1.000.000. Lọc tìm kiếm hỗ trợ tiếng Việt không dấu.

## Lệnh và chống trùng

Lệnh lưu/ghi sổ/đảo/danh mục/kỳ/người dùng/nhập nghiệp vụ/đính kèm gửi `Idempotency-Key` và `X-Dataset-Epoch`. Server kiểm tra epoch, quyền, version, kỳ và khóa chống trùng trong cùng transaction. Cùng actor/operation/key/payload trả kết quả đã commit; đổi payload với key cũ bị 409. Khóa và payload hash lưu bền vững.

`POST /operations/preview` tính toàn bộ trong transaction rồi rollback. `POST /operations` bắt buộc gửi `expectedPolicyId` và `expectedRevision` từ kết quả xem trước. Server tính lại; thay đổi dữ liệu/chính sách làm từ chối với `VERSION_CONFLICT`. Kết chuyển cuối kỳ chỉ chủ sở hữu thực hiện.

Chứng từ thủ công dùng sourceType khớp type; `manual` được chuẩn hóa về type. Mã nguồn phải thống nhất giữa nhập tay, CSV và nghiệp vụ tự động. Không dùng namespace của kênh nhập để né kiểm tra trùng. Các loại nguồn kết chuyển/đảo/lương/tài sản được giữ cho service nghiệp vụ.

Backup nhận epoch, tạo và kiểm tra gói trước khi trả ZIP; retry có thể tạo thêm một bản backup. Restore stage nhận ZIP, chỉ kiểm tra vào vùng tạm. Restore activate nhận `stageId`, `confirmCompanyId` và epoch; tạo backup trước chuyển, chặn request qua cổng dữ liệu, đổi epoch và thu hồi phiên. Hai endpoint này không dùng hợp đồng idempotency của lệnh tài chính.

## Sổ và import/export

`workspace` trả danh mục, chỉ số do server tính và tối đa 100 chứng từ gần nhất của kỳ. Không dùng danh sách rút gọn này để tính tổng sổ. `documents` và `reports` trả trang hiện tại cùng tổng của toàn bộ tập lọc trong cùng snapshot; revision tùy chọn chặn việc đọc lệch phiên bản. Sổ gồm journal, ledger, trial, debt và income (chi tiết doanh thu/chi phí).

Xuất/in lấy tối đa 100.000 chứng từ/dòng, từ chối khi vượt giới hạn. CSV có metadata, chống ô công thức, tiền thêm hậu tố VND; bản in HTML escape dữ liệu người dùng. Các báo cáo này chưa phải tờ khai thuế hoặc mẫu BCTC pháp định.

Import nhận CSV trong JSON, header bắt buộc:

```csv
sourceType,sourceId,type,date,description,partnerId,accountId,linePartnerId,lineDescription,debit,credit
```

Tối đa 500 chứng từ, 5.000 dòng/gói. Các dòng cùng nguồn tạo một nháp; phải có header chung nhất quán. Preview kiểm tra trong transaction rollback; commit cần expectedRevision và ghi cả gói hoặc không ghi dòng nào. Không thực thi công thức/macro; chưa nhận XLSX. Chứng từ kho/tài sản phải đi qua nghiệp vụ phân hệ.

## Lỗi

Response lỗi có `code`, `message`, `request_id`. HTTP 400: cấu trúc/tham số; 401: phiên; 403: quyền; 404: tài nguyên; 409: version/epoch/idempotency/nguồn/lịch sử; 422: quy tắc nghiệp vụ; 429: khóa đăng nhập tạm thời; 500: lỗi nội bộ với thông báo không lộ SQL/path. Log dùng request ID, không log mật khẩu/body tài chính.

Các mã thường gặp: `UNBALANCED_ENTRY`, `PERIOD_CLOSED`, `PERIOD_FINALIZED`, `VERSION_CONFLICT`, `IDEMPOTENCY_CONFLICT`, `DATASET_CHANGED`, `DUPLICATE_SOURCE`, `AMOUNT_OVERFLOW`, `POLICY_REQUIRED`, `STOCK_POLICY_LOCKED`, `INSUFFICIENT_STOCK`. Không đổi kết quả commit thành thất bại chỉ vì request tải lại dữ liệu lỗi.

## Thu/chi và thanh toán theo chứng từ

`POST /operations/preview` và `/operations` hỗ trợ thêm `receipt`, `payment`, `transfer`, `treasury_reversal`, dùng cùng cơ chế xem trước/chính sách/revision/epoch/chống trùng. Thu/chi nhận `moneyAccount` là `111` hoặc `1121`, `amount` là số nguyên VND dương. Thu/chi khác nhận `counterAccount` do người lập xác nhận; không tự tách thuế. Kho, tài sản, công nợ và chuyển tiền dùng luồng nghiệp vụ tương ứng.

Thanh toán nhận `partnerId` và `allocations: [{ documentId, amount }]`; server xác định tài khoản 131/331. Tổng phân bổ bằng số tiền phiếu; mỗi nguồn chỉ xuất hiện một lần, cùng đối tượng, đúng loại và không sau ngày thanh toán. Nguồn phải được quản lý bởi nghiệp vụ mua/bán/ghi tăng tài sản; chứng từ thủ công cần đối soát riêng. Mọi phân bổ cùng transaction với phiếu tiền. `OVERPAYMENT`, `ALLOCATION_MISMATCH`, `INVALID_INVOICE`, `UNALLOCATED_SETTLEMENT` là lỗi nghiệp vụ 422. Không tự phân bổ bút toán công nợ cũ hoặc suy đoán chiết khấu/chênh lệch.

Chuyển tiền nhận `moneyAccount` và `destinationAccount` khác nhau. Đảo phiếu nhận `originalDocumentId`, ngày và diễn giải lý do; đảo toàn bộ, giữ liên kết và trả lại phân bổ theo ngày đảo. Phiếu gốc vẫn còn trong lịch sử, cùng nguồn không được đảo lần hai. Chưa kiểm soát hạn mức thấu chi hoặc âm quỹ riêng theo chính sách.

`GET /trade-invoices?asOf=YYYY-MM-DD&kind=sale|purchase&partner=&page=1&pageSize=10` trả công nợ từ chứng từ mua/bán và ghi tăng tài sản có 331. Số đã phân bổ chỉ gồm phân bổ đến ngày tra cứu chưa bị đảo tại ngày đó. Có thanh toán sau ngày tra cứu hoặc bút toán giảm công nợ ngoài luồng sẽ đánh dấu `unallocated` và chặn thanh toán tự động; số còn lại của dòng này chưa phải số dư công nợ đã đối soát. Công nợ từ chứng từ khác vẫn xem bằng báo cáo theo đối tượng. Tổng toàn bộ tập lọc dùng số nguyên lớn; giới hạn tập 100.000 chứng từ.

Mua/bán nhận thông tin hóa đơn tùy chọn: `invoiceNumber`, `invoiceSeries`, `invoiceDate`, `dueDate`. Nhập số hóa đơn phải có ngày; hạn thanh toán không trước ngày hóa đơn. Kiểm tra trùng theo bên phát hành + ký hiệu + số trong các nghiệp vụ tự động đã lưu, lỗi `DUPLICATE_INVOICE` trả 409. Chưa xác minh hóa đơn với nhà cung cấp dịch vụ bên ngoài, không thay cho khóa định danh hóa đơn điện tử pháp định đầy đủ.

## Biểu mẫu, danh mục tham chiếu và chiều ngân hàng/kho (0.3.0)

- `POST /voucher-drafts`, `GET /documents/{id}/voucher` lưu/đọc toàn bộ input thu/chi/chuyển nội bộ/mua/bán. Nháp có draftId/draftVersion, được server tính kiểm tra rồi rollback biến động trước khi lưu. Post bằng operations, không post nháp này qua API thủ công. Kết chuyển có biểu mẫu xem/ghi sổ riêng, không lưu nháp nghiệp vụ.
- `GET /documents/{id}/print` in snapshot chứng từ, tên tài khoản lúc lưu và cơ sở tính; không dùng tổng cộng riêng trang frontend. Metadata voucher nhận ngày chứng từ, người liên hệ, địa chỉ, nhân viên, tham chiếu, hóa đơn và ghi chú.
- `GET/POST /accounts/reference`: đọc bộ 184 tài khoản người dùng chỉ định, hoặc chủ sở hữu áp dụng đúng version để thêm mã thiếu. Không sửa tên/nhóm tài khoản đã có.
- `GET /bank-accounts?period=YYYY-MM`, `POST /bank-accounts`: danh mục và dư đầu/thu/chi/dư cuối từng ngân hàng, số tài khoản/tên/TK kế toán bất biến. Biến động mới dùng bankAccountId; TK tiền được suy ra/kiểm tra. Cho chuyển giữa hai ngân hàng khác ID cùng TK112. Bộ lọc documents hỗ trợ bank; bank không tự gán cho dữ liệu cũ.
- `GET/POST /warehouses`: danh mục và tồn hiện tại theo từng kho; dòng hàng nhận warehouseId, bỏ trống tương thích Kho mặc định. Giữ mã/tên kho đã tạo.
- Mua/bán nhận discount (none/line/percent/total), tỷ lệ hoặc tiền CK từng dòng, tiêu thức quantity/value/manual. Giá/VAT và phần dư tính trên server; kết quả lineAmounts có base/tax/total/discount/purchaseCost/purchaseValue.
- `GET /purchase-costs?asOf=YYYY-MM-DD`: các dòng dịch vụ đã đánh dấu isPurchaseCost và phần chưa phân bổ. Mua nhập kho nhận purchaseCosts[{documentId,line,amount}], costAllocation (manual/quantity/value/percent), purchaseCost hoặc costPercent từng dòng tương ứng. Chặn vượt nguồn, sai ngày, lặp nguồn, lệch tổng và tỷ lệ không đủ100%; chặn kết chuyển khi còn phí chưa phân bổ.
- Thanh toán ngay nhận paymentAccount và bankAccountId nếu qua ngân hàng; chứng từ mua/bán, kho, phiếu tiền và phân bổ công nợ commit hoặc rollback cùng nhau. Kết quả trả paymentDocument để truy phiếu liên kết.

47 method/path đã được mô tả trong OpenAPI. Dữ liệu tài chính vẫn là chuỗi; không thêm production dependency. Giới hạn nghiệp vụ và nguồn AMIS ở [ma trận màn hình](../docs/reference/misa-screen-map.md).


## Bản nháp BCTC (0.4.0)

Sáu operation `/financial-reports`: template, danh sách/đọc theo ID, preview, lưu, source và print (GET/POST danh sách là hai operation). Mẫu `tt99-2026-v1` giữ các chỉ tiêu B01/B02/B03 trực tiếp hoặc gián tiếp của năm dương lịch từ2026, hoạt động liên tục; B01 bắt buộc. Chỉ tiêu chi tiết nhập tay; không tự lấy số dư tài khoản làm BCTC. B09 hiện là vùng thuyết minh bổ sung, chưa có toàn bộ bảng pháp định.

Preview không ghi; save dùng version/idempotency/epoch và transaction chung, cho phép nháp còn thiếu và trả `issues`. Tổng được server tính lại bằng big.Int, thiếu thành phần giữ trống. Ô tiền tối đa30 chữ số nguyên, EPS tối đa2 chữ số lẻ. `sourceRevision` là phiên bản tham chiếu dữ liệu, không phải bằng chứng đã khớp; lần lưu riêng báo cáo có thể tăng cùng global revision. Source đọc đúng khoảng tối đa1 năm và cùng snapshot; dư Nợ/Có giữ theo đối tượng, không rollup bằng tiền tố. Phát sinh gồm kết chuyển nên không dùng trực tiếp làm doanh thu/chi phí. Print yêu cầu đúng version đã lưu; CSV từ snapshot server đã trả, có phòng formula injection. Không có XML, nộp thuế, kiểm toán hoặc phê duyệt pháp định.
