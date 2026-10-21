import { useState } from 'react';
import { Alert, Button, Collapse, Descriptions, Input, Tag } from 'antd';
import { Link, useLocation } from 'react-router-dom';
import { useWorkspace } from '../../app/WorkspaceProvider';
import { normalizeSearch } from '../../shared/accounting/money';
import { PageHeading } from '../../shared/ui/Page';
import { Icon } from '../../shared/ui/Icon';
import styles from '../../app/App.module.css';
import { isDemo } from '../../shared/api/client';
import { Administration } from './Administration';

const guides = [
  { key: '1', title: 'Bắt đầu với không gian kế toán mẫu', text: 'Chọn kỳ kế toán ở phía trên để xem chứng từ và số liệu của kỳ đó. Mọi màn hình dùng cùng bộ dữ liệu giả lập. Dữ liệu mẫu không phải số liệu doanh nghiệp thực tế. Danh tính “Minh Anh” chỉ dùng để minh họa giao diện.' },
  { key: '2', title: 'Tạo chứng từ và nhập nhiều dòng hạch toán', text: 'Chọn “Tạo chứng từ”, nhập loại, ngày và diễn giải. Ở mỗi dòng, chọn tài khoản, đối tượng nếu bắt buộc và nhập số tiền ở một bên Nợ hoặc Có. Dùng Tab để di chuyển giữa các ô; “Thêm dòng hạch toán” để bổ sung. Ô tiền chỉ nhận số nguyên VND, không nhận số thập phân.' },
  { key: '3', title: 'Giữ nháp khi chuyển trang và bảo vệ nội dung đang nhập', text: 'Đóng ngăn chứng từ để giữ nội dung trong bộ nhớ của phiên. Chọn “Tiếp tục nháp” ở phía trên để mở lại. Ứng dụng chỉ giữ một biểu mẫu đang nhập; hãy lưu hoặc bỏ nháp hiện tại trước khi mở nháp khác. Tải lại hoặc đóng ứng dụng sẽ mất dữ liệu trong phiên; dùng “Xuất nháp” để giữ bản JSON trước khi thoát.' },
  { key: '4', title: 'Phân biệt bản nháp và chứng từ đã ghi sổ', text: 'Bản nháp chưa làm thay đổi sổ, số dư hoặc tổng quan. Có thể lưu nháp lệch Nợ/Có để bổ sung sau; giao diện luôn hiển thị chênh lệch. Các chứng từ có nhãn “Đã ghi sổ” được nạp sẵn từ bộ dữ liệu mẫu. Chức năng ghi sổ thật chỉ được bật khi có backend kiểm tra nghiệp vụ, kỳ, quyền và transaction.' },
  { key: '5', title: 'Tra cứu, đối chiếu và xuất số liệu', text: 'Trong Sổ sách & báo cáo, chọn Nhật ký chung, Sổ cái hoặc Cân đối phát sinh. Bấm số chứng từ để truy ngược nguồn. Báo cáo chỉ lấy chứng từ mẫu đã ghi sổ. Nút “Xuất CSV” xuất toàn bộ kết quả đang lọc, kèm kỳ và phiên bản. Các ô tiền xuất kèm VND để giữ dạng văn bản; không dùng CSV mẫu làm dữ liệu kê khai.' },
  { key: '6', title: 'Quản lý khách hàng và nhà cung cấp', text: 'Vào Danh mục để thêm đối tượng hoặc sửa thông tin liên hệ. Mã không được trùng. Mã và tên đối tượng đã có lịch sử được giữ nguyên trong phiên mẫu. Ngừng sử dụng sẽ ẩn đối tượng khỏi danh sách chọn trên chứng từ mới, đồng thời giữ lịch sử.' },
];
const liveGuides = [
  { key: '1', title: 'Thiết lập trước khi ghi sổ', text: 'Vào Cài đặt, mở kỳ kế toán và tạo phiên bản chính sách có ngày hiệu lực. Xác nhận phương pháp VAT, giá xuất kho, thông số lương, bảo hiểm, thuế và cách làm tròn đang áp dụng. Phiên bản mới không thay đổi chứng từ lịch sử. Danh mục tài khoản khởi tạo cần được rà soát theo chế độ kế toán doanh nghiệp.' },
  { key: '2', title: 'Lập nghiệp vụ tự động', text: 'Thêm đối tượng, hàng hóa/dịch vụ hoặc nhân viên cần thiết. Vào Nghiệp vụ tự động, chọn ngày, mã sự kiện nguồn và nhập dữ liệu. Chọn Tính và xem trước để kiểm tra số tiền và các dòng hạch toán. Khi xác nhận, máy chủ tính lại và ghi đồng thời chứng từ cùng biến động phân hệ. Nếu dữ liệu thay đổi sau khi xem trước, cần tính lại.' },
  { key: '3', title: 'Lưu nháp và ghi sổ thủ công', text: 'Tạo chứng từ nhiều dòng, chọn tài khoản và đối tượng, nhập số nguyên VND ở một bên Nợ hoặc Có. Lưu nháp để giữ trên máy chủ; nháp chưa ảnh hưởng sổ. Mở chi tiết rồi chọn Ghi sổ sau khi kiểm tra. Chứng từ đã ghi sổ không sửa trực tiếp; chứng từ thủ công được sửa sai bằng chứng từ đảo liên kết. Tiền, thanh toán công nợ và các khoản chi có thể hạch toán bằng chứng từ thủ công.' },
  { key: '4', title: 'Bảo vệ nội dung và xử lý lỗi kết nối', text: 'Biểu mẫu chưa lưu được giữ trong bộ nhớ khi chuyển trang, nhưng mất khi tải lại hoặc đóng trình duyệt. Chọn Lưu nháp hoặc Xuất nháp trước khi thoát. Khi mất phản hồi, giữ nguyên nội dung và thử lại để hệ thống kiểm tra kết quả theo cùng khóa chống trùng. Không tự tạo nghiệp vụ mới để thay cho yêu cầu chưa rõ kết quả.' },
  { key: '5', title: 'Tra cứu, đối chiếu và xuất số liệu', text: 'Sổ sách lấy dữ liệu đã ghi sổ. Bộ lọc và tổng toàn bộ tập lọc do máy chủ tính trong cùng phiên bản dữ liệu. Bấm số chứng từ để truy nguồn, xem và tải file đính kèm. Xuất CSV giữ chính xác số tiền, có kỳ và phiên bản. Bảng tính thuế theo cấu hình hỗ trợ nghiệp vụ nội bộ; cần đối chiếu trước khi lập hồ sơ kê khai.' },
  { key: '6', title: 'Khóa kỳ, mở lại kỳ và khôi phục', text: 'Chủ sở hữu quản lý kỳ và người dùng trong Cài đặt. Kết chuyển tự động khóa kỳ. Mở lại kỳ tạo chứng từ đảo liên kết cho kết chuyển trước; sau điều chỉnh phải kết chuyển lại. Backup chứa database và file đính kèm, được kiểm tra trước khi tải. Khôi phục cần xem trước gói và xác nhận; hệ thống tạo bản sao hiện tại, đổi phiên dữ liệu và yêu cầu đăng nhập lại.' },
];

