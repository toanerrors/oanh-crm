import { lazy, Suspense, useEffect, useState } from 'react';
import { Avatar, Badge, Button, Drawer, Dropdown, Modal, Select, Tooltip } from 'antd';
import { Link, NavLink, Outlet, useLocation, useNavigate, useNavigation } from 'react-router-dom';
import { useWorkspace } from './WorkspaceProvider';
import { Icon } from '../shared/ui/Icon';
import type { IconName } from '../shared/ui/Icon';
import styles from './App.module.css';
import { api, apiScope, isDemo } from '../shared/api/client';
import { searchOptions } from '../shared/ui/search';
import { DocumentSearch } from './DocumentSearch';
import { accountingModule } from './module-navigation';

const DocumentEditor = lazy(() => import('../features/documents/DocumentDrawer').then(module => ({ default: module.DocumentEditor })));
const DocumentDetail = lazy(() => import('../features/documents/DocumentDrawer').then(module => ({ default: module.DocumentDetail })));

const navigation: { path: string; label: string; icon: IconName; group?: string }[] = [
  { path: '/', label: 'Tổng quan', icon: 'grid', group: 'KHÔNG GIAN LÀM VIỆC' },
  { path: '/cash', label: isDemo ? 'Tiền mặt & ngân hàng' : 'Tiền mặt', icon: 'wallet', group: 'NGHIỆP VỤ' },
  ...(!isDemo ? [{ path: '/bank', label: 'Tiền gửi', icon: 'building' as const }] : []),
  { path: '/purchases', label: 'Mua hàng', icon: 'bag' },
  { path: '/sales', label: 'Bán hàng', icon: 'cart' },
  { path: '/inventory', label: 'Hàng tồn kho', icon: 'box' },
  { path: '/receivables', label: 'Công nợ', icon: 'users' },
  { path: '/documents', label: 'Kế toán tổng hợp', icon: 'book' },
  ...(!isDemo ? [{ path: '/automation', label: 'Nghiệp vụ tự động', icon: 'spark' as const }] : []),
  { path: '/reports', label: 'Sổ sách & báo cáo', icon: 'chart', group: 'QUẢN LÝ' },
  { path: '/catalog', label: 'Danh mục', icon: 'folder' },
];

