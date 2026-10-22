import { useState } from 'react';
import { useLocation } from 'react-router-dom';
import { accountingModule } from '../../app/module-navigation';
import { Alert, App, Button, Form, Input, Modal, Select, Switch, Table, Tabs, Tag } from 'antd';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useWorkspace } from '../../app/WorkspaceProvider';
import { normalizeSearch } from '../../shared/accounting/money';
import { downloadCsv } from '../../shared/accounting/csv';
import { saveDemoPartners, workspaceKey } from '../../shared/data/workspace';
import type { Account, Partner } from '../../shared/data/types';
import { DataBoundary, PageHeading } from '../../shared/ui/Page';
import { Icon } from '../../shared/ui/Icon';
import styles from '../../app/App.module.css';
import { api, apiScope, command, isDemo } from '../../shared/api/client';

const roles = { customer: 'Khách hàng', supplier: 'Nhà cung cấp', both: 'Khách hàng & NCC' };

export function Component() {
  const module = accountingModule(useLocation().pathname);
  const moduleRole = module && 'tradeKind' in module ? module.tradeKind === 'sale' ? 'customer' : 'supplier' : undefined;
  const { data } = useWorkspace();
  const { message } = App.useApp();
  const client = useQueryClient();
  const [search, setSearch] = useState('');
  const [selectedTab, setTab] = useState('partners');
  const tab = moduleRole ? 'partners' : selectedTab;
  const [selectedRole, setRole] = useState('all');
  const role = moduleRole ?? selectedRole;
  const [editing, setEditing] = useState<Partner | null>(null);
  const [open, setOpen] = useState(false);
  const [form] = Form.useForm<Partner>();
  const [account, setAccount] = useState<Account | null>(null);
  const [accountForm] = Form.useForm<Account>();
  const canEdit = isDemo || apiScope().permissions !== 'viewer';
  const accountMutation = useMutation({ mutationFn: (payload: Account) => command('/accounts', payload), retry: false, onSuccess: async () => { setAccount(null); void message.success('Đã lưu tài khoản.'); await client.invalidateQueries({ queryKey: workspaceKey() }); }, onError: error => void message.error(error.message) });
  function editAccount(value?: Account) { const next = value ?? { id: crypto.randomUUID(), code: '', name: '', group: 'expense' as const, requiresPartner: false, active: true }; setAccount(next); accountForm.resetFields(); accountForm.setFieldsValue(next); }
  const mutation = useMutation({ mutationFn: saveDemoPartners, retry: false, onSuccess: async () => { setOpen(false); void message.success(isDemo ? 'Đã cập nhật danh mục trong phiên mẫu.' : 'Đã lưu danh mục vào cơ sở dữ liệu.'); await client.invalidateQueries({ queryKey: workspaceKey() }); }, onError: error => { void message.error(error.message); } });
  if (!data) return <DataBoundary><></></DataBoundary>;
  const partners = data.partners.filter(item => normalizeSearch(`${item.code} ${item.name} ${item.email} ${item.phone}`).includes(normalizeSearch(search)) && (role === 'all' || item.role === role || item.role === 'both'));
  const accounts = data.accounts.filter(item => normalizeSearch(`${item.code} ${item.name}`).includes(normalizeSearch(search)));
  const referenced = editing ? data.documents.some(item => item.partnerId === editing.id || item.lines.some(line => line.partnerId === editing.id)) : false;

  function edit(partner: Partner | null) {
    setEditing(partner);
    form.resetFields();
    form.setFieldsValue(partner ?? { code: '', name: '', role: moduleRole ?? 'customer', email: '', phone: '', active: true });
    setOpen(true);
  }

  function exportData() {
    downloadCsv(`danh-muc-${isDemo ? 'mau-' : ''}${tab}.csv`, tab === 'partners' ? [[isDemo ? 'DỮ LIỆU MẪU · Phiên bản' : `${data!.companyName} · Phiên bản`, String(data!.revision)], ['Mã', 'Tên', 'Vai trò', 'Email', 'Điện thoại', 'Trạng thái'], ...partners.map(item => [item.code, item.name, roles[item.role], item.email, `'${item.phone}`, item.active ? 'Đang sử dụng' : 'Ngừng sử dụng'])] : [[isDemo ? 'DỮ LIỆU MẪU' : data!.companyName ?? 'Danh mục doanh nghiệp'], ['Mã tài khoản', 'Tên tài khoản', 'Yêu cầu đối tượng'], ...accounts.map(item => [item.code, item.name, item.requiresPartner ? 'Có' : 'Không'])]);
  }

  return <DataBoundary>
    <PageHeading title={moduleRole === 'supplier' ? 'Nhà cung cấp' : moduleRole === 'customer' ? 'Khách hàng' : 'Danh mục'} description="Sắp xếp dữ liệu nền tảng để mỗi lần nhập liệu đều thuận tiện hơn." actions={<><Button icon={<Icon name="download" size={16} />} onClick={exportData}>Xuất CSV</Button>{tab === 'accounts' && !isDemo && apiScope().permissions === 'owner' && <Button type="primary" onClick={() => editAccount()}>Thêm tài khoản</Button>}{tab === 'partners' && canEdit && <Button type="primary" icon={<Icon name="plus" size={17} />} onClick={() => edit(null)}>Thêm đối tượng</Button>}</>} />
    {!moduleRole && <div className={styles.catalogSummary}>{[{ label: 'Tổng đối tượng', count: data.partners.length, icon: 'users' }, { label: 'Khách hàng', count: data.partners.filter(item => item.role !== 'supplier').length, icon: 'cart' }, { label: 'Nhà cung cấp', count: data.partners.filter(item => item.role !== 'customer').length, icon: 'bag' }, { label: isDemo ? 'Tài khoản mẫu' : 'Tài khoản', count: data.accounts.length, icon: 'book' }].map(item => <div key={item.label}><span>{item.label}</span><strong>{item.count}</strong></div>)}</div>}
    <section className={styles.panel}>{!moduleRole && <div className={styles.listTabs}><Tabs activeKey={tab} onChange={value => { setTab(value); setSearch(''); }} items={[{ key: 'partners', label: 'Khách hàng & nhà cung cấp' }, { key: 'accounts', label: isDemo ? 'Hệ thống tài khoản mẫu' : 'Hệ thống tài khoản' }, ...(!isDemo ? [{ key: 'reference', label: 'Tài khoản từ Google Sheet' }] : [])]} /></div>}
      <div className={styles.tableToolbar}><Input prefix={<Icon name="search" size={17} />} aria-label="Tìm trong danh mục" placeholder={tab === 'partners' ? 'Tìm mã, tên hoặc thông tin liên hệ…' : 'Tìm mã hoặc tên tài khoản…'} value={search} allowClear onChange={event => setSearch(event.target.value)} className={styles.filterSearch} />{tab === 'partners' && !moduleRole && <Select aria-label="Lọc vai trò đối tượng" value={role} onChange={setRole} style={{ minWidth: 180 }} options={[{ value: 'all', label: 'Tất cả vai trò' }, { value: 'customer', label: 'Khách hàng' }, { value: 'supplier', label: 'Nhà cung cấp' }]} />}</div>
      {tab === 'reference' ? <ReferenceChart /> : tab === 'partners' ? <Table<Partner> rowKey="id" dataSource={partners} pagination={{ pageSize: 8, hideOnSinglePage: true }} scroll={{ x: 850 }} columns={[
        { title: 'MÃ ĐỐI TƯỢNG', dataIndex: 'code', width: 130, render: (code: string, item) => canEdit ? <button className={styles.tableLink} onClick={() => edit(item)}>{code}</button> : <strong>{code}</strong> },
        { title: 'TÊN ĐỐI TƯỢNG', dataIndex: 'name', render: (name: string, item) => <div className={styles.partnerCell}><span className={styles.partnerAvatar}>{name.replace(/^Công ty (TNHH|CP) /, '').slice(0, 1)}</span><span><strong>{name}</strong><small className={styles.cellSub}>{item.email || 'Chưa có email'}</small></span></div> },
        { title: 'VAI TRÒ', dataIndex: 'role', width: 150, render: (value: Partner['role']) => <Tag color={value === 'supplier' ? 'blue' : 'cyan'} variant="filled">{roles[value]}</Tag> },
        { title: 'ĐIỆN THOẠI', dataIndex: 'phone', width: 125, render: (value: string) => value || '—' },
        { title: 'TRẠNG THÁI', dataIndex: 'active', width: 125, render: (value: boolean) => <span className={value ? styles.positive : styles.muted}>{value ? 'Đang sử dụng' : 'Ngừng sử dụng'}</span> },
        { width: 45, render: (_, item) => canEdit && <Button type="text" icon={<Icon name="chevron" size={16} />} onClick={() => edit(item)} aria-label={`Sửa ${item.code}`} /> },
      ]} /> : <><div className={styles.inlineNotice}><Icon name="help" size={16} />{isDemo ? 'Danh sách tài khoản phục vụ dữ liệu mẫu, chưa phải hệ thống tài khoản được duyệt cho doanh nghiệp.' : 'Danh mục tài khoản khởi tạo đã xác nhận. Tài khoản có lịch sử được giữ nguyên mã và phân loại.'}</div><Table rowKey="id" dataSource={accounts} pagination={false} columns={[{ title: 'MÃ TÀI KHOẢN', dataIndex: 'code', width: 180, render: (value: string) => <strong className={styles.positive}>{value}</strong> }, { title: 'TÊN TÀI KHOẢN', dataIndex: 'name' }, ...(!isDemo && apiScope().permissions === 'owner' ? [{ title: 'SỬA', render: (_: unknown, row: Account) => <Button onClick={() => editAccount(row)}>Sửa</Button> }] : []), { title: 'ĐỐI TƯỢNG', dataIndex: 'requiresPartner', render: (value: boolean) => value ? 'Bắt buộc theo dòng' : 'Không bắt buộc' }]} /></>}
    </section>
    <Modal title="Tài khoản kế toán" open={Boolean(account)} onCancel={() => setAccount(null)} footer={null}><Form<Account> form={accountForm} layout="vertical" onFinish={values => accountMutation.mutate({ ...account!, ...values })}><Form.Item name="code" label="Mã tài khoản" rules={[{ required: true }]}><Input disabled={data.accounts.some(item => item.id === account?.id)} /></Form.Item><Form.Item name="name" label="Tên tài khoản" rules={[{ required: true }]}><Input maxLength={200} /></Form.Item><Form.Item name="group" label="Nhóm nghiệp vụ" rules={[{ required: true }]}><Select options={Object.entries({ wip: 'Chi phí dở dang', cash: 'Tiền', receivable: 'Phải thu', payable: 'Phải trả', revenue: 'Doanh thu', expense: 'Chi phí', inventory: 'Kho', capital: 'Nguồn vốn / kết quả', asset: 'Tài sản', tax: 'Thuế', liability: 'Nợ phải trả khác' }).map(([value, label]) => ({ value, label }))} /></Form.Item><Form.Item name="requiresPartner" label="Bắt buộc đối tượng theo dòng" valuePropName="checked"><Switch /></Form.Item><Form.Item name="active" label="Đang sử dụng" valuePropName="checked"><Switch /></Form.Item><p className={styles.muted}>Máy chủ giữ nguyên mã và thông tin tài khoản đã có lịch sử. Các nghiệp vụ tự động sử dụng tài khoản khởi tạo.</p><Button type="primary" htmlType="submit" loading={accountMutation.isPending}>Lưu tài khoản</Button></Form></Modal>
    <Modal title={editing ? 'Thông tin đối tượng' : isDemo ? 'Thêm đối tượng mẫu' : 'Thêm đối tượng'} open={open} onCancel={() => setOpen(false)} footer={null}>
      <p className={styles.muted}>{isDemo ? 'Thay đổi chỉ tồn tại trong phiên mẫu.' : 'Thay đổi được lưu vào cơ sở dữ liệu của doanh nghiệp.'} {referenced && 'Mã và tên đối tượng đã có chứng từ được giữ nguyên.'}</p>
      <Form form={form} layout="vertical" onFinish={values => mutation.mutate([{ ...values, id: editing?.id ?? crypto.randomUUID() }])}>
        <div className={styles.formGrid}><Form.Item name="code" label="Mã đối tượng" rules={[{ required: true, whitespace: true, message: 'Nhập mã đối tượng' }, { pattern: /^[A-Za-z0-9_-]+$/, message: 'Dùng chữ không dấu, chữ số, dấu gạch ngang hoặc gạch dưới' }]}><Input maxLength={40} disabled={Boolean(editing)} placeholder="VD: KH005" /></Form.Item><Form.Item name="role" label="Vai trò" rules={[{ required: true }]}><Select options={Object.entries(roles).map(([value, label]) => ({ value, label }))} disabled={referenced} /></Form.Item></div>
        <Form.Item name="name" label="Tên đối tượng" rules={[{ required: true, whitespace: true, message: 'Nhập tên đối tượng' }]}><Input maxLength={200} disabled={referenced} /></Form.Item>
        <div className={styles.formGrid}><Form.Item name="email" label="Email" rules={[{ type: 'email', message: 'Email không đúng định dạng' }]}><Input type="email" autoComplete="email" spellCheck={false} maxLength={254} /></Form.Item><Form.Item name="phone" label="Điện thoại" rules={[{ pattern: /^[+\d ()-]*$/, message: 'Số điện thoại không hợp lệ' }]}><Input type="tel" autoComplete="tel" maxLength={30} /></Form.Item></div>
        <Form.Item name="active" label="Đang sử dụng" valuePropName="checked"><Switch /></Form.Item><div className={styles.modalActions}><Button onClick={() => setOpen(false)}>Đóng</Button><Button type="primary" htmlType="submit" loading={mutation.isPending}>{isDemo ? 'Lưu trong phiên mẫu' : 'Lưu đối tượng'}</Button></div>
      </Form>
    </Modal>
  </DataBoundary>;
}