export function Component() {
  const { pathname } = useLocation();
  return pathname === '/help' ? <Help /> : isDemo ? <Settings /> : <Administration />;
}

function Help() {
  const [search, setSearch] = useState('');
  const { newDraft } = useWorkspace();
  const filtered = (isDemo ? guides : liveGuides).filter(item => normalizeSearch(`${item.title} ${item.text}`).includes(normalizeSearch(search)));
  return <>
    <PageHeading title="Trung tâm trợ giúp" description="Những hướng dẫn ngắn để làm việc tự tin hơn với từng con số." />
    <div className={styles.helpHero}><span className={styles.heroIcon}><Icon name="book" size={30} /></span><h2>Bắt đầu đơn giản, làm việc rõ ràng.</h2><p>Tìm hướng dẫn về chứng từ, danh mục và tra cứu số liệu.</p><Input prefix={<Icon name="search" size={18} />} placeholder="Bạn cần tìm hướng dẫn gì?" aria-label="Tìm hướng dẫn sử dụng" allowClear value={search} onChange={event => setSearch(event.target.value)} /></div>
    <div className={styles.helpGrid}><div className={styles.panel}><div className={styles.panelHeader}><h2>Hướng dẫn sử dụng</h2><span className={styles.softLabel}>{filtered.length} bài viết</span></div><Collapse ghost items={filtered.map(item => ({ key: item.key, label: <strong>{item.title}</strong>, children: <p className={styles.helpText}>{item.text}</p> }))} />{!filtered.length && <p className={styles.emptyHelp}>Không có hướng dẫn phù hợp. Hãy thử từ khóa khác.</p>}</div><aside className={styles.helpSide}><div className={styles.panel}><Icon name="spark" size={26} /><h3>Thử một chứng từ đầu tiên</h3><p>Tạo nháp, thêm dòng hạch toán và kiểm tra cân đối ngay trong giao diện.</p><Button type="primary" onClick={() => newDraft()}>Tạo bản nháp <Icon name="arrow" size={16} /></Button></div><div className={styles.panel}><h3>Thao tác bàn phím</h3><p><kbd>Tab</kbd> Di chuyển đến ô tiếp theo</p><p><kbd>Shift + Tab</kbd> Quay về ô trước</p><p><kbd>Esc</kbd> Đóng ngăn đang mở</p><p><kbd>Enter</kbd> Tìm chứng từ ở thanh trên</p></div></aside></div>
  </>;
}