export function AppShell() {
  const { period, setPeriod, data, draft, voucherDraft, setDraftOpen, detailId, refresh } = useWorkspace();
  const [mobileOpen, setMobileOpen] = useState(false);
  const [notificationsOpen, setNotificationsOpen] = useState(false);
  const location = useLocation();
  const navigate = useNavigate();
  const navigationState = useNavigation();
  const module = accountingModule(location.pathname);
  const current = navigation.find(item => item.path === (module?.path ?? location.pathname))?.label ?? module?.title ?? (location.pathname.startsWith('/vouchers/') ? 'Chứng từ' : location.pathname === '/settings' ? 'Thiết lập' : 'Trợ giúp');
  const person = isDemo ? 'Minh Anh' : apiScope().name;
  const initials = person.split(' ').slice(-2).map(value => value[0]).join('').toUpperCase();
  const draftCount = data?.server?.draftCount ?? data?.documents.filter(item => item.status === 'draft' && item.date.startsWith(period)).length ?? 0;
  const drafts = data?.documents.filter(item => item.status === 'draft' && item.date.startsWith(period)) ?? [];

  useEffect(() => { document.title = `${current} · Oanh`; }, [current]);

  const sidebar = <div className={styles.sidebarContent}>
    <Link to="/" className={styles.brand} aria-label="Oanh, về tổng quan"><span className={styles.brandMark}><span /><span /><span /></span><span>oanh<span className={styles.brandDot}>.</span></span></Link>
    <button className={styles.companySwitch} onClick={() => navigate('/settings')}><span className={styles.companyIcon}><Icon name="building" size={20} /></span><span><strong>{isDemo ? 'Không gian mẫu' : data?.companyName ?? 'Doanh nghiệp'}</strong><small>Kế toán doanh nghiệp</small></span><Icon name="down" size={14} /></button>
    <nav className={styles.navigation} aria-label="Điều hướng chính">{navigation.map(item => <div key={item.path}>{item.group && <div className={styles.navGroup}>{item.group}</div>}<NavLink to={item.path} end={item.path === '/'} onClick={() => setMobileOpen(false)} className={({ isActive }) => `${styles.navLink} ${isActive ? styles.activeNav : ''}`}><Icon name={item.icon} size={19} /><span>{item.label}</span>{item.path === '/documents' && draftCount > 0 && <span className={styles.navCount}>{draftCount}</span>}</NavLink></div>)}</nav>
    <div className={styles.sidebarBottom}>
      <div className={styles.helpCard}><span className={styles.helpIcon}><Icon name="spark" size={18} /></span><strong>Kế toán nhẹ nhàng hơn</strong><p>Bắt đầu từ một chứng từ.<br />Kết nối mọi con số.</p><Link to="/help" onClick={() => setMobileOpen(false)}>Khám phá hướng dẫn <Icon name="arrow" size={15} /></Link></div>
      <NavLink to="/settings" className={({ isActive }) => `${styles.navLink} ${isActive ? styles.activeNav : ''}`} onClick={() => setMobileOpen(false)}><Icon name="settings" size={19} />Thiết lập</NavLink>
    </div>
  </div>;

  return <div className={styles.app}>
    <a href="#main-content" className={styles.skipLink}>Chuyển đến nội dung</a>
    <aside className={styles.sidebar}>{sidebar}</aside>
    <Drawer className={styles.mobileDrawer} placement="left" open={mobileOpen} onClose={() => setMobileOpen(false)} size={280} title="Điều hướng" styles={{ body: { padding: 16 } }}>{sidebar}</Drawer>
    <div className={styles.workspace}>
      <header className={styles.topbar}>
        <Button className={styles.mobileMenu} type="text" icon={<Icon name="menu" />} aria-label="Mở điều hướng" onClick={() => setMobileOpen(true)} />
        <div className={styles.topbarActions}>
          <DocumentSearch />
          <Tooltip title="Hướng dẫn sử dụng"><Button type="text" icon={<Icon name="help" />} aria-label="Hướng dẫn sử dụng" onClick={() => navigate('/help')} /></Tooltip>
          <Badge dot={draftCount > 0} offset={[-7, 6]}><Button type="text" icon={<Icon name="bell" />} aria-label="Xem thông báo" onClick={() => setNotificationsOpen(true)} /></Badge>
          <div className={styles.headerDivider} />
          <Dropdown menu={{ items: [{ key: 'settings', label: isDemo ? 'Thông tin không gian mẫu' : 'Cài đặt doanh nghiệp', onClick: () => navigate('/settings') }, { key: 'help', label: 'Hướng dẫn sử dụng', onClick: () => navigate('/help') }, { key: 'refresh', label: 'Tải lại dữ liệu', onClick: refresh }, ...(!isDemo ? [{ key: 'logout', label: 'Đăng xuất', onClick: () => { void api('/auth/logout', { method: 'POST' }).then(() => window.location.reload()); } }] : [])] }} trigger={['click']}><button className={styles.profile}><Avatar size={34} style={{ background: '#e4eee8', color: '#3f6651', fontWeight: 700 }}>{initials}</Avatar><span>{person}<small>{isDemo ? 'Kế toán viên · Mẫu' : ({ owner: 'Chủ sở hữu', accountant: 'Kế toán', viewer: 'Chỉ xem' }[apiScope().permissions as 'owner'])}</small></span><Icon name="down" size={12} /></button></Dropdown>
        </div>
      </header>
      {navigationState.state !== 'idle' && <div className={styles.routeProgress} role="progressbar" aria-label="Đang chuyển trang" />}
      <main id="main-content" className={`${styles.main} ${!isDemo && module ? styles.denseWorkspace : ''}`} tabIndex={-1}>
        <div className={styles.contextBar}><span><span className={styles.liveDot} /> Không gian kế toán của bạn</span><div>{voucherDraft && <Button type="text" onClick={() => navigate(voucherDraft.draftId ? `/vouchers/${voucherDraft.draftId}` : '/vouchers/new')}>Tiếp tục phiếu đang nhập</Button>}{draft && <Button type="text" onClick={() => setDraftOpen(true)} icon={<Icon name="file" size={14} />}>Tiếp tục nháp</Button>}<span className={styles.periodLabel}>Kỳ kế toán</span><Select showSearch={searchOptions} aria-label="Chọn kỳ kế toán" value={period} onChange={setPeriod} className={styles.periodSelect} options={Array.from({ length: isDemo ? 12 : 84 }, (_, index) => { const year = Number(period.slice(0, 4)) + (isDemo ? 0 : Math.floor(index / 12) - 3); const month = String(index % 12 + 1).padStart(2, '0'); return { value: `${year}-${month}`, label: `Tháng ${month}, ${year}` }; })} suffixIcon={<Icon name="down" size={13} />} /></div></div>
        <Outlet />
      </main>
    </div>
    <Suspense fallback={null}>{draft && <DocumentEditor />}{detailId && <DocumentDetail />}</Suspense>
    <Modal title={isDemo ? 'Thông báo trong phiên mẫu' : 'Chứng từ cần kiểm tra'} open={notificationsOpen} onCancel={() => setNotificationsOpen(false)} footer={<Button onClick={() => setNotificationsOpen(false)}>Đóng</Button>}>
      <div className={styles.notificationIntro}><Icon name="clock" />Có {draftCount} bản nháp trong kỳ cần kiểm tra.</div>
      {drafts.map(item => <button className={styles.notification} key={item.id} onClick={() => { setNotificationsOpen(false); navigate('/documents?status=draft'); }}><span><strong>{item.reference}</strong><small>{item.description}</small></span><Icon name="chevron" size={16} /></button>)}
      <p className={styles.muted}>{isDemo ? 'Thông báo được tạo từ bộ dữ liệu mẫu, chưa kết nối dịch vụ bên ngoài.' : 'Hiển thị nháp trong 100 chứng từ gần nhất. Mở Kế toán tổng hợp để tra cứu toàn bộ.'}</p>
    </Modal>
  </div>;
}
