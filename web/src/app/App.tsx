import { lazy, Suspense, useCallback, useEffect, useMemo, useState } from 'react';
import { Avatar, Badge, Button, Drawer, Dropdown, Modal, Select, Spin, Tooltip } from 'antd';
import { Link, NavLink, Outlet, useBeforeUnload, useLocation, useNavigate, useNavigation } from 'react-router-dom';
import { useWorkspace } from './WorkspaceProvider';
import { voucherDraftLabel, voucherDraftModule } from '../features/vouchers/draft-state';
import { documentTypes } from '../shared/data/types';
import { Icon } from '../shared/ui/Icon';
import styles from './App.module.css';
import { api, apiScope, isDemo } from '../shared/api/client';
import { searchOptions } from '../shared/ui/search';
import { DocumentSearch } from './DocumentSearch';
import { accountingModule } from './module-navigation';
import { SidebarNavigation } from './SidebarNavigation';
import { sidebarPinsKey, sidebarShortcuts } from './sidebar-shortcuts';
import type { SidebarItem } from './sidebar-shortcuts';
import { useSidebarPins } from './useSidebarPins';

const DocumentEditor = lazy(() => import('../features/documents/DocumentDrawer').then(module => ({ default: module.DocumentEditor })));
const DocumentDetail = lazy(() => import('../features/documents/DocumentDrawer').then(module => ({ default: module.DocumentDetail })));

const navigation: SidebarItem[] = [
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
  ...(!isDemo ? [{ path: '/imports', label: 'Nhập từ Excel', icon: 'upload' as const }] : []),
];

