import { useState } from 'react';
import { useLocation } from 'react-router-dom';
import { accountTree, accountBranchKeys, accountDescendants } from './account-tree';
import type { AccountNode } from './account-tree';
import { accountingModule } from '../../app/module-navigation';
import { App, Button, Form, Input, Modal, Select, Switch, Table, Tabs, Tag, TreeSelect } from 'antd';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useWorkspace } from '../../app/WorkspaceProvider';
import { normalizeSearch } from '../../shared/accounting/money';
import { downloadCsv } from '../../shared/accounting/csv';
import { saveDemoPartners, workspaceKey } from '../../shared/data/workspace';
import type { Account, Partner } from '../../shared/data/types';
import { DataBoundary, PageHeading, PageToolbar } from '../../shared/ui/Page';
import { Icon } from '../../shared/ui/Icon';
import { ImportExcelButton } from '../imports/ExcelImport';
import { ExportMenu } from '../../shared/ui/ExportMenu';
import type { ExportFormat } from '../../shared/ui/ExportMenu';
import { ToolbarButton } from '../../shared/ui/ToolbarButton';
import styles from '../../app/App.module.css';
import { apiScope, command, downloadApi, isDemo } from '../../shared/api/client';

interface AccountParentNode { value: string; title: string; disabled: boolean; children?: AccountParentNode[] }

const roles = { customer: 'Khách hàng', supplier: 'Nhà cung cấp', both: 'Khách hàng & NCC' };