interface ChartAccount { code: string; name: string; parentCode: string; level: number; group: Account['group']; requiresPartner: boolean; sourceRow: number }
interface ChartTemplate { version: string; effectiveFrom: string; source: string; sha256: string; accounts: ChartAccount[] }
function ReferenceChart() {
 const { data } = useWorkspace();
 const { message, modal } = App.useApp();
 const client = useQueryClient();
 const [search, setSearch] = useState('');
 const query = useQuery({ queryKey: [...workspaceKey(), 'account-reference'], queryFn: ({ signal }) => api<ChartTemplate>('/accounts/reference', { signal }), staleTime: Infinity, retry: false });
 const apply = useMutation({ retry: false, mutationFn: () => command<{ added: number }>('/accounts/reference', { version: query.data!.version }), onSuccess: async result => { void message.success(`Đã bổ sung ${result.added} mã tài khoản; tài khoản hiện có được giữ nguyên.`); await client.invalidateQueries({ queryKey: workspaceKey() }); }, onError: error => void message.error(error.message) });
 if (query.error) return <Alert type="error" title={query.error.message} />;
 const chart = query.data;
 const missing = chart?.accounts.filter(a => !data?.accounts.some(current => current.code === a.code)) ?? [];
 return <div style={{ padding: 16 }}>
  <Alert type="info" showIcon title={`Hệ thống tài khoản do người dùng cung cấp · ${chart?.accounts.length ?? '…'} mã`} description={<>Phiên bản {chart?.version ?? '…'}. Tên được giữ theo nguồn; quan hệ cha/con đã chuẩn hóa theo mã tại các dòng lệch cột. <a href={chart?.source} target="_blank" rel="noreferrer">Mở Google Sheet nguồn</a>. Nạp danh mục chỉ thêm mã còn thiếu, giữ nguyên tài khoản và số liệu đã có.</>} />
  <div className={styles.tableToolbar}><Input aria-label="Tìm tài khoản tham chiếu" placeholder="Tìm mã hoặc tên tài khoản…" allowClear value={search} onChange={event => setSearch(event.target.value)} />{apiScope().permissions === 'owner' && <Button type="primary" disabled={!missing.length} loading={apply.isPending} onClick={() => modal.confirm({ title: `Bổ sung ${missing.length} mã tài khoản còn thiếu?`, content: 'Mã, tên và phân loại của tài khoản hiện có được giữ nguyên. Lịch sử chứng từ và chính sách hạch toán tự động không thay đổi.', okText: 'Bổ sung tài khoản', cancelText: 'Đóng', onOk: () => apply.mutateAsync().then(() => undefined) })}>Nạp {missing.length} mã còn thiếu</Button>}</div>
  <Table<ChartAccount> rowKey="code" loading={query.isPending} dataSource={chart?.accounts.filter(a => normalizeSearch(`${a.code} ${a.name}`).includes(normalizeSearch(search)))} pagination={{ pageSize: 25, showSizeChanger: true }} scroll={{ x: 800 }} columns={[
   { title: 'Mã tài khoản', dataIndex: 'code', width: 180, render: (value: string, row) => <strong style={{ paddingLeft: (row.level - 1) * 16 }}>{value}</strong> },
   { title: 'Tên theo nguồn', dataIndex: 'name' }, { title: 'Tài khoản cha', dataIndex: 'parentCode', width: 120, render: value => value || '—' },
   { title: 'Dòng nguồn', dataIndex: 'sourceRow', width: 100 },
   { title: 'Trong doanh nghiệp', width: 180, render: (_, row) => { const current = data?.accounts.find(a => a.code === row.code); return current ? current.name === row.name ? 'Đã có' : <span>Đã có · giữ tên hiện tại</span> : <Tag color="blue">Chưa nạp</Tag>; } },
  ]} />
 </div>;
}
