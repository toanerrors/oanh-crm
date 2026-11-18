import { useState } from 'react';
import { Link, useLocation, useSearchParams } from 'react-router-dom';
import { accountTree, accountBranchKeys, accountDescendants } from './account-tree';
import type { AccountNode } from './account-tree';
import { accountingModule } from '../../app/module-navigation';
import { App, Button, Form, Input, Modal, Select, Switch, Table, Tabs, Tag, TreeSelect } from 'antd';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useWorkspace } from '../../app/WorkspaceProvider';
import { normalizeSearch } from '../../shared/accounting/money';
import { downloadCsv } from '../../shared/accounting/csv';
import { filterCatalogPartners, filterCatalogProjects, partnerRoles } from '../../shared/data/partner-catalog';
import type { PartnerRoleFilter } from '../../shared/data/partner-catalog';
import { workspaceKey } from '../../shared/data/workspace';
import type { Account, Partner, Project } from '../../shared/data/types';
import { DataBoundary, PageHeading, PageToolbar } from '../../shared/ui/Page';
import { Icon } from '../../shared/ui/Icon';
import { ImportExcelButton } from '../imports/ExcelImport';
import { ExportMenu } from '../../shared/ui/ExportMenu';
import type { ExportFormat } from '../../shared/ui/ExportMenu';
import { ToolbarButton } from '../../shared/ui/ToolbarButton';
import styles from '../../app/App.module.css';
import { apiScope, command, downloadApi, isDemo } from '../../shared/api/client';
import { PartnerEditor } from './PartnerEditor';
import { ProjectEditor } from './ProjectEditor';

interface AccountParentNode { value: string; title: string; disabled: boolean; children?: AccountParentNode[] }