export function Component() {
  const module = accountingModule(useLocation().pathname);
  const moduleRole = module && 'tradeKind' in module ? module.tradeKind === 'sale' ? 'customer' : 'supplier' : undefined;
  const { data, period, refresh } = useWorkspace();
  const { message } = App.useApp();
  const client = useQueryClient();
  const [search, setSearch] = useState('');
  const [expandedAccounts, setExpandedAccounts] = useState<string[] | null>(null);
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
  const accountMutation = useMutation({ mutationFn: ({ id, code, name, group, requiresPartner, active, parentId, parentRevision }: Account) => command<Account>('/accounts', { id, code, name, group, requiresPartner, active, parentId, parentRevision }), retry: false, onSuccess: async result => { setAccount(null); if (result.parentId) setExpandedAccounts(current => current === null ? null : [...new Set([...current, result.parentId!])]); void message.success('Đã lưu tài khoản.'); await client.invalidateQueries({ queryKey: workspaceKey() }); }, onError: error => void message.error(error.message) });
  function editAccount(value?: Account, parent?: Account) { const next = value ?? { id: crypto.randomUUID(), code: '', name: '', group: parent?.group ?? 'expense' as const, requiresPartner: parent?.requiresPartner ?? false, active: true, parentId: parent?.id ?? '', parentRevision: 0 }; setAccount(next); accountForm.resetFields(); accountForm.setFieldsValue(next); }
  const mutation = useMutation({ mutationFn: saveDemoPartners, retry: false, onSuccess: async () => { setOpen(false); void message.success(isDemo ? 'Đã cập nhật danh mục trong phiên mẫu.' : 'Đã lưu danh mục vào cơ sở dữ liệu.'); await client.invalidateQueries({ queryKey: workspaceKey() }); }, onError: error => { void message.error(error.message); } });
  if (!data) return <DataBoundary><></></DataBoundary>;
  const partners = data.partners.filter(item => normalizeSearch(`${item.code} ${item.name} ${item.email} ${item.phone}`).includes(normalizeSearch(search)) && (role === 'all' || item.role === role || item.role === 'both'));
  const accounts = data.accounts.filter(item => normalizeSearch(`${item.code} ${item.name}`).includes(normalizeSearch(search)));
  const tree = accountTree(data.accounts, search);
  const branches = accountBranchKeys(tree);
  const blockedParents = account ? accountDescendants(data.accounts, account.id) : new Set<string>();
  const parentNodes = (items: AccountNode[]): AccountParentNode[] => items.map(item => ({ value: item.id, title: `${item.code} · ${item.name}`, disabled: item.active === false || blockedParents.has(item.id), children: item.children ? parentNodes(item.children) : undefined }));
  const referenced = editing ? isDemo ? data.documents.some(item => item.partnerId === editing.id || item.lines.some(line => line.partnerId === editing.id)) : Boolean(editing.hasReferences) : false;
  const accountReferenced = Boolean(account?.hasReferences);
  const parentReferenced = Boolean(account && data.accounts.some(item => blockedParents.has(item.id) && item.hasReferences));

  function edit(partner: Partner | null) {
    setEditing(partner);
    form.resetFields();
    form.setFieldsValue(partner ?? { code: '', name: '', role: moduleRole ?? 'customer', email: '', phone: '', active: true });
    setOpen(true);
  }

  function exportData(format: ExportFormat) {
    if (!isDemo) {
      const path = `/catalog/export?${new URLSearchParams({ period, catalog: tab, role, q: search, revision: String(data!.revision), format })}`;
      if (format === 'print') { window.open(`/api/v1${path}`, '_blank', 'noopener,noreferrer'); return; }
      return downloadApi(path, `danh-muc-${tab}.${format}`);
    }
    downloadCsv(`danh-muc-${isDemo ? 'mau-' : ''}${tab}.csv`, tab === 'partners' ? [[isDemo ? 'DỮ LIỆU MẪU · Phiên bản' : `${data!.companyName} · Phiên bản`, String(data!.revision)], ['Mã', 'Tên', 'Vai trò', 'Email', 'Điện thoại', 'Trạng thái'], ...partners.map(item => [item.code, item.name, roles[item.role], item.email, `'${item.phone}`, item.active ? 'Đang sử dụng' : 'Ngừng sử dụng'])] : [[isDemo ? 'DỮ LIỆU MẪU' : data!.companyName ?? 'Danh mục doanh nghiệp'], ['Mã tài khoản', 'Tên tài khoản', 'Tài khoản cha', 'Yêu cầu đối tượng'], ...accounts.map(item => [item.code, item.name, data!.accounts.find(parent => parent.id === item.parentId)?.code ?? '', item.requiresPartner ? 'Có' : 'Không'])]);
  }

  const pageActions = <>{(tab === 'partners' || tab === 'accounts' && apiScope().permissions === 'owner') && <ImportExcelButton kind={tab === 'partners' ? 'partners' : 'accounts'} />}{tab === 'accounts' && <><ToolbarButton label="Mở rộng tất cả tài khoản" icon="arrowDown" disabled={Boolean(search)} onClick={() => setExpandedAccounts(branches)} /><ToolbarButton label="Thu gọn tất cả tài khoản" icon="arrowUp" disabled={Boolean(search)} onClick={() => setExpandedAccounts([])} /></>}<ExportMenu onReload={refresh} onExport={exportData} formats={isDemo ? ['csv'] : undefined} />{tab === 'accounts' && !isDemo && apiScope().permissions === 'owner' && <ToolbarButton type="primary" onClick={() => editAccount()} label="Thêm tài khoản" icon="plus" />}{tab === 'partners' && canEdit && <ToolbarButton type="primary" onClick={() => edit(null)} label="Thêm đối tượng" icon="plus" />}</>;

  return <DataBoundary>
    <PageHeading title={moduleRole === 'supplier' ? 'Nhà cung cấp' : moduleRole === 'customer' ? 'Khách hàng' : 'Danh mục'} description="Sắp xếp dữ liệu nền tảng để mỗi lần nhập liệu đều thuận tiện hơn." />
    {!moduleRole && <div className={styles.catalogSummary}>{[{ label: 'Tổng đối tượng', count: data.partners.length, icon: 'users' }, { label: 'Khách hàng', count: data.partners.filter(item => item.role !== 'supplier').length, icon: 'cart' }, { label: 'Nhà cung cấp', count: data.partners.filter(item => item.role !== 'customer').length, icon: 'bag' }, { label: isDemo ? 'Tài khoản mẫu' : 'Tài khoản', count: data.accounts.length, icon: 'book' }].map(item => <div key={item.label}><span>{item.label}</span><strong>{item.count}</strong></div>)}</div>}
    <section className={styles.panel}>{!moduleRole && <div className={styles.listTabs}><Tabs activeKey={tab} onChange={value => { setTab(value); setSearch(''); }} items={[{ key: 'partners', label: 'Khách hàng & nhà cung cấp' }, { key: 'accounts', label: isDemo ? 'Hệ thống tài khoản mẫu' : 'Hệ thống tài khoản' }]} /></div>}
      <PageToolbar label="Công cụ danh mục" actions={pageActions} activeFilters={Number(Boolean(search.trim())) + Number(tab === 'partners' && !moduleRole && role !== 'all')} onResetFilters={() => { setSearch(''); if (!moduleRole) setRole('all'); }}><Input prefix={<Icon name="search" size={17} />} aria-label="Tìm trong danh mục" placeholder={tab === 'partners' ? 'Tìm mã, tên hoặc thông tin liên hệ…' : 'Tìm mã hoặc tên tài khoản…'} value={search} allowClear onChange={event => setSearch(event.target.value)} className={styles.filterSearch} />{tab === 'partners' && !moduleRole && <Select aria-label="Lọc vai trò đối tượng" value={role} onChange={setRole} style={{ minWidth: 180 }} options={[{ value: 'all', label: 'Tất cả vai trò' }, { value: 'customer', label: 'Khách hàng' }, { value: 'supplier', label: 'Nhà cung cấp' }]} />}</PageToolbar>
      {tab === 'partners' ? <Table<Partner> rowKey="id" dataSource={partners} pagination={{ pageSize: 8, hideOnSinglePage: true }} scroll={{ x: 850 }} columns={[
        { title: 'MÃ ĐỐI TƯỢNG', dataIndex: 'code', width: 130, render: (code: string, item) => canEdit ? <button className={styles.tableLink} onClick={() => edit(item)}>{code}</button> : <strong>{code}</strong> },
        { title: 'TÊN ĐỐI TƯỢNG', dataIndex: 'name', render: (name: string, item) => <div className={styles.partnerCell}><span className={styles.partnerAvatar}>{name.replace(/^Công ty (TNHH|CP) /, '').slice(0, 1)}</span><span><strong>{name}</strong><small className={styles.cellSub}>{item.email || 'Chưa có email'}</small></span></div> },
        { title: 'VAI TRÒ', dataIndex: 'role', width: 150, render: (value: Partner['role']) => <Tag color={value === 'supplier' ? 'blue' : 'cyan'} variant="filled">{roles[value]}</Tag> },
        { title: 'ĐIỆN THOẠI', dataIndex: 'phone', width: 125, render: (value: string) => value || '—' },
        { title: 'TRẠNG THÁI', dataIndex: 'active', width: 125, render: (value: boolean) => <span className={value ? styles.positive : styles.muted}>{value ? 'Đang sử dụng' : 'Ngừng sử dụng'}</span> },
        { width: 45, render: (_, item) => canEdit && <Button type="text" icon={<Icon name="chevron" size={16} />} onClick={() => edit(item)} aria-label={`Sửa ${item.code}`} /> },
      ]} /> : <><Table<AccountNode> rowKey="id" dataSource={tree} pagination={false} scroll={{ x: 1050 }} expandable={{ expandedRowKeys: search ? branches : expandedAccounts ?? branches, onExpandedRowsChange: keys => setExpandedAccounts(keys.map(String)), indentSize: 22, columnTitle: 'Mã tài khoản' }} columns={[
        { title: 'MÃ TÀI KHOẢN', dataIndex: 'code', width: 230, render: (value: string, row) => <strong className={styles.positive} title={row.path}>{value}</strong> },
        { title: 'TÊN TÀI KHOẢN', dataIndex: 'name', render: (value: string, row) => <span title={row.path}>{row.childCount ? <strong>{value}</strong> : value}{row.childCount > 0 && <Tag style={{ marginLeft: 8 }}>{row.childCount} tài khoản con</Tag>}</span> },
        { title: 'CẤP', dataIndex: 'level', width: 70, render: value => `Cấp ${value}` },
        { title: 'TRẠNG THÁI', dataIndex: 'active', width: 130, render: value => value === false ? <span className={styles.muted}>Ngừng sử dụng</span> : 'Đang sử dụng' },
        { title: 'ĐỐI TƯỢNG', dataIndex: 'requiresPartner', width: 170, render: value => value ? 'Bắt buộc theo dòng' : 'Không bắt buộc' },
        ...(!isDemo && apiScope().permissions === 'owner' ? [{ title: 'CHỨC NĂNG', width: 110, render: (_: unknown, row: AccountNode) => <><Button type="text" size="small" onClick={() => editAccount(row)} aria-label={`Sửa tài khoản ${row.code}`}>Sửa</Button><ToolbarButton label={`Thêm tài khoản con của ${row.code}`} icon="plus" onClick={() => editAccount(undefined, row)} /></> }] : []),
      ]} /></>}
    </section>
    <Modal title="Tài khoản kế toán" open={Boolean(account)} onCancel={() => setAccount(null)} footer={null}><Form<Account> form={accountForm} layout="vertical" onFinish={values => accountMutation.mutate({ ...account!, ...values })}><div className={styles.formGrid}><Form.Item name="code" label="Mã tài khoản" rules={[{ required: true }]}><Input disabled={accountReferenced || accountMutation.isPending} /></Form.Item><Form.Item name="group" label="Nhóm nghiệp vụ" rules={[{ required: true }]}><Select disabled={accountReferenced || accountMutation.isPending} options={Object.entries({ wip: 'Chi phí dở dang', cash: 'Tiền', receivable: 'Phải thu', payable: 'Phải trả', revenue: 'Doanh thu', expense: 'Chi phí', inventory: 'Kho', capital: 'Nguồn vốn / kết quả', asset: 'Tài sản', tax: 'Thuế', liability: 'Nợ phải trả khác' }).map(([value, label]) => ({ value, label }))} /></Form.Item></div><Form.Item name="parentId" label="Tài khoản cha" getValueProps={value => ({ value: value || undefined })} normalize={value => value ?? ''}><TreeSelect disabled={parentReferenced || accountMutation.isPending} aria-label="Tài khoản cha" allowClear showSearch treeLine treeDefaultExpandAll placeholder="Không có · tài khoản cấp 1" treeData={parentNodes(accountTree(data.accounts))} filterTreeNode={(input, node) => normalizeSearch(String(node.title)).includes(normalizeSearch(input))} onChange={value => { if (!data.accounts.some(item => item.id === account?.id)) { const parent = data.accounts.find(item => item.id === value); if (parent) accountForm.setFieldsValue({ group: parent.group, requiresPartner: parent.requiresPartner }); } }} /></Form.Item><Form.Item name="name" label="Tên tài khoản" rules={[{ required: true }]}><Input disabled={accountReferenced || accountMutation.isPending} maxLength={200} /></Form.Item><div className={styles.formGrid}><Form.Item name="requiresPartner" label="Bắt buộc đối tượng theo dòng" valuePropName="checked"><Switch disabled={accountReferenced || accountMutation.isPending} /></Form.Item><Form.Item name="active" label="Đang sử dụng" valuePropName="checked"><Switch /></Form.Item></div><p className={styles.muted}>Tài khoản chưa có chứng từ được sửa thông tin. Khi đã có chứng từ, chỉ đổi trạng thái sử dụng để giữ lịch sử; nhánh có chứng từ giữ nguyên tài khoản cha. Thay đổi được ghi nhật ký.</p><Button type="primary" htmlType="submit" loading={accountMutation.isPending}>Lưu tài khoản</Button></Form></Modal>
    <Modal title={editing ? 'Thông tin đối tượng' : isDemo ? 'Thêm đối tượng mẫu' : 'Thêm đối tượng'} open={open} onCancel={() => setOpen(false)} footer={null}>
      <p className={styles.muted}>{isDemo ? 'Thay đổi chỉ tồn tại trong phiên mẫu.' : 'Thay đổi được lưu vào cơ sở dữ liệu của doanh nghiệp.'} {referenced ? 'Đã có chứng từ: giữ mã, tên và vai trò; thông tin liên hệ và trạng thái vẫn sửa được.' : 'Chưa có chứng từ: bạn có thể sửa mã, tên, vai trò và thông tin liên hệ.'}</p>
      <Form form={form} layout="vertical" onFinish={values => mutation.mutate([{ ...values, id: editing?.id ?? crypto.randomUUID() }])}>
        <div className={styles.formGrid}><Form.Item name="code" label="Mã đối tượng" rules={[{ required: true, whitespace: true, message: 'Nhập mã đối tượng' }, { pattern: /^[A-Za-z0-9_-]+$/, message: 'Dùng chữ không dấu, chữ số, dấu gạch ngang hoặc gạch dưới' }]}><Input maxLength={40} disabled={referenced || mutation.isPending} placeholder="VD: KH005" /></Form.Item><Form.Item name="role" label="Vai trò" rules={[{ required: true }]}><Select options={Object.entries(roles).map(([value, label]) => ({ value, label }))} disabled={referenced} /></Form.Item></div>
        <Form.Item name="name" label="Tên đối tượng" rules={[{ required: true, whitespace: true, message: 'Nhập tên đối tượng' }]}><Input maxLength={200} disabled={referenced} /></Form.Item>
        <div className={styles.formGrid}><Form.Item name="email" label="Email" rules={[{ type: 'email', message: 'Email không đúng định dạng' }]}><Input type="email" autoComplete="email" spellCheck={false} maxLength={254} /></Form.Item><Form.Item name="phone" label="Điện thoại" rules={[{ pattern: /^[+\d ()-]*$/, message: 'Số điện thoại không hợp lệ' }]}><Input type="tel" autoComplete="tel" maxLength={30} /></Form.Item></div>
        <Form.Item name="active" label="Đang sử dụng" valuePropName="checked"><Switch /></Form.Item><div className={styles.modalActions}><Button onClick={() => setOpen(false)}>Đóng</Button><Button type="primary" htmlType="submit" loading={mutation.isPending}>{isDemo ? 'Lưu trong phiên mẫu' : 'Lưu đối tượng'}</Button></div>
      </Form>
    </Modal>
  </DataBoundary>;
}