export function AppShell() {
  const { period, setPeriod, data, draft, documentDrafts, resumeDocumentDraft, voucherDrafts, detailId, refresh, newDraft } = useWorkspace();
  const scope = apiScope();
  const shortcuts = useMemo(() => sidebarShortcuts(navigation, isDemo ? 'owner' : scope.permissions, isDemo), [scope.permissions]);
  const pins = useSidebarPins(sidebarPinsKey(isDemo ? 'demo' : scope.companyId, isDemo ? 'demo' : scope.actor), shortcuts);
  const [mobileOpen, setMobileOpen] = useState(false);
  const [collapsed, setCollapsed] = useState(false);
  const [notificationsOpen, setNotificationsOpen] = useState(false);
  const location = useLocation();
  useBeforeUnload(useCallback((event: BeforeUnloadEvent) => { if (Object.keys(voucherDrafts).length || Object.keys(documentDrafts).length) { event.preventDefault(); event.returnValue = ''; } }, [voucherDrafts, documentDrafts]));
  const navigate = useNavigate();
  const navigationState = useNavigation();
  const module = accountingModule(location.pathname);
  const moduleDrafts = [
    ...Object.entries(voucherDrafts).filter(([, value]) => voucherDraftModule(value) === module?.path).map(([key, value]) => ({ key: `voucher:${key}`, label: voucherDraftLabel(value), onClick: () => navigate(value.route, { state: { returnTo: value.returnTo } }) })),
    ...Object.entries(documentDrafts).filter(([, value]) => ({ receipt: '/cash', payment: '/cash', sale: '/sales', purchase: '/purchases', journal: '/documents' }[value.type]) === module?.path).map(([key, value]) => ({ key: `journal:${key}`, label: `${documentTypes[value.type]} · ${value.reference || value.description || 'Nháp mới'}`, onClick: () => resumeDocumentDraft(key) })),
  ];
  const current = navigation.find(item => item.path === (module?.path ?? location.pathname))?.label ?? module?.title ?? (location.pathname.startsWith('/vouchers/') ? 'Chứng từ' : location.pathname === '/settings' ? 'Thiết lập' : 'Trợ giúp');
  const companyName = isDemo ? 'Không gian mẫu' : data?.companyName ?? 'Doanh nghiệp';
  const person = isDemo ? 'Minh Anh' : apiScope().name;
  const initials = person.split(' ').slice(-2).map(value => value[0]).join('').toUpperCase();
  const draftCount = data?.server?.draftCount ?? data?.documents.filter(item => item.status === 'draft' && item.date.startsWith(period)).length ?? 0;
  const drafts = data?.documents.filter(item => item.status === 'draft' && item.date.startsWith(period)) ?? [];

  useEffect(() => { document.title = `${current} · Oanh`; }, [current]);

  const sidebar = (compact = false, mobile = false) => <div className={styles.sidebarContent}>
    <Link to="/" className={styles.brand} aria-label="Oanh, về tổng quan"><span className={styles.brandMark}><span /><span /><span /></span><span className={styles.brandName}>oanh<span className={styles.brandDot}>.</span></span></Link>
    <SidebarNavigation navigation={navigation} shortcuts={shortcuts} pinnedIds={pins.ids} togglePin={pins.toggle} onJournal={() => newDraft()} onNavigate={() => setMobileOpen(false)} compact={compact} mobile={mobile} draftCount={draftCount} />
    <div className={styles.sidebarBottom}>
      <div className={styles.helpCard}><span className={styles.helpIcon}><Icon name="spark" size={18} /></span><strong>Kế toán nhẹ nhàng hơn</strong><p>Bắt đầu từ một chứng từ.<br />Kết nối mọi con số.</p><Link to="/help" onClick={() => setMobileOpen(false)}>Khám phá hướng dẫn <Icon name="arrow" size={15} /></Link></div>
      <Tooltip title={compact ? 'Thiết lập' : undefined} placement="right" trigger={['hover', 'focus']}><NavLink to="/settings" className={({ isActive }) => `${styles.navLink} ${isActive ? styles.activeNav : ''}`} onClick={() => setMobileOpen(false)}><Icon name="settings" size={19} /><span className={styles.navLabel}>Thiết lập</span></NavLink></Tooltip>
    </div>
  </div>;

  return <div className={`${styles.app} ${collapsed ? styles.sidebarCollapsed : ''}`}>
    <a href="#main-content" className={styles.skipLink}>Chuyển đến nội dung</a>
    <aside id="desktop-sidebar" className={styles.sidebar}>{sidebar(collapsed)}</aside>
    <Drawer className={styles.mobileDrawer} placement="left" open={mobileOpen} onClose={() => setMobileOpen(false)} size={280} title="Điều hướng" styles={{ body: { padding: 16, background: '#202329' }, header: { background: '#202329', borderColor: '#3a4046' } }}>{sidebar(false, true)}</Drawer>
    <div className={styles.workspace}>
      <header className={styles.topbar}>
        <Tooltip title={collapsed ? 'Mở rộng thanh bên' : 'Thu gọn thanh bên'} trigger={['hover', 'focus']}><Button className={styles.sidebarToggle} type="text" icon={<Icon name="menu" />} aria-label={collapsed ? 'Mở rộng thanh bên' : 'Thu gọn thanh bên'} aria-expanded={!collapsed} aria-controls="desktop-sidebar" onClick={() => setCollapsed(value => !value)} /></Tooltip>
        <Button className={styles.mobileMenu} type="text" icon={<Icon name="menu" />} aria-label="Mở điều hướng" onClick={() => setMobileOpen(true)} />
        <button type="button" className={styles.companySwitch} aria-label="Thiết lập công ty" title={companyName} onClick={() => navigate('/settings')}><span className={styles.companyIcon}><Icon name="building" size={20} /></span><span><strong>{companyName}</strong><small>Kế toán doanh nghiệp</small></span><Icon name="down" size={14} /></button>
        <div className={styles.topbarActions}>
          <DocumentSearch shortcuts={shortcuts} pinnedIds={pins.ids} />
          <Tooltip title="Hướng dẫn sử dụng"><Button type="text" icon={<Icon name="help" />} aria-label="Hướng dẫn sử dụng" onClick={() => navigate('/help')} /></Tooltip>
          <Badge dot={draftCount > 0} offset={[-7, 6]}><Button type="text" icon={<Icon name="bell" />} aria-label="Xem thông báo" onClick={() => setNotificationsOpen(true)} /></Badge>
          <div className={styles.headerDivider} />
          <Dropdown menu={{ items: [{ key: 'settings', label: isDemo ? 'Thông tin không gian mẫu' : 'Cài đặt doanh nghiệp', onClick: () => navigate('/settings') }, ...(apiScope().permissions === 'owner' && !isDemo ? [{ key: 'audit', label: 'Nhật ký thao tác', icon: <Icon name="clock" size={16} />, onClick: () => navigate('/settings?tab=audit') }] : []), { key: 'help', label: 'Hướng dẫn sử dụng', onClick: () => navigate('/help') }, { key: 'refresh', label: 'Tải lại dữ liệu', onClick: refresh }, ...(!isDemo ? [{ key: 'logout', label: 'Đăng xuất', onClick: () => { void api('/auth/logout', { method: 'POST' }).then(() => window.location.reload()); } }] : [])] }} trigger={['click']}><button className={styles.profile}><Avatar size={34} style={{ background: '#e4eee8', color: '#3f6651', fontWeight: 700 }}>{initials}</Avatar><span>{person}<small>{isDemo ? 'Kế toán viên · Mẫu' : ({ owner: 'Chủ sở hữu', accountant: 'Kế toán', viewer: 'Chỉ xem' }[apiScope().permissions as 'owner'])}</small></span><Icon name="down" size={12} /></button></Dropdown>
        </div>
      </header>
      {navigationState.state !== 'idle' && <div className={styles.routeProgress} role="progressbar" aria-label="Đang chuyển trang" />}
      <main id="main-content" className={`${styles.main} ${!isDemo && module ? styles.denseWorkspace : ''}`} tabIndex={-1}>
        <div className={styles.contextBar}><div>{moduleDrafts.length > 0 && <Dropdown trigger={['click']} menu={{ items: moduleDrafts }}><Button type="text" icon={<Icon name="file" size={14} />}>Nháp đang nhập ({moduleDrafts.length}) <Icon name="down" size={12} /></Button></Dropdown>}<span className={styles.periodLabel}>Kỳ kế toán</span><Select showSearch={searchOptions} aria-label="Chọn kỳ kế toán" value={period} onChange={value => { if (value === period) return; setPeriod(value); const params = new URLSearchParams(location.search); if (params.has('page')) { params.delete('page'); navigate({ pathname: location.pathname, search: params.toString(), hash: location.hash }, { replace: true, state: location.state }); } }} className={styles.periodSelect} options={Array.from({ length: isDemo ? 12 : 84 }, (_, index) => { const year = Number(period.slice(0, 4)) + (isDemo ? 0 : Math.floor(index / 12) - 3); const month = String(index % 12 + 1).padStart(2, '0'); return { value: `${year}-${month}`, label: `Tháng ${month}, ${year}` }; })} suffixIcon={<Icon name="down" size={13} />} /></div></div>
        <Outlet />
      </main>
    </div>
    <Suspense fallback={<Spin fullscreen tip="Đang mở chứng từ…" />}>{draft && <DocumentEditor />}{detailId && <DocumentDetail />}</Suspense>
    <Modal title={isDemo ? 'Thông báo trong phiên mẫu' : 'Chứng từ cần kiểm tra'} open={notificationsOpen} onCancel={() => setNotificationsOpen(false)} footer={<Button onClick={() => setNotificationsOpen(false)}>Đóng</Button>}>
      <div className={styles.notificationIntro}><Icon name="clock" />Có {draftCount} bản nháp trong kỳ cần kiểm tra.</div>
      {drafts.map(item => <button className={styles.notification} key={item.id} onClick={() => { setNotificationsOpen(false); navigate('/documents?status=draft'); }}><span><strong>{item.reference}</strong><small>{item.description}</small></span><Icon name="chevron" size={16} /></button>)}
      <p className={styles.muted}>{isDemo ? 'Thông báo được tạo từ bộ dữ liệu mẫu, chưa kết nối dịch vụ bên ngoài.' : 'Hiển thị nháp trong 100 chứng từ gần nhất. Mở Kế toán tổng hợp để tra cứu toàn bộ.'}</p>
    </Modal>
  </div>;
}
