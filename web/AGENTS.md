# Hướng dẫn frontend

Áp dụng cùng `../AGENTS.md`. Giao diện phục vụ nhập liệu kế toán nhiều dòng, thao tác bàn phím và tra cứu số liệu có nguồn gốc.

## Stack và ranh giới

- React + TypeScript strict + Vite SPA; pnpm; TanStack Query cho dữ liệu API. Giữ một QueryClient ổn định ở cấp ứng dụng.
- React Router đã được chọn cho điều hướng SPA. Route loader, nếu dùng, tái sử dụng `queryOptions`/QueryClient để không tạo đường tải dữ liệu hoặc cache thứ hai.
- Ant Design là bộ UI đã chốt: layout/menu/tab/modal/date picker; Ant Design Form cho biểu mẫu/validation/dòng nhập động; Ant Design Table cho danh sách và sổ sách. Dùng theme token và CSS Modules để tùy chỉnh thống nhất.
- Form state do Ant Design Form quản lý, tách khỏi Query cache. Không chồng thêm thư viện form/UI/table khác khi chưa có nhu cầu được chứng minh. AG Grid là phương án đánh giá riêng cho yêu cầu nhập liệu kiểu Excel; kiểm tra tính năng Community/Enterprise và license trước khi áp dụng.
- `src/app/`: provider, router, app shell. `src/features/<feature>/`: page, component, form, query key/options và mutation của phân hệ.
- `src/shared/`: API transport, formatter chính xác và UI dùng chung có nhu cầu thật. Kiểu API sinh từ hợp đồng sau khi công cụ được chọn; không sửa file sinh tự động.
- API client dùng `fetch`, nhận `AbortSignal`, xử lý lỗi có mã và chuỗi số liệu. Không cần thêm Axios chỉ để bọc HTTP.
- Chỉ thêm global store khi có state UI dùng chung không phù hợp context/local state. Dữ liệu API không sao chép vào store riêng.

## TanStack Query và tính đúng

- Định nghĩa query key factory theo feature. Key phải chứa mọi biến ảnh hưởng kết quả, gồm người dùng/phạm vi quyền, doanh nghiệp, `dataset_epoch`, kỳ, filter, sort và trang.
- Truyền `signal` từ query function xuống `fetch`. Không fetch dữ liệu API bằng `useEffect` song song với Query.
- Cấu hình `staleTime`, `gcTime`, retry và refetch tường minh theo loại dữ liệu. Khởi điểm: danh mục 60 giây; số dư/chứng từ đang hoạt động 0 giây; cache inactive 5 phút. Đây là cấu hình hiển thị để đo và điều chỉnh, không bảo đảm dữ liệu hiện hành tại thời điểm ghi.
- Refetch khi focus/reconnect cho dữ liệu nghiệp vụ đang xem; giữ bộ lọc và form đang sửa. Các phiên/tab khác cần kiểm tra revision hoặc refetch để thấy thay đổi, vì invalidation chỉ tác động QueryClient hiện tại.
- Query retry tối đa hai lần cho lỗi mạng/5xx tạm thời; không retry lỗi validation, quyền hoặc conflict. Mutation tài chính đặt `retry: false`.
- Tạo idempotency key cho một ý định ghi; retry cùng payload dùng cùng key. Thay payload là ý định mới. Gửi version và `dataset_epoch` đã đọc; xử lý 409 bằng tải lại/đối chiếu.
- Không optimistic-update ghi sổ, thanh toán, tồn kho, số dư, khóa kỳ hoặc restore. Chờ phản hồi đã commit từ server.
- Sau thành công, cập nhật bản ghi authoritative rồi `await` invalidate các query bị ảnh hưởng: danh sách/chi tiết chứng từ và sổ, kho, công nợ, tiền, báo cáo tương ứng. Không invalidate toàn bộ ứng dụng cho mọi thao tác.
- Nếu đã commit nhưng refetch thất bại, thông báo đã lưu và dữ liệu đang chưa tải lại; không báo lưu thất bại hoặc tự gửi mutation lần nữa.
- `keepPreviousData`/placeholder chỉ dùng trong cùng phạm vi hợp lệ; đánh dấu dữ liệu cũ và không cho thao tác dựa trên trang cũ như trang mới. Không giữ dữ liệu cũ qua thay đổi doanh nghiệp, quyền, dataset hoặc kỳ khi có thể gây hiểu sai.
- Khi logout/đổi danh tính, hủy request và xóa cache nhạy cảm. Khi restore làm đổi epoch, chặn mutation cũ và tải lại context dữ liệu trước khi tiếp tục.

## Số liệu và biểu mẫu

