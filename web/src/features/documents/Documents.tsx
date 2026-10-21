import { useState } from 'react';
import { Alert, App, Button, Input, Select, Table, Tabs, Tag } from 'antd';
import { useLocation, useSearchParams } from 'react-router-dom';
import { useWorkspace } from '../../app/WorkspaceProvider';
import { accountBalance, filterDocuments, periodDocuments, totals } from '../../shared/accounting/ledger';
import { downloadCsv } from '../../shared/accounting/csv';
import { formatDate, formatMoney } from '../../shared/accounting/money';
import { documentTypes, documentStatuses } from '../../shared/data/types';
import type { AccountingDocument, DocumentType } from '../../shared/data/types';
import { DataBoundary, EmptyData, PageHeading, ScopeNote, StatusTag } from '../../shared/ui/Page';
import { Icon } from '../../shared/ui/Icon';
import styles from '../../app/App.module.css';
import { api, apiScope, downloadApi, isDemo } from '../../shared/api/client';
import { useQuery } from '@tanstack/react-query';
import { workspaceKey } from '../../shared/data/workspace';
import type { DocumentList } from '../../shared/data/reports';
import { useNavigate } from 'react-router-dom';
import { ImportDocuments } from './ImportDocuments';

const configs: Record<string, { title: string; description: string; type?: DocumentType; accounts?: string[] }> = {
  '/documents': { title: 'Kế toán tổng hợp', description: 'Mỗi nghiệp vụ một chứng từ. Mọi con số đều có nguồn.' },
  '/sales': { title: 'Bán hàng', description: 'Quản lý chứng từ bán hàng và theo dõi doanh thu.', type: 'sale' },
  '/purchases': { title: 'Mua hàng', description: 'Theo dõi chứng từ mua hàng và nhà cung cấp.', type: 'purchase' },
  '/cash': { title: 'Tiền mặt & ngân hàng', description: 'Tra cứu thu, chi và số dư các tài khoản tiền.', type: 'receipt', accounts: ['111', '1121'] },
  '/inventory': { title: 'Hàng tồn kho', description: 'Tra cứu giá trị hàng hóa từ các bút toán nguồn.', type: 'purchase', accounts: ['156'] },
};

export function Component() {
  const { pathname } = useLocation();
  const { period } = useWorkspace();
  const config = configs[pathname] ?? configs['/documents'];
  return <DocumentsPage key={`${pathname}-${period}`} config={config} />;
}

