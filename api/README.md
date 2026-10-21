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
