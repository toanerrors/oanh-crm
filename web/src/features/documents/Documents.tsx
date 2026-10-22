import { useState } from 'react';
import { Alert, App, Button, Grid, Input, Select, Table, Tabs, Tag } from 'antd';
import { useLocation, useSearchParams } from 'react-router-dom';
import { useWorkspace } from '../../app/WorkspaceProvider';
import { accountBalance, filterDocuments, periodDocuments, totals } from '../../shared/accounting/ledger';
import { downloadCsv } from '../../shared/accounting/csv';
import { formatDate, formatMoney } from '../../shared/accounting/money';
import { documentTypes, documentStatuses } from '../../shared/data/types';
import type { AccountingDocument, DocumentType } from '../../shared/data/types';
import { DataBoundary, EmptyData, PageHeading, StatusTag } from '../../shared/ui/Page';
import { Icon } from '../../shared/ui/Icon';
import styles from '../../app/App.module.css';
import { api, apiScope, downloadApi, isDemo } from '../../shared/api/client';
import { useQuery } from '@tanstack/react-query';
import { workspaceKey } from '../../shared/data/workspace';
import { banksOptions } from '../../shared/data/operations';
import { BankAccountsButton } from '../vouchers/BankAccounts';
import type { DocumentList } from '../../shared/data/reports';
import { useNavigate } from 'react-router-dom';
import { ImportDocuments } from './ImportDocuments';