function DocumentsPage({ config }: { config: typeof configs[string] }) {
  const { data, period, newDraft, openDocument, refresh } = useWorkspace();
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const [type, setType] = useState<string>('all');
  const [page, setPage] = useState(1);
  const [sort, setSort] = useState('date');
  const [direction, setDirection] = useState<'asc' | 'desc'>('desc');
  const { message } = App.useApp();
  const search = params.get('q') ?? '';
  const status = params.get('status') ?? 'all';
  const updateFilter = (key: string, value: string) => { setPage(1); setParams(previous => { const next = new URLSearchParams(previous); if (value && value !== 'all') next.set(key, value); else next.delete(key); return next; }, { replace: true }); };
  const serverParams = new URLSearchParams({ period, page: String(page), pageSize: '8', q: search, status, type: type !== 'all' ? type : config.accounts ? 'all' : config.type ?? 'all', account: config.accounts?.join(',') ?? 'all', sort, direction, revision: String(data?.revision ?? 0) });
  const serverQuery = useQuery({ queryKey: [...workspaceKey(), 'documents', serverParams.toString()], queryFn: ({ signal }) => api<DocumentList>(`/documents?${serverParams}`, { signal }), enabled: !isDemo && Boolean(data), retry: false });
  if (!data) return <DataBoundary><></></DataBoundary>;
  const scope = periodDocuments(data.documents, period).filter(item => config.accounts ? item.lines.some(line => config.accounts?.includes(line.accountId)) : !config.type || item.type === config.type);
  const filtered = isDemo ? filterDocuments(scope, search, data.partners).filter(item => (status === 'all' || item.status === status) && (type === 'all' || item.type === type)) : serverQuery.data?.documents ?? [];
  const filteredTotal = isDemo ? filtered.reduce((sum, item) => sum + totals(item.lines).debit, 0n) : BigInt(serverQuery.data?.amount ?? '0');
  const count = isDemo ? filtered.length : serverQuery.data?.total ?? 0;
  const counts = serverQuery.data?.counts;
  const canWrite = isDemo || apiScope().permissions !== 'viewer';
  const exportRows = () => { if (!isDemo) { void downloadApi(`/documents/export?${serverParams}`, `chung-tu-${period}.csv`).catch(error => void message.error((error as Error).message)); return; } downloadCsv(`chung-tu-mau-${period}.csv`, [
    ['DỮ LIỆU MẪU', `Kỳ ${period}`, `Phiên bản ${data.revision}`, 'Đơn vị VND'],
    ['Số chứng từ', 'Ngày', 'Loại', 'Diễn giải', 'Đối tượng', 'Tổng Nợ (VND)', 'Trạng thái'],
    ...filtered.map(item => [item.reference, item.date, documentTypes[item.type], item.description, data.partners.find(partner => partner.id === item.partnerId)?.name ?? '', `${item.lines.length ? formatMoney(totals(item.lines).debit) : '0'} VND`, documentStatuses[item.status]]),
  ]); };

  return <DataBoundary>
    <PageHeading title={config.title} description={config.description} actions={<>{!isDemo && canWrite && <ImportDocuments />}<Button icon={<Icon name="download" size={16} />} onClick={exportRows} disabled={!filtered.length}>Xuất CSV</Button>{canWrite && <Button type="primary" icon={<Icon name="plus" size={17} />} onClick={() => !isDemo && config.type && ['sale', 'purchase'].includes(config.type) ? navigate(`/automation?kind=${config.type}`) : newDraft(config.type)}>{!isDemo && config.type && ['sale', 'purchase'].includes(config.type) ? 'Lập nghiệp vụ tự động' : 'Tạo chứng từ'}</Button>}</>} />
    {config.accounts && <div className={styles.accountCards}>{config.accounts.map(id => <div key={id} className={styles.accountCard}><span className={styles.metricIcon}><Icon name={id === '156' ? 'box' : 'wallet'} /></span><div><span>{data.accounts.find(item => item.id === id)?.name} · {id}</span><strong>{formatMoney(data.server ? data.server.balances[id] : accountBalance(data.documents, [id], period))}<small>VND</small></strong></div><span className={styles.softLabel}>{isDemo ? 'Số dư cuối kỳ mẫu' : 'Số dư cuối kỳ'}</span></div>)}</div>}
    {config.accounts?.includes('156') && <div className={styles.inlineNotice}><Icon name="help" size={16} />{isDemo ? 'Sổ này phản ánh giá trị hạch toán mẫu. Số lượng, tính giá xuất kho và kiểm kê cần phân hệ kho cùng chính sách đã duyệt.' : 'Xem số lượng, lô và giá trị tồn ở Nghiệp vụ tự động → Hàng hóa & dịch vụ. Biến động kho được ghi cùng transaction với chứng từ.'}</div>}
    {serverQuery.error && <Alert type="error" title={serverQuery.error.message} action={<Button onClick={() => { setPage(1); refresh(); }}>Tải lại dữ liệu</Button>} />}
    <section className={styles.panel}>
      <div className={styles.listTabs}><Tabs activeKey={status} onChange={value => updateFilter('status', value)} items={[
        { key: 'all', label: <>Tất cả <span className={styles.countBadge}>{isDemo ? scope.length : counts?.all ?? '—'}</span></> },
        { key: 'posted', label: <>Đã ghi sổ <span className={styles.countBadge}>{isDemo ? scope.filter(item => item.status === 'posted').length : counts?.posted ?? '—'}</span></> },
        { key: 'draft', label: <>Bản nháp <span className={styles.countBadge}>{isDemo ? scope.filter(item => item.status === 'draft').length : counts?.draft ?? '—'}</span></> },
      ]} /></div>
      <div className={styles.tableToolbar}><Input prefix={<Icon name="search" size={17} />} placeholder="Tìm số chứng từ, diễn giải, đối tượng…" aria-label="Tìm trong danh sách chứng từ" value={search} onChange={event => updateFilter('q', event.target.value)} allowClear className={styles.filterSearch} /><div>{!config.type && <Select aria-label="Lọc loại chứng từ" value={type} onChange={value => { setType(value); setPage(1); }} style={{ minWidth: 170 }} options={[{ value: 'all', label: 'Tất cả loại chứng từ' }, ...Object.entries(documentTypes).map(([value, label]) => ({ value, label }))]} />}<Tag className={styles.demoPill}>{isDemo ? 'Dữ liệu mẫu' : 'Dữ liệu doanh nghiệp'}</Tag></div></div>
      <Table<AccountingDocument> loading={!isDemo && serverQuery.isPending} onChange={(_, __, sorter, extra) => { if (isDemo || extra.action !== 'sort') return; const current = Array.isArray(sorter) ? sorter[0] : sorter; setPage(1); setSort(current.columnKey === 'amount' ? 'amount' : 'date'); setDirection(current.order === 'ascend' ? 'asc' : 'desc'); }} rowKey="id" dataSource={filtered} scroll={{ x: 950 }} pagination={{ current: page, total: count, onChange: setPage, pageSize: 8, showSizeChanger: false, hideOnSinglePage: true, showTotal: total => `${total} chứng từ` }} locale={{ emptyText: <EmptyData action={<Button onClick={() => { setType('all'); setParams({}); }}>Xóa bộ lọc</Button>} /> }} columns={[
        { title: 'SỐ CHỨNG TỪ', dataIndex: 'reference', width: 135, render: (value: string, item) => <button className={styles.tableLink} onClick={() => openDocument(item.id)}>{value}</button> },
        { title: 'NGÀY', dataIndex: 'date', width: 105, sorter: isDemo ? (a, b) => a.date.localeCompare(b.date) : true, render: formatDate },
        { title: 'DIỄN GIẢI / ĐỐI TƯỢNG', dataIndex: 'description', render: (value: string, item) => <><span>{value}</span><small className={styles.cellSub}>{data.partners.find(partner => partner.id === item.partnerId)?.name ?? 'Không có đối tượng chung'}</small></> },
        { title: 'LOẠI', dataIndex: 'type', width: 120, render: (value: DocumentType) => documentTypes[value] },
        { title: 'TỔNG NỢ (VND)', key: 'amount', width: 150, align: 'right', sorter: isDemo ? (a, b) => { const diff = totals(a.lines).debit - totals(b.lines).debit; return diff < 0n ? -1 : diff > 0n ? 1 : 0; } : true, render: (_, item) => <strong className={styles.tableAmount}>{formatMoney(totals(item.lines).debit)}</strong> },
        { title: 'TRẠNG THÁI', dataIndex: 'status', width: 125, render: value => <StatusTag status={value} /> },
        { width: 46, render: (_, item) => <Button type="text" icon={<Icon name="chevron" size={15} />} aria-label={`Xem ${item.reference}`} onClick={() => openDocument(item.id)} /> },
      ]} />
      <div className={styles.tableTotal}><span>{serverQuery.isPending && !isDemo ? 'Đang tải…' : count} chứng từ trong toàn bộ tập lọc <small>· Tổng Nợ, gồm cả bản nháp</small></span><strong>{!isDemo && !serverQuery.data ? '—' : formatMoney(filteredTotal)}<small>VND</small></strong></div>
    </section><ScopeNote />
  </DataBoundary>;
}