function Settings() {
  const { data, period } = useWorkspace();
  return <>
    <PageHeading title="Thiết lập không gian" description="Thông tin và phạm vi hoạt động của phiên trải nghiệm hiện tại." />
    <Alert showIcon type="info" title="Phiên trải nghiệm giao diện" description="Dữ liệu được giữ trong bộ nhớ, không lưu vào cơ sở dữ liệu hoặc trình duyệt. Các chức năng nghiệp vụ thực tế cần backend và các chính sách đã duyệt." className={styles.formAlert} />
    <div className={styles.settingsGrid}><section className={styles.panel}><div className={styles.panelHeader}><h2>Thông tin không gian</h2><Tag color="cyan" variant="filled">Dữ liệu mẫu</Tag></div><Descriptions column={1} className={styles.settingsDescription} items={[
      { key: 'company', label: 'Không gian', children: 'Doanh nghiệp mẫu Oanh' },
      { key: 'currency', label: 'Đồng tiền mẫu', children: 'VND · Đơn vị đồng · Không tự tính thuế' },
      { key: 'period', label: 'Kỳ đang xem', children: period.split('-').reverse().join('/') },
      { key: 'epoch', label: 'Phiên dữ liệu mẫu', children: data?.epoch ?? '—' },
      { key: 'revision', label: 'Phiên bản dữ liệu', children: data?.revision ?? '—' },
      { key: 'storage', label: 'Lưu trữ', children: 'Bộ nhớ phiên · Mất khi tải lại ứng dụng' },
      { key: 'identity', label: 'Người dùng hiển thị', children: 'Minh Anh · Danh tính minh họa' },
    ]} /></section><section className={styles.panel}><div className={styles.panelHeader}><h2>Phạm vi chức năng</h2></div><div className={styles.capabilities}>{[
      ['Tổng quan và tra cứu sổ từ chứng từ mẫu', true], ['Tạo, sửa nháp nhiều dòng, kiểm tra Nợ/Có', true], ['Thêm đối tượng, tìm kiếm và xuất CSV', true], ['Ghi sổ và lưu dữ liệu bền vững', false], ['Đăng nhập, phân quyền và khóa kỳ', false], ['Thuế, lương, tính giá kho, khấu hao', false], ['Backup và khôi phục SQLite', false],
    ].map(([label, implemented]) => <div key={String(label)}><Icon name={implemented ? 'check' : 'clock'} size={17} /><span>{label}</span><small>{implemented ? 'Có trong phiên mẫu' : 'Cần backend / chính sách'}</small></div>)}</div><div className={styles.settingsHelp}><Link to="/help">Xem hướng dẫn và giới hạn phiên mẫu <Icon name="arrow" size={15} /></Link></div></section></div>
  </>;
}