const configs: Record<string, { title: string; description: string; type?: DocumentType; accounts?: string[]; treasury?: boolean }> = {
  '/documents': { title: 'Kế toán tổng hợp', description: 'Mỗi nghiệp vụ một chứng từ. Mọi con số đều có nguồn.' },
  '/sales': { title: 'Bán hàng', description: 'Quản lý chứng từ bán hàng và theo dõi doanh thu.', type: 'sale' },
  '/purchases': { title: 'Mua hàng', description: 'Theo dõi chứng từ mua hàng và nhà cung cấp.', type: 'purchase' },
  '/cash': { title: isDemo ? 'Tiền mặt & ngân hàng' : 'Tiền mặt', description: 'Tra cứu thu, chi và số dư đã ghi sổ trong kỳ.', accounts: isDemo ? ['111', '1121'] : ['111'], treasury: true },
  '/bank': { title: 'Tiền gửi', description: 'Tra cứu thu, chi tiền gửi và chuyển tiền nội bộ.', accounts: ['1121', '112'], treasury: true },
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
  const screens = Grid.useBreakpoint();
  const compact = screens.md === false;
  const [params, setParams] = useSearchParams();
  const [type, setType] = useState<string>('all');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(isDemo ? 8 : 20);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [sort, setSort] = useState('date');
  const [direction, setDirection] = useState<'asc' | 'desc'>('desc');
  const { message } = App.useApp();
  const search = params.get('q') ?? '';
  const status = params.get('status') ?? 'all';
  const updateFilter = (key: string, value: string) => { setPage(1); setParams(previous => { const next = new URLSearchParams(previous); if (value && value !== 'all') next.set(key, value); else next.delete(key); return next; }, { replace: true }); };
  const bankPage = config.title === 'Tiền gửi';
  const banks = useQuery({ ...banksOptions(period), enabled: !isDemo && bankPage && Boolean(data) });
  const selectedBank = banks.data?.accounts.find(item => item.id === params.get('bank'));
  const accountIds = bankPage ? [...new Set([...(config.accounts ?? []), ...(banks.data?.accounts.map(item => item.accountId) ?? [])])] : config.accounts;
  const serverParams = new URLSearchParams({ period, page: String(page), pageSize: String(pageSize), bank: params.get('bank') ?? '', source: params.get('source') ?? '', q: search, status, type: type !== 'all' ? type : config.accounts ? 'all' : config.type ?? 'all', account: accountIds?.join(',') ?? 'all', sort, direction, revision: String(data?.revision ?? 0) });
  const serverQuery = useQuery({ queryKey: [...workspaceKey(), 'documents', serverParams.toString()], queryFn: ({ signal }) => api<DocumentList>(`/documents?${serverParams}`, { signal }), enabled: !isDemo && Boolean(data), retry: false });
  if (!data) return <DataBoundary><></></DataBoundary>;
  const scope = periodDocuments(data.documents, period).filter(item => config.accounts ? item.lines.some(line => config.accounts?.includes(line.accountId)) : !config.type || item.type === config.type);
  function openEntry(item: AccountingDocument) {
    if (!isDemo && (item.voucher || item.policyId && ['receipt', 'payment', 'transfer', 'sale', 'purchase', 'closing'].includes(item.sourceType ?? ''))) navigate(`/vouchers/${item.id}`);
    else openDocument(item.id);
  }
  const filtered = isDemo ? filterDocuments(scope, search, data.partners).filter(item => (status === 'all' || item.status === status) && (type === 'all' || item.type === type)) : serverQuery.data?.documents ?? [];
  const filteredTotal = isDemo ? filtered.reduce((sum, item) => sum + totals(item.lines).debit, 0n) : BigInt(serverQuery.data?.amount ?? '0');
  const count = isDemo ? filtered.length : serverQuery.data?.total ?? 0;
  const rows = isDemo ? [...filtered].sort((a, b) => {
    let comparison = a.date.localeCompare(b.date);
    if (sort === 'amount') {
      const difference = totals(a.lines).debit - totals(b.lines).debit;
      comparison = difference < 0n ? -1 : difference > 0n ? 1 : 0;
    }
    return direction === 'asc' ? comparison : -comparison;
  }) : filtered;
  const selected = rows.find(item => item.id === selectedId) ?? rows[0];
  const counts = serverQuery.data?.counts;
  const canWrite = isDemo || apiScope().permissions !== 'viewer';
  const exportRows = () => { if (!isDemo) { void downloadApi(`/documents/export?${serverParams}`, `chung-tu-${period}.csv`).catch(error => void message.error((error as Error).message)); return; } downloadCsv(`chung-tu-mau-${period}.csv`, [
    ['DỮ LIỆU MẪU', `Kỳ ${period}`, `Phiên bản ${data.revision}`, 'Đơn vị VND'],
    ['Số chứng từ', 'Ngày', 'Loại', 'Diễn giải', 'Đối tượng', 'Tổng Nợ (VND)', 'Trạng thái'],
    ...filtered.map(item => [item.reference, item.date, documentTypes[item.type], item.description, data.partners.find(partner => partner.id === item.partnerId)?.name ?? '', `${item.lines.length ? formatMoney(totals(item.lines).debit) : '0'} VND`, documentStatuses[item.status]]),
  ]); };

  const pageActions = <>{!isDemo && canWrite && <ImportDocuments />}{!isDemo && bankPage && <BankAccountsButton />}{!isDemo && config.type && ['sale', 'purchase'].includes(config.type) && <Button onClick={() => navigate(`${config.type === 'sale' ? '/sales' : '/purchases'}/settlements`)}>Công nợ theo chứng từ</Button>}{!isDemo && config.type === 'purchase' && canWrite && <Button onClick={() => navigate('/automation?kind=asset')}>Mua tài sản / chi phí trả trước</Button>}{!isDemo && config.treasury && canWrite && <><Button onClick={() => navigate(`/vouchers/new?kind=payment&account=${config.accounts?.[0]}`)}>Chi tiền</Button><Button onClick={() => navigate(`/vouchers/new?kind=transfer&account=${config.accounts?.[0]}`)}>Chuyển tiền</Button></>}{!isDemo && apiScope().permissions === 'owner' && !config.accounts && !config.type && <><Button onClick={() => navigate('/vouchers/new?kind=closing')}>Kết chuyển lãi lỗ</Button><Button onClick={() => navigate('/settings')}>Quản lý / khóa kỳ</Button></>}<Button icon={<Icon name="download" size={16} />} onClick={exportRows} disabled={!filtered.length}>Xuất CSV</Button>{canWrite && <>{!isDemo && config.type && ['sale', 'purchase'].includes(config.type) && <Button onClick={() => navigate(`/vouchers/new?kind=${config.type}&service=1`)}>{config.type === 'sale' ? 'Bán dịch vụ' : 'Mua dịch vụ'}</Button>}<Button type="primary" icon={<Icon name="plus" size={17} />} onClick={() => !isDemo && config.treasury ? navigate(`/vouchers/new?kind=receipt&account=${config.accounts?.[0]}`) : !isDemo && config.type && ['sale', 'purchase'].includes(config.type) ? navigate(`/vouchers/new?kind=${config.type}`) : newDraft(config.treasury ? 'receipt' : config.type)}>{!isDemo && config.treasury ? 'Thu tiền' : !isDemo && config.type && ['sale', 'purchase'].includes(config.type) ? config.type === 'sale' ? 'Bán hàng' : 'Mua hàng' : 'Tạo chứng từ'}</Button></>}</>;
  return <DataBoundary>
    {isDemo ? <PageHeading title={config.title} description={config.description} /> : <h1 className={styles.visuallyHidden}>{params.get('source') === 'closing' ? 'Kết chuyển lãi lỗ' : config.title}</h1>}

    {!isDemo && config.treasury && <div className={styles.accountCards}>{([['Dư đầu kỳ', 'opening'], ['Thu trong kỳ', 'receipts'], ['Chi trong kỳ', 'payments'], ['Dư cuối kỳ', 'closing']] as const).map(([label, key]) => <div className={styles.accountCard} key={key}><div><span>{label} · TK {selectedBank?.number ?? config.accounts?.join(', ')}</span><strong>{serverQuery.data?.money ? formatMoney(serverQuery.data.money[key]) : '—'}<small>VND</small></strong></div></div>)}</div>}
    {config.accounts && (!config.treasury || isDemo) && <div className={styles.accountCards}>{config.accounts.map(id => <div key={id} className={styles.accountCard}><span className={styles.metricIcon}><Icon name={id === '156' ? 'box' : 'wallet'} /></span><div><span>{data.accounts.find(item => item.id === id)?.name} · {id}</span><strong>{formatMoney(data.server ? data.server.balances[id] : accountBalance(data.documents, [id], period))}<small>VND</small></strong></div><span className={styles.softLabel}>{isDemo ? 'Số dư cuối kỳ mẫu' : 'Số dư cuối kỳ'}</span></div>)}</div>}
    {config.accounts?.includes('156') && <div className={styles.inlineNotice}><Icon name="help" size={16} />{isDemo ? 'Sổ này phản ánh giá trị hạch toán mẫu. Số lượng, tính giá xuất kho và kiểm kê cần phân hệ kho cùng chính sách đã duyệt.' : 'Xem số lượng, lô và giá trị tồn ở Nghiệp vụ tự động → Hàng hóa & dịch vụ. Biến động kho được ghi cùng transaction với chứng từ.'}</div>}
    {serverQuery.error && <Alert type="error" title={serverQuery.error.message} action={<Button onClick={() => { setPage(1); refresh(); }}>Tải lại dữ liệu</Button>} />}
    <section className={`${styles.panel} ${!isDemo ? styles.documentList : ''}`}>
      {isDemo && <div className={styles.listTabs}><Tabs activeKey={status} onChange={value => updateFilter('status', value)} items={[
        { key: 'all', label: <>Tất cả <span className={styles.countBadge}>{isDemo ? scope.length : counts?.all ?? '—'}</span></> },
        { key: 'posted', label: <>Đã ghi sổ <span className={styles.countBadge}>{isDemo ? scope.filter(item => item.status === 'posted').length : counts?.posted ?? '—'}</span></> },
        { key: 'draft', label: <>Bản nháp <span className={styles.countBadge}>{isDemo ? scope.filter(item => item.status === 'draft').length : counts?.draft ?? '—'}</span></> },
      ]} /></div>}
      <div className={styles.tableToolbar}><Input prefix={<Icon name="search" size={17} />} placeholder="Tìm số chứng từ, diễn giải, đối tượng…" aria-label="Tìm trong danh sách chứng từ" value={search} onChange={event => updateFilter('q', event.target.value)} allowClear className={styles.filterSearch} /><div>{!isDemo && bankPage && <Select aria-label="Lọc tài khoản ngân hàng" loading={banks.isPending} value={params.get('bank') ?? 'all'} onChange={value => updateFilter('bank', value)} style={{ minWidth: 220 }} options={[{ value: 'all', label: 'Tất cả tài khoản ngân hàng' }, ...(banks.data?.accounts.map(item => ({ value: item.id, label: `${item.number} · ${item.bankName}` })) ?? [])]} />}{!isDemo && <Select aria-label="Lọc trạng thái chứng từ" value={status} onChange={value => updateFilter('status', value)} style={{ minWidth: 155 }} options={[{ value: 'all', label: `Tất cả (${counts?.all ?? '…'})` }, { value: 'posted', label: `Đã ghi sổ (${counts?.posted ?? '…'})` }, { value: 'draft', label: `Bản nháp (${counts?.draft ?? '…'})` }]} />}{!config.type && <Select aria-label="Lọc loại chứng từ" value={type} onChange={value => { setType(value); setPage(1); }} style={{ minWidth: 170 }} options={[{ value: 'all', label: 'Tất cả loại chứng từ' }, ...Object.entries(documentTypes).map(([value, label]) => ({ value, label }))]} />}{compact && <Select aria-label="Sắp xếp chứng từ" value={`${sort}:${direction}`} style={{ minWidth: 160 }} onChange={value => { const [field, order] = value.split(':'); setSort(field); setDirection(order === 'asc' ? 'asc' : 'desc'); setPage(1); }} options={[{ value: 'date:desc', label: 'Mới nhất' }, { value: 'date:asc', label: 'Cũ nhất' }, { value: 'amount:desc', label: 'Số tiền giảm dần' }, { value: 'amount:asc', label: 'Số tiền tăng dần' }]} />}{isDemo && <Tag className={styles.demoPill}>Dữ liệu mẫu</Tag>}{pageActions}</div></div>
      <Table<AccountingDocument> className={compact ? styles.compactDocuments : undefined} showHeader={!compact} loading={!isDemo && serverQuery.isPending} onChange={(_, __, sorter, extra) => { if (extra.action !== 'sort') return; const current = Array.isArray(sorter) ? sorter[0] : sorter; setPage(1); setSort(current.order && current.columnKey === 'amount' ? 'amount' : 'date'); setDirection(current.order === 'ascend' ? 'asc' : 'desc'); }} rowKey="id" dataSource={rows} onRow={item => ({ onClick: () => setSelectedId(item.id) })} rowClassName={item => item.id === selected?.id && !compact ? styles.selectedDocument : ''} scroll={compact ? undefined : { x: 950, y: isDemo ? undefined : 'max(200px, calc(100vh - 520px))' }} pagination={{ current: page, total: count, onChange: (next, size) => { setPage(size === pageSize ? next : 1); setPageSize(size); }, pageSize, showSizeChanger: !isDemo, pageSizeOptions: [10, 20, 50, 100], hideOnSinglePage: true, showTotal: total => `${total} chứng từ` }} locale={{ emptyText: <EmptyData action={<Button onClick={() => { setType('all'); setParams({}); }}>Xóa bộ lọc</Button>} /> }} columns={compact ? [{ key: 'document', render: (_, item) => <article className={styles.documentCard}>
        <div><button className={styles.tableLink} onClick={() => openEntry(item)}>{item.reference}</button><StatusTag status={item.status} /></div>
        <p>{item.description}</p>
        <span className={styles.cellSub}>{data.partners.find(partner => partner.id === item.partnerId)?.name ?? 'Không có đối tượng chung'}</span>
        <div className={styles.documentCardMeta}><span>{formatDate(item.date)} · {documentTypes[item.type]}</span><span>Tổng Nợ (VND)<strong>{formatMoney(totals(item.lines).debit)}</strong></span></div>
      </article> }] : [
        { title: 'SỐ CHỨNG TỪ', dataIndex: 'reference', width: 155, render: (value: string, item) => <button className={styles.tableLink} onClick={() => openEntry(item)}>{value}</button> },
        { title: 'NGÀY', dataIndex: 'date', width: 105, sortOrder: sort === 'date' ? (direction === 'asc' ? 'ascend' : 'descend') : null, sorter: isDemo ? (a, b) => a.date.localeCompare(b.date) : true, render: formatDate },
        { title: 'DIỄN GIẢI / ĐỐI TƯỢNG', dataIndex: 'description', render: (value: string, item) => <><span>{value}</span><small className={styles.cellSub}>{data.partners.find(partner => partner.id === item.partnerId)?.name ?? 'Không có đối tượng chung'}</small></> },
        { title: 'LOẠI', dataIndex: 'type', width: 120, render: (value: DocumentType, row: AccountingDocument) => row.sourceType === 'transfer' ? 'Chuyển tiền nội bộ' : row.sourceType === 'closing' ? 'Kết chuyển lãi lỗ' : documentTypes[value] },
        { title: 'TỔNG NỢ (VND)', key: 'amount', width: 150, align: 'right', sortOrder: sort === 'amount' ? (direction === 'asc' ? 'ascend' : 'descend') : null, sorter: isDemo ? (a, b) => { const diff = totals(a.lines).debit - totals(b.lines).debit; return diff < 0n ? -1 : diff > 0n ? 1 : 0; } : true, render: (_, item) => <strong className={styles.tableAmount}>{formatMoney(totals(item.lines).debit)}</strong> },
        { title: 'TRẠNG THÁI', dataIndex: 'status', width: 125, render: value => <StatusTag status={value} /> },
        { width: 46, render: (_, item) => <Button type="text" icon={<Icon name="chevron" size={15} />} aria-label={`Xem ${item.reference}`} onClick={() => openEntry(item)} /> },
      ]} />
      <div className={styles.tableTotal}><span>{serverQuery.isPending && !isDemo ? 'Đang tải…' : count} chứng từ trong toàn bộ tập lọc <small>· Tổng Nợ, gồm cả bản nháp</small></span><strong>{!isDemo && !serverQuery.data ? '—' : formatMoney(filteredTotal)}<small>VND</small></strong></div>
    </section>
    {!isDemo && selected && <section className={`${styles.panel} ${styles.documentLines}`} aria-label="Chi tiết chứng từ đang chọn"><div className={styles.panelHeader}><h2>Chi tiết · {selected.reference}</h2><Button type="link" onClick={() => openEntry(selected)}>Mở chứng từ</Button></div><Table rowKey={(_, index) => String(index)} size="small" dataSource={selected.lines} pagination={false} scroll={{ x: 850, y: 280 }} columns={[{ title: '#', width: 42, render: (_, __, index) => index + 1 }, { title: 'Diễn giải', dataIndex: 'description' }, { title: 'Tài khoản', dataIndex: 'accountId', width: 130 }, { title: 'Đối tượng', dataIndex: 'partnerId', width: 230, render: (id: string | undefined) => data.partners.find(item => item.id === id)?.name ?? '—' }, { title: 'Nợ (VND)', dataIndex: 'debit', align: 'right', width: 170, render: formatMoney }, { title: 'Có (VND)', dataIndex: 'credit', align: 'right', width: 170, render: formatMoney }]} summary={() => <Table.Summary.Row><Table.Summary.Cell index={0} colSpan={4}>Tổng</Table.Summary.Cell><Table.Summary.Cell index={4} align="right">{formatMoney(totals(selected.lines).debit)}</Table.Summary.Cell><Table.Summary.Cell index={5} align="right">{formatMoney(totals(selected.lines).credit)}</Table.Summary.Cell></Table.Summary.Row>} /></section>}
  </DataBoundary>;
}