- Giữ tiền, quantity, unit price, rate dưới dạng chuỗi chính xác hoặc kiểu exact đã thống nhất; không ép sang `number` để hiển thị, sort hoặc tính tổng.
- Định dạng tiền bằng thao tác chuỗi/BigInt hoặc thư viện exact đã được chấp thuận; `Intl.NumberFormat` chỉ dùng khi bảo toàn độ chính xác. Không truyền số tiền lớn qua `Number`.
- Ô tiền dùng component `MoneyInput` chung bọc Ant Design `InputNumber` với `stringMode`; giá trị form và callback luôn giữ chuỗi chính xác hoặc giá trị rỗng đã định nghĩa. Parser/formatter không ép sang number; kiểm tra scale/range và hành vi blur, step, wheel để tránh tự đổi tiền ngoài ý định người dùng.
- Tách chuỗi nhập theo locale `vi-VN` và giá trị canonical gửi API. Không đoán dấu phân cách trong dữ liệu import mơ hồ.
- Backend tính lại mọi tổng số. UI phân biệt số nháp/tạm tính, dữ liệu cũ, dữ liệu đang tải và kết quả đã ghi sổ. `null`, chưa có dữ liệu và lỗi không hiển thị như số 0.
- Tổng toàn bộ tập lọc nhận từ API; tổng của trang hiện tại phải được đặt tên rõ ràng. Không tính báo cáo từ số đã làm tròn để hiển thị.
- State nháp gắn với document ID và version, không gắn vào query cache như số đã lưu. Không reset dirty form khi background refetch trả về.
- Chuyển tab giữ filter, scroll và nháp khi phù hợp, có giới hạn số form/tab được giữ. Nhắc khi rời form chưa lưu; mặc định không lưu lương/chứng từ nhạy cảm vào localStorage.

## Mượt và nhẹ có đo lường

- Giữ app shell ổn định; lazy-load route theo phân hệ; lazy-load riêng thư viện Excel/PDF/chart nặng. Prefetch route/query có chọn lọc khi hover/focus hoặc theo ý định người dùng.
- Không tải tất cả phân hệ lúc khởi động hoặc giữ toàn bộ tab trong DOM. Dùng cache dữ liệu và state nháp có giới hạn để quay lại nhanh.
- Phân trang/lọc/sort phía server cho danh sách lớn; virtualization khi số hàng cần hiển thị thực sự lớn. Chọn một bộ table/virtualization, tránh cài nhiều bộ tương đương.
- Giữ dữ liệu hiện tại khi refetch đúng phạm vi; hiện tiến độ nhẹ. Dùng skeleton cho lần tải đầu; có empty/error/retry rõ ràng và không chớp số 0.
- Dùng debounce cho tìm kiếm tốn request; không debounce việc cập nhật input nhập liệu. Không dùng animation dài che thời gian tải dữ liệu.
- Không dùng `memo`, `useMemo` hoặc dependency nặng chỉ vì phỏng đoán. Đo bundle, React render và mạng trên bảng chứng từ đại diện.
- Khi chunk cũ không tải được sau deploy, giữ nháp và thông báo phiên bản mới; không tự reload làm mất dữ liệu chưa lưu.
- Ưu tiên HTML semantic, label, focus, tab bàn phím và thông báo lỗi cạnh field. Điều hướng không làm mất focus ngoài ý muốn; hỗ trợ reduced motion.

## Kiểm tra khi có ứng dụng

- Chạy typecheck và test ảnh hưởng trước. Hiện dùng Node test runner (`pnpm test`) và Playwright (`pnpm test:e2e` cho mẫu, `pnpm test:e2e:live` cho backend thật); chưa có lint script, Vitest hoặc React Testing Library.
- Test frontend cần bao phủ chuỗi tiền lớn hơn `2^53`, formatter/locale, cache key, invalidation, lỗi commit so với lỗi refetch, version conflict và đổi epoch/phạm vi dữ liệu.
- E2E trọng tâm: ghi sổ đúng một lần khi double-click/mất mạng; chuyển trang/tab không mất nháp; không lộ dữ liệu cũ khi đổi phạm vi; restore yêu cầu tải lại context.
- Chỉ báo UI mượt hoặc đạt hiệu năng khi đã đo; chưa có benchmark thì ghi rõ chưa đo.
- Trước khi nhân rộng UI, kiểm tra một màn hình chứng từ nhiều dòng với Ant Design Form/Table: số tiền lớn, chọn tài khoản/đối tác, bàn phím, thêm/xóa dòng, chuyển tab giữ nháp và refetch không ghi đè nội dung đang sửa.