export function Component() {
  const location = useLocation();
  const [params, setParams] = useSearchParams();
  const module = accountingModule(location.pathname);
  const isProjectsRoute = Boolean(module && location.pathname.endsWith('/projects'));
  const moduleRole = !isProjectsRoute && module && 'tradeKind' in module ? module.tradeKind === 'sale' ? 'customer' : 'supplier' : undefined;
  const { data, period, refresh } = useWorkspace();
  const { message } = App.useApp();
  const client = useQueryClient();
  const [search, setSearch] = useState('');
  const [expandedAccounts, setExpandedAccounts] = useState<string[] | null>(null);
  const requestedTab = params.get('tab');
  const [selectedTab, setTab] = useState(requestedTab === 'projects' || requestedTab === 'accounts' ? requestedTab : 'partners');
  const tab = isProjectsRoute ? 'projects' : moduleRole ? 'partners' : requestedTab === 'projects' || requestedTab === 'accounts' || requestedTab === 'partners' ? requestedTab : selectedTab;
  const [selectedRole, setRole] = useState<PartnerRoleFilter>('all');
  const role = moduleRole ?? selectedRole;
  const [editing, setEditing] = useState<Partner | null>(null);
  const [editingProject, setEditingProject] = useState<Project | null>(null);
  const [account, setAccount] = useState<Account | null>(null);
  const [accountForm] = Form.useForm<Account>();
  const canEdit = isDemo || apiScope().permissions !== 'viewer';
  const accountMutation = useMutation({ mutationFn: ({ id, code, name, group, requiresPartner, active, parentId, parentRevision }: Account) => command<Account>('/accounts', { id, code, name, group, requiresPartner, active, parentId, parentRevision }), retry: false, onSuccess: async result => { setAccount(null); if (result.parentId) setExpandedAccounts(current => current === null ? null : [...new Set([...current, result.parentId!])]); void message.success('Đã lưu tài khoản.'); await client.invalidateQueries({ queryKey: workspaceKey() }); }, onError: error => void message.error(error.message) });
  function editAccount(value?: Account, parent?: Account) { const next = value ?? { id: crypto.randomUUID(), code: '', name: '', group: parent?.group ?? 'expense' as const, requiresPartner: parent?.requiresPartner ?? false, active: true, parentId: parent?.id ?? '', parentRevision: 0 }; setAccount(next); accountForm.resetFields(); accountForm.setFieldsValue(next); }
  if (!data) return <DataBoundary><></></DataBoundary>;
  const partners = filterCatalogPartners(data.partners, role, search);
  const projects = filterCatalogProjects(data.projects ?? [], search);
  const accounts = data.accounts.filter(item => normalizeSearch(`${item.code} ${item.name}`).includes(normalizeSearch(search)));
  const tree = accountTree(data.accounts, search);
  const branches = accountBranchKeys(tree);
  const blockedParents = account ? accountDescendants(data.accounts, account.id) : new Set<string>();
  const parentNodes = (items: AccountNode[]): AccountParentNode[] => items.map(item => ({ value: item.id, title: `${item.code} · ${item.name}`, disabled: item.active === false || blockedParents.has(item.id), children: item.children ? parentNodes(item.children) : undefined }));
  const existingPartner = Boolean(editing && data.partners.some(item => item.id === editing.id));
  const existingProject = Boolean(editingProject && (data.projects ?? []).some(item => item.id === editingProject.id));
  const referenced = existingPartner && editing ? isDemo ? data.documents.some(item => item.partnerId === editing.id || item.lines.some(line => line.partnerId === editing.id)) : Boolean(editing.hasReferences) : false;

  function edit(partner: Partner | null) {
    setEditing(partner ?? { id: crypto.randomUUID(), code: '', name: '', role: moduleRole ?? 'customer', taxCode: '', address: '', email: '', phone: '', active: true });
  }
  function editProject(project: Project | null) {
    setEditingProject(project ?? { id: crypto.randomUUID(), code: '', name: '', description: '', active: true });
  }

  function exportData(format: ExportFormat) {
    if (!isDemo) {
      const path = `/catalog/export?${new URLSearchParams({ period, catalog: tab, role, q: search, revision: String(data!.revision), format })}`;
      if (format === 'print') { window.open(`/api/v1${path}`, '_blank', 'noopener,noreferrer'); return; }
      return downloadApi(path, `danh-muc-${tab}.${format}`);
    }
    downloadCsv(`danh-muc-${isDemo ? 'mau-' : ''}${tab}.csv`, tab === 'partners' ? [[isDemo ? 'DỮ LIỆU MẪU · Phiên bản' : `${data!.companyName} · Phiên bản`, String(data!.revision)], ['Mã', 'Tên', 'Mã số thuế', 'Địa chỉ', 'Vai trò', 'Email', 'Điện thoại', 'Trạng thái'], ...partners.map(item => [item.code, item.name, item.taxCode ? `'${item.taxCode}` : '', item.address ?? '', partnerRoles[item.role], item.email, `'${item.phone}`, item.active ? 'Đang sử dụng' : 'Ngừng sử dụng'])] : tab === 'projects' ? [[isDemo ? 'DỮ LIỆU MẪU · Phiên bản' : `${data!.companyName} · Phiên bản`, String(data!.revision)], ['Mã công trình', 'Tên công trình', 'Mô tả', 'Trạng thái'], ...projects.map(item => [item.code, item.name, item.description, item.active ? 'Đang sử dụng' : 'Ngừng sử dụng'])] : [[isDemo ? 'DỮ LIỆU MẪU' : data!.companyName ?? 'Danh mục doanh nghiệp'], ['Mã tài khoản', 'Tên tài khoản', 'Tài khoản cha', 'Yêu cầu đối tượng'], ...accounts.map(item => [item.code, item.name, data!.accounts.find(parent => parent.id === item.parentId)?.code ?? '', item.requiresPartner ? 'Có' : 'Không'])]);
  }

  const reportBase = module ? `${module.path}/reports` : '/reports';
  const pageActions = <>{(tab === 'partners' || tab === 'projects' || tab === 'accounts' && apiScope().permissions === 'owner') && <ImportExcelButton kind={tab === 'partners' ? 'partners' : tab === 'projects' ? 'projects' : 'accounts'} />}{tab === 'projects' && <Link to={`${reportBase}?tab=project`}><ToolbarButton label="Doanh thu & chi phí công trình" icon="chart" /></Link>}{tab === 'accounts' && <><ToolbarButton label="Mở rộng tất cả tài khoản" icon="arrowDown" disabled={Boolean(search)} onClick={() => setExpandedAccounts(branches)} /><ToolbarButton label="Thu gọn tất cả tài khoản" icon="arrowUp" disabled={Boolean(search)} onClick={() => setExpandedAccounts([])} /></>}<ExportMenu onReload={refresh} onExport={exportData} formats={isDemo ? ['csv'] : undefined} />{tab === 'accounts' && !isDemo && apiScope().permissions === 'owner' && <ToolbarButton type="primary" onClick={() => editAccount()} label="Thêm tài khoản" icon="plus" />}{tab === 'partners' && canEdit && <ToolbarButton type="primary" onClick={() => edit(null)} label="Thêm đối tượng" icon="plus" />}{tab === 'projects' && canEdit && <ToolbarButton type="primary" onClick={() => editProject(null)} label="Thêm công trình" icon="plus" />}</>;

  return <DataBoundary>
    <PageHeading title={isProjectsRoute ? 'Công trình' : moduleRole === 'supplier' ? 'Nhà cung cấp' : moduleRole === 'customer' ? 'Khách hàng' : 'Danh mục'} description={isProjectsRoute ? 'Khai báo công trình để gắn trên chứng từ mua hàng, bán hàng và tổng hợp doanh thu, chi phí theo từng công trình.' : 'Sắp xếp dữ liệu nền tảng để mỗi lần nhập liệu đều thuận tiện hơn.'} />
    {!moduleRole && !isProjectsRoute && <div className={styles.catalogSummary}>{[{ label: 'Tổng đối tượng', count: data.partners.length, icon: 'users' }, { label: 'Khách hàng', count: data.partners.filter(item => item.role !== 'supplier').length, icon: 'cart' }, { label: 'Nhà cung cấp', count: data.partners.filter(item => item.role !== 'customer').length, icon: 'bag' }, { label: 'Công trình', count: (data.projects ?? []).length, icon: 'chart' }].map(item => <div key={item.label}><span>{item.label}</span><strong>{item.count}</strong></div>)}</div>}
    <section className={styles.panel}>{!moduleRole && !isProjectsRoute && <div className={styles.listTabs}><Tabs activeKey={tab} onChange={value => { setTab(value); setParams(value === 'partners' ? {} : { tab: value }, { replace: true }); setSearch(''); }} items={[{ key: 'partners', label: 'Khách hàng & nhà cung cấp' }, { key: 'projects', label: 'Công trình' }, { key: 'accounts', label: isDemo ? 'Hệ thống tài khoản mẫu' : 'Hệ thống tài khoản' }]} /></div>}
      <PageToolbar label="Công cụ danh mục" actions={pageActions} activeFilters={Number(Boolean(search.trim())) + Number(tab === 'partners' && !moduleRole && role !== 'all')} onResetFilters={() => { setSearch(''); if (!moduleRole) setRole('all'); }}><Input prefix={<Icon name="search" size={17} />} aria-label="Tìm trong danh mục" placeholder={tab === 'partners' ? 'Tìm mã, tên, MST, địa chỉ hoặc liên hệ…' : tab === 'projects' ? 'Tìm mã, tên hoặc mô tả công trình…' : 'Tìm mã hoặc tên tài khoản…'} value={search} allowClear onChange={event => setSearch(event.target.value)} className={styles.filterSearch} />{tab === 'partners' && !moduleRole && <Select aria-label="Lọc vai trò đối tượng" value={role} onChange={setRole} style={{ minWidth: 180 }} options={[{ value: 'all', label: 'Tất cả vai trò' }, { value: 'customer', label: 'Khách hàng' }, { value: 'supplier', label: 'Nhà cung cấp' }]} />}</PageToolbar>
      {tab === 'partners' ? <Table<Partner> rowKey="id" dataSource={partners} pagination={{ pageSize: 8, hideOnSinglePage: true }} scroll={{ x: 960 }} columns={[
        { title: 'MÃ ĐỐI TƯỢNG', dataIndex: 'code', width: 130, render: (code: string, item) => canEdit ? <button className={styles.tableLink} onClick={() => edit(item)}>{code}</button> : <strong>{code}</strong> },
        { title: 'TÊN ĐỐI TƯỢNG', dataIndex: 'name', render: (name: string, item) => <div className={styles.partnerCell}><span className={styles.partnerAvatar}>{name.replace(/^Công ty (TNHH|CP) /, '').slice(0, 1)}</span><span><strong>{name}</strong><small className={styles.cellSub}>{item.address || item.email || 'Chưa có địa chỉ'}</small></span></div> },
        { title: 'MÃ SỐ THUẾ', dataIndex: 'taxCode', width: 135, render: (value?: string) => value || '—' },
        { title: 'VAI TRÒ', dataIndex: 'role', width: 145, render: (value: Partner['role']) => <Tag color={value === 'supplier' ? 'blue' : 'cyan'} variant="filled">{partnerRoles[value]}</Tag> },
        { title: 'ĐIỆN THOẠI', dataIndex: 'phone', width: 125, render: (value: string) => value || '—' },
        { title: 'TRẠNG THÁI', dataIndex: 'active', width: 125, render: (value: boolean) => <span className={value ? styles.positive : styles.muted}>{value ? 'Đang sử dụng' : 'Ngừng sử dụng'}</span> },
        { width: 45, render: (_, item) => canEdit && <Button type="text" icon={<Icon name="chevron" size={16} />} onClick={() => edit(item)} aria-label={`Sửa ${item.code}`} /> },
      ]} /> : tab === 'projects' ? <Table<Project> rowKey="id" dataSource={projects} pagination={{ pageSize: 10, hideOnSinglePage: true }} scroll={{ x: 820 }} columns={[
        { title: 'MÃ CÔNG TRÌNH', dataIndex: 'code', width: 150, render: (code: string, item) => canEdit ? <button className={styles.tableLink} onClick={() => editProject(item)}>{code}</button> : <strong>{code}</strong> },
        { title: 'TÊN CÔNG TRÌNH', dataIndex: 'name', render: (name: string, item) => <><strong>{name}</strong>{item.description && <small className={styles.cellSub}>{item.description}</small>}</> },
        { title: 'TRẠNG THÁI', dataIndex: 'active', width: 135, render: (value: boolean) => <span className={value ? styles.positive : styles.muted}>{value ? 'Đang sử dụng' : 'Ngừng sử dụng'}</span> },
        { title: 'DOANH THU & CHI PHÍ', width: 155, render: (_, item) => <Link to={`${reportBase}?tab=project&project=${encodeURIComponent(item.id)}`}>Xem sổ công trình</Link> },
        { width: 45, render: (_, item) => canEdit && <Button type="text" icon={<Icon name="chevron" size={16} />} onClick={() => editProject(item)} aria-label={`Sửa công trình ${item.code}`} /> },
      ]} /> : <><Table<AccountNode> rowKey="id" dataSource={tree} pagination={false} scroll={{ x: 1050 }} expandable={{ expandedRowKeys: search ? branches : expandedAccounts ?? branches, onExpandedRowsChange: keys => setExpandedAccounts(keys.map(String)), indentSize: 22, columnTitle: 'Mã tài khoản' }} columns={[
        { title: 'MÃ TÀI KHOẢN', dataIndex: 'code', width: 230, render: (value: string, row) => <strong className={styles.positive} title={row.path}>{value}</strong> },
        { title: 'TÊN TÀI KHOẢN', dataIndex: 'name', render: (value: string, row) => <span title={row.path}>{row.childCount ? <strong>{value}</strong> : value}{row.childCount > 0 && <Tag style={{ marginLeft: 8 }}>{row.childCount} tài khoản con</Tag>}</span> },
        { title: 'CẤP', dataIndex: 'level', width: 70, render: value => `Cấp ${value}` },
        { title: 'TRẠNG THÁI', dataIndex: 'active', width: 130, render: value => value === false ? <span className={styles.muted}>Ngừng sử dụng</span> : 'Đang sử dụng' },
        { title: 'ĐỐI TƯỢNG', dataIndex: 'requiresPartner', width: 170, render: value => value ? 'Bắt buộc theo dòng' : 'Không bắt buộc' },
        ...(!isDemo && apiScope().permissions === 'owner' ? [{ title: 'CHỨC NĂNG', width: 110, render: (_: unknown, row: AccountNode) => <><Button type="text" size="small" onClick={() => editAccount(row)} aria-label={`Sửa tài khoản ${row.code}`}>Sửa</Button><ToolbarButton label={`Thêm tài khoản con của ${row.code}`} icon="plus" onClick={() => editAccount(undefined, row)} /></> }] : []),
      ]} /></>}
    </section>
    <Modal title="Tài khoản kế toán" open={Boolean(account)} onCancel={() => setAccount(null)} footer={null}><Form<Account> form={accountForm} layout="vertical" onFinish={values => accountMutation.mutate({ ...account!, ...values })}><div className={styles.formGrid}><Form.Item name="code" label="Mã tài khoản" rules={[{ required: true }]}><Input disabled={accountMutation.isPending} /></Form.Item><Form.Item name="group" label="Nhóm nghiệp vụ" rules={[{ required: true }]}><Select disabled={accountMutation.isPending} options={Object.entries({ wip: 'Chi phí dở dang', cash: 'Tiền', receivable: 'Phải thu', payable: 'Phải trả', revenue: 'Doanh thu', expense: 'Chi phí', inventory: 'Kho', capital: 'Nguồn vốn / kết quả', asset: 'Tài sản', tax: 'Thuế', liability: 'Nợ phải trả khác' }).map(([value, label]) => ({ value, label }))} /></Form.Item></div><Form.Item name="parentId" label="Tài khoản cha" getValueProps={value => ({ value: value || undefined })} normalize={value => value ?? ''}><TreeSelect disabled={accountMutation.isPending} aria-label="Tài khoản cha" allowClear showSearch treeLine treeDefaultExpandAll placeholder="Không có · tài khoản cấp 1" treeData={parentNodes(accountTree(data.accounts))} filterTreeNode={(input, node) => normalizeSearch(String(node.title)).includes(normalizeSearch(input))} onChange={value => { if (!data.accounts.some(item => item.id === account?.id)) { const parent = data.accounts.find(item => item.id === value); if (parent) accountForm.setFieldsValue({ group: parent.group, requiresPartner: parent.requiresPartner }); } }} /></Form.Item><Form.Item name="name" label="Tên tài khoản" rules={[{ required: true }]}><Input disabled={accountMutation.isPending} maxLength={200} /></Form.Item><div className={styles.formGrid}><Form.Item name="requiresPartner" label="Bắt buộc đối tượng theo dòng" valuePropName="checked"><Switch disabled={accountMutation.isPending} /></Form.Item><Form.Item name="active" label="Đang sử dụng" valuePropName="checked"><Switch /></Form.Item></div><p className={styles.muted}>Bạn có thể sửa mã, tên, nhóm nghiệp vụ, tài khoản cha, yêu cầu đối tượng và trạng thái sử dụng. Thay đổi được ghi nhật ký.</p><Button type="primary" htmlType="submit" loading={accountMutation.isPending}>Lưu tài khoản</Button></Form></Modal>
    {editing && <PartnerEditor key={editing.id} partner={editing} existing={existingPartner} referenced={referenced} onClose={() => setEditing(null)} />}
    {editingProject && <ProjectEditor key={editingProject.id} project={editingProject} existing={existingProject} onClose={() => setEditingProject(null)} />}
  </DataBoundary>;
}
