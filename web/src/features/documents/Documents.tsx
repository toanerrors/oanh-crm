import { useState } from 'react';
import { Alert, Button, Grid, Select, Table, Tabs, Tag } from 'antd';
import { useLocation, useSearchParams } from 'react-router-dom';
import { useWorkspace } from '../../app/WorkspaceProvider';
import { accountBalance, filterDocuments, periodDocuments, totals } from '../../shared/accounting/ledger';
import { downloadCsv } from '../../shared/accounting/csv';
import { formatDate, formatMoney } from '../../shared/accounting/money';
import { documentTypes, documentStatuses } from '../../shared/data/types';
import type { AccountingDocument, DocumentType } from '../../shared/data/types';
import { DataBoundary, EmptyData, PageHeading, StatusTag } from '../../shared/ui/Page';
import { Icon } from '../../shared/ui/Icon';
import { ToolbarButton } from '../../shared/ui/ToolbarButton';
import { ToolbarActionMenu } from '../../shared/ui/ToolbarActionMenu';
import styles from '../../app/App.module.css';
import { api, apiScope, downloadApi, isDemo } from '../../shared/api/client';
import { useQuery } from '@tanstack/react-query';
import { workspaceKey } from '../../shared/data/workspace';
import { banksOptions } from '../../shared/data/operations';
import { BankAccountsButton } from '../vouchers/BankAccounts';
import type { DocumentList } from '../../shared/data/reports';
import { useNavigate } from 'react-router-dom';
import { ImportDocuments } from './ImportDocuments';
import { ExportMenu } from '../../shared/ui/ExportMenu';
import type { ExportFormat } from '../../shared/ui/ExportMenu';
import { DebouncedSearchInput } from '../../shared/ui/DebouncedSearchInput';
import { samePaginationScope } from '../../shared/data/pagination-scope';
import { documentListState } from './list-state';

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
  const { data, period, refresh, newDraft, openDocument } = useWorkspace();
  const location = useLocation();
  const openVoucher = (to: string) => navigate(to, { state: { returnTo: location.pathname + location.search } });
  const navigate = useNavigate();
  const screens = Grid.useBreakpoint();
  const compact = screens.md === false;
  const [params, setParams] = useSearchParams();
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const search = params.get('q') ?? '';
  const { type, page, pageSize, sort, direction, status } = documentListState(params, isDemo);
  const updateFilters = (updates: Record<string, string>, resetPage = true) => setParams(previous => {
    const next = new URLSearchParams(previous);
    for (const [key, value] of Object.entries(updates)) { if (value && value !== 'all') next.set(key, value); else next.delete(key); }
    if (resetPage) next.delete('page');
    return next;
  }, { replace: true });
  const updateFilter = (key: string, value: string) => updateFilters({ [key]: value });
  const searchScope = new URLSearchParams(params); searchScope.delete('q'); searchScope.delete('page');
  const bankPage = config.title === 'Tiền gửi';
  const banks = useQuery({ ...banksOptions(period), enabled: !isDemo && bankPage && Boolean(data) });
  const selectedBank = banks.data?.accounts.find(item => item.id === params.get('bank'));
  const accountIds = bankPage ? [...new Set([...(config.accounts ?? []), ...(banks.data?.accounts.map(item => item.accountId) ?? [])])] : config.accounts;
  const serverParams = new URLSearchParams({ period, page: String(page), pageSize: String(pageSize), bank: params.get('bank') ?? '', source: params.get('source') ?? '', q: search, status, type: type !== 'all' ? type : config.accounts ? 'all' : config.type ?? 'all', account: accountIds?.join(',') ?? 'all', sort, direction, revision: String(data?.revision ?? 0) });
  const queryKey = [...workspaceKey(), 'documents', serverParams.toString()];
  const serverQuery = useQuery<DocumentList>({ queryKey, placeholderData: (previous, query) => samePaginationScope(queryKey, query?.queryKey) ? previous : undefined, queryFn: ({ signal }) => api<DocumentList>(`/documents?${serverParams}`, { signal }), enabled: !isDemo && Boolean(data), retry: false });
  if (!data) return <DataBoundary><></></DataBoundary>;
  const scope = periodDocuments(data.documents, period).filter(item => config.accounts ? item.lines.some(line => config.accounts?.includes(line.accountId)) : !config.type || item.type === config.type);
  function openEntry(item: AccountingDocument) {
    if (serverQuery.isPlaceholderData) return;
    if (!isDemo && (item.voucher || item.policyId && ['receipt', 'payment', 'transfer', 'sale', 'purchase', 'closing'].includes(item.sourceType ?? ''))) openVoucher(`/vouchers/${item.id}`);
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
  const exportRows = (format: ExportFormat) => { if (!isDemo) { const path = `/documents/export?${serverParams}&format=${format}`; if (format === 'print') { window.open(`/api/v1${path}`, '_blank', 'noopener,noreferrer'); return; } return downloadApi(path, `chung-tu-${period}.${format}`); } downloadCsv(`chung-tu-mau-${period}.csv`, [
    ['DỮ LIỆU MẪU', `Kỳ ${period}`, `Phiên bản ${data.revision}`, 'Đơn vị VND'],
    ['Số chứng từ', 'Ngày', 'Loại', 'Diễn giải', 'Đối tượng', 'Tổng Nợ (VND)', 'Trạng thái'],
    ...filtered.map(item => [item.reference, item.date, documentTypes[item.type], item.description, data.partners.find(partner => partner.id === item.partnerId)?.name ?? '', `${item.lines.length ? formatMoney(totals(item.lines).debit) : '0'} VND`, documentStatuses[item.status]]),
  ]); };

  const actions = (openExcel?: (kind?: string) => void) => {
    const importItems = (kind?: string) => openExcel ? [{ type: 'divider' as const }, { key: 'excel', label: 'Nhập từ Excel', icon: <Icon name="upload" size={16} />, onClick: () => openExcel(kind) }] : [];
    const treasury = (kind: 'receipt' | 'payment') => () => openVoucher(`/vouchers/new?kind=${kind}&account=${config.accounts?.[0]}`);
    const trade = config.type === 'sale' || config.type === 'purchase';
    const create = () => !isDemo && trade ? openVoucher(`/vouchers/new?kind=${config.type}`) : newDraft(config.treasury ? 'receipt' : config.type);
    const createLabel = !isDemo && trade ? config.type === 'sale' ? 'Bán hàng' : 'Mua hàng' : 'Tạo chứng từ';
    return <>
      {openExcel && <ToolbarButton label="Nhập từ Excel" icon="upload" onClick={() => openExcel()} />}
      {!isDemo && bankPage && <BankAccountsButton />}
      {!isDemo && trade && <ToolbarButton onClick={() => navigate(`${config.type === 'sale' ? '/sales' : '/purchases'}/settlements`)} label="Công nợ theo chứng từ" icon="users" />}
      <ExportMenu onReload={refresh} onExport={exportRows} disabled={!filtered.length || serverQuery.isPlaceholderData} formats={isDemo ? ['csv'] : undefined} />
      {canWrite && <div className={styles.toolbarPrimaryActions}>{!isDemo && config.treasury ? <>
        <ToolbarActionMenu label="Thu tiền" icon="arrowDown" onClick={treasury('receipt')} menu={{ items: [
          { key: 'receipt', label: 'Thu tiền', onClick: treasury('receipt') },
          { key: 'receipt-invoice', label: 'Thu tiền theo hóa đơn', onClick: () => openVoucher(`/vouchers/new?kind=receipt&account=${config.accounts?.[0]}&settlement=1`) },
          ...importItems('receipt'),
        ] }} />
        <ToolbarActionMenu label="Chi tiền" icon="arrowUp" onClick={treasury('payment')} menu={{ items: [
          { key: 'payment', label: 'Chi tiền', onClick: treasury('payment') },
          { key: 'payment-invoice', label: 'Trả tiền theo hóa đơn', onClick: () => openVoucher(`/vouchers/new?kind=payment&account=${config.accounts?.[0]}&settlement=1`) },
          { key: 'transfer', label: 'Chuyển tiền nội bộ', onClick: () => openVoucher(`/vouchers/new?kind=transfer&account=${config.accounts?.[0]}`) },
          ...importItems('payment'),
        ] }} />
      </> : !isDemo ? <ToolbarActionMenu label={createLabel} icon="plus" onClick={create} menu={{ items: [
        { key: 'create', label: createLabel, onClick: create },
        ...(trade ? [{ key: 'service', label: config.type === 'sale' ? 'Bán dịch vụ' : 'Mua dịch vụ', onClick: () => openVoucher(`/vouchers/new?kind=${config.type}&service=1`) }] : []),
        ...(config.type === 'purchase' ? [{ key: 'asset', label: 'Mua tài sản / chi phí trả trước', onClick: () => navigate('/automation?kind=asset') }] : []),
        ...(apiScope().permissions === 'owner' && !config.accounts && !config.type ? [
          { key: 'closing', label: 'Kết chuyển lãi lỗ', onClick: () => openVoucher('/vouchers/new?kind=closing') },
          { key: 'periods', label: 'Quản lý / khóa kỳ', onClick: () => navigate('/settings') },
        ] : []),
        ...importItems(),
      ] }} /> : <ToolbarButton type="primary" label={createLabel} icon={config.treasury ? 'arrowDown' : 'plus'} onClick={create} />}</div>}
    </>;
  };
  const pageActions = !isDemo && canWrite ? <ImportDocuments>{actions}</ImportDocuments> : actions();
  return <DataBoundary>
    {isDemo ? <PageHeading title={config.title} description={config.description} /> : <h1 className={styles.visuallyHidden}>{params.get('source') === 'closing' ? 'Kết chuyển lãi lỗ' : config.title}</h1>}

    {!isDemo && config.treasury && <div className={styles.accountCards}>{([['Dư đầu kỳ', 'opening'], ['Thu trong kỳ', 'receipts'], ['Chi trong kỳ', 'payments'], ['Dư cuối kỳ', 'closing']] as const).map(([label, key]) => <div className={styles.accountCard} key={key}><div><span>{label} · TK {selectedBank?.number ?? config.accounts?.join(', ')}</span><strong>{serverQuery.data?.money ? formatMoney(serverQuery.data.money[key]) : '—'}<small>VND</small></strong></div></div>)}</div>}
    {config.accounts && (!config.treasury || isDemo) && <div className={styles.accountCards}>{config.accounts.map(id => <div key={id} className={styles.accountCard}><span className={styles.metricIcon}><Icon name={id === '156' ? 'box' : 'wallet'} /></span><div><span>{data.accounts.find(item => item.id === id)?.name} · {id}</span><strong>{formatMoney(data.server ? data.server.balances[id] : accountBalance(data.documents, [id], period))}<small>VND</small></strong></div><span className={styles.softLabel}>{isDemo ? 'Số dư cuối kỳ mẫu' : 'Số dư cuối kỳ'}</span></div>)}</div>}
    {config.accounts?.includes('156') && <div className={styles.inlineNotice}><Icon name="help" size={16} />{isDemo ? 'Sổ này phản ánh giá trị hạch toán mẫu. Số lượng, tính giá xuất kho và kiểm kê cần phân hệ kho cùng chính sách đã duyệt.' : 'Xem số lượng, lô và giá trị tồn ở Nghiệp vụ tự động → Hàng hóa & dịch vụ. Biến động kho được ghi cùng transaction với chứng từ.'}</div>}
    {serverQuery.error && <Alert type="error" title={serverQuery.error.message} action={<Button onClick={() => { refresh(); void serverQuery.refetch(); }}>Tải lại dữ liệu</Button>} />}
    <section className={`${styles.panel} ${!isDemo ? styles.documentList : ''}`}>
      {isDemo && <div className={styles.listTabs}><Tabs activeKey={status} onChange={value => updateFilter('status', value)} items={[
        { key: 'all', label: <>Tất cả <span className={styles.countBadge}>{isDemo ? scope.length : counts?.all ?? '—'}</span></> },
        { key: 'posted', label: <>Đã ghi sổ <span className={styles.countBadge}>{isDemo ? scope.filter(item => item.status === 'posted').length : counts?.posted ?? '—'}</span></> },
        { key: 'draft', label: <>Bản nháp <span className={styles.countBadge}>{isDemo ? scope.filter(item => item.status === 'draft').length : counts?.draft ?? '—'}</span></> },
      ]} /></div>}
      <div className={styles.tableToolbar}><DebouncedSearchInput scopeKey={`${period}-${location.pathname}-${searchScope}`} prefix={<Icon name="search" size={17} />} placeholder="Tìm số chứng từ, diễn giải, đối tượng…" aria-label="Tìm trong danh sách chứng từ" value={search} onSearch={value => updateFilter('q', value)} allowClear className={styles.filterSearch} /><div>{!isDemo && bankPage && <Select aria-label="Lọc tài khoản ngân hàng" loading={banks.isPending} value={params.get('bank') ?? 'all'} onChange={value => updateFilter('bank', value)} style={{ minWidth: 220 }} options={[{ value: 'all', label: 'Tất cả tài khoản ngân hàng' }, ...(banks.data?.accounts.map(item => ({ value: item.id, label: `${item.number} · ${item.bankName}` })) ?? [])]} />}{!isDemo && <Select aria-label="Lọc trạng thái chứng từ" value={status} onChange={value => updateFilter('status', value)} style={{ minWidth: 155 }} options={[{ value: 'all', label: `Tất cả (${counts?.all ?? '…'})` }, { value: 'posted', label: `Đã ghi sổ (${counts?.posted ?? '…'})` }, { value: 'draft', label: `Bản nháp (${counts?.draft ?? '…'})` }]} />}{!config.type && <Select aria-label="Lọc loại chứng từ" value={type} onChange={value => updateFilter('type', value)} style={{ minWidth: 170 }} options={[{ value: 'all', label: 'Tất cả loại chứng từ' }, ...Object.entries(documentTypes).map(([value, label]) => ({ value, label }))]} />}{compact && <Select aria-label="Sắp xếp chứng từ" value={`${sort}:${direction}`} style={{ minWidth: 160 }} onChange={value => { const [field, order] = value.split(':'); updateFilters({ sort: field, direction: order === 'asc' ? 'asc' : 'desc' }); }} options={[{ value: 'date:desc', label: 'Mới nhất' }, { value: 'date:asc', label: 'Cũ nhất' }, { value: 'amount:desc', label: 'Số tiền giảm dần' }, { value: 'amount:asc', label: 'Số tiền tăng dần' }]} />}{isDemo && <Tag className={styles.demoPill}>Dữ liệu mẫu</Tag>}<div className={styles.toolbarActions} role="group" aria-label="Thao tác chứng từ">{pageActions}</div></div></div>
      {serverQuery.isPlaceholderData && <div role="status" className={styles.searchHint}>Đang tải trang {page}… Nội dung đang cập nhật.</div>}
      <Table<AccountingDocument> tableLayout="fixed" className={compact ? styles.compactDocuments : undefined} showHeader={!compact} loading={{ spinning: !isDemo && serverQuery.isFetching, delay: 120 }} onChange={(_, __, sorter, extra) => { if (extra.action !== 'sort') return; const current = Array.isArray(sorter) ? sorter[0] : sorter; updateFilters({ sort: current.order && current.columnKey === 'amount' ? 'amount' : 'date', direction: current.order === 'ascend' ? 'asc' : 'desc' }); }} rowKey="id" dataSource={rows} onRow={item => ({ onClick: () => { if (!serverQuery.isPlaceholderData) setSelectedId(item.id); } })} rowClassName={item => item.id === selected?.id && !compact ? styles.selectedDocument : ''} scroll={compact ? undefined : { x: 950, y: isDemo ? undefined : 'max(200px, calc(100vh - 520px))' }} pagination={{ current: page, total: count, onChange: (next, size) => { updateFilters({ page: String(size === pageSize ? next : 1), pageSize: String(size) }, false); }, pageSize, showSizeChanger: !isDemo, pageSizeOptions: [10, 20, 50, 100], hideOnSinglePage: true, showTotal: total => `${total} chứng từ` }} locale={{ emptyText: <EmptyData action={<Button onClick={() => { setParams({}); }}>Xóa bộ lọc</Button>} /> }} columns={compact ? [{ key: 'document', render: (_, item) => <article className={styles.documentCard}>
        <div><button className={`${styles.tableLink} ${styles.documentReference}`} disabled={serverQuery.isPlaceholderData} title={item.reference} onClick={() => openEntry(item)}>{item.reference}</button><StatusTag status={item.status} /></div>
        <p>{item.description}</p>
        <span className={styles.cellSub}>{data.partners.find(partner => partner.id === item.partnerId)?.name ?? 'Không có đối tượng chung'}</span>
        <div className={styles.documentCardMeta}><span>{formatDate(item.date)} · {documentTypes[item.type]}</span><span>Tổng Nợ (VND)<strong>{formatMoney(totals(item.lines).debit)}</strong></span></div>
      </article> }] : [
        { title: 'SỐ CHỨNG TỪ', dataIndex: 'reference', width: 155, ellipsis: { showTitle: false }, render: (value: string, item) => <button className={`${styles.tableLink} ${styles.documentReference}`} disabled={serverQuery.isPlaceholderData} title={value} onClick={() => openEntry(item)}>{value}</button> },
        { title: 'NGÀY', dataIndex: 'date', width: 105, sortOrder: sort === 'date' ? (direction === 'asc' ? 'ascend' : 'descend') : null, sorter: isDemo ? (a, b) => a.date.localeCompare(b.date) : true, render: formatDate },
        { title: 'DIỄN GIẢI / ĐỐI TƯỢNG', dataIndex: 'description', render: (value: string, item) => <><span>{value}</span><small className={styles.cellSub}>{data.partners.find(partner => partner.id === item.partnerId)?.name ?? 'Không có đối tượng chung'}</small></> },
        { title: 'LOẠI', dataIndex: 'type', width: 120, render: (value: DocumentType, row: AccountingDocument) => row.sourceType === 'transfer' ? 'Chuyển tiền nội bộ' : row.sourceType === 'closing' ? 'Kết chuyển lãi lỗ' : documentTypes[value] },
        { title: 'TỔNG NỢ (VND)', key: 'amount', width: 220, align: 'right', sortOrder: sort === 'amount' ? (direction === 'asc' ? 'ascend' : 'descend') : null, sorter: isDemo ? (a, b) => { const diff = totals(a.lines).debit - totals(b.lines).debit; return diff < 0n ? -1 : diff > 0n ? 1 : 0; } : true, render: (_, item) => <strong className={styles.tableAmount}>{formatMoney(totals(item.lines).debit)}</strong> },
        { title: 'TRẠNG THÁI', dataIndex: 'status', width: 125, render: value => <StatusTag status={value} /> },
        { width: 46, render: (_, item) => <Button type="text" icon={<Icon name="chevron" size={15} />} disabled={serverQuery.isPlaceholderData} aria-label={`Xem ${item.reference}`} onClick={() => openEntry(item)} /> },
      ]} />
      <div className={styles.tableTotal}><span>{serverQuery.isPending && !isDemo ? 'Đang tải…' : count} chứng từ trong toàn bộ tập lọc <small>· Tổng Nợ, gồm cả bản nháp</small></span><strong>{!isDemo && !serverQuery.data ? '—' : formatMoney(filteredTotal)}<small>VND</small></strong></div>
    </section>
    {!isDemo && selected && <section className={`${styles.panel} ${styles.documentLines}`} aria-label="Chi tiết chứng từ đang chọn"><div className={styles.panelHeader}><h2>Chi tiết · {selected.reference}</h2><Button type="link" disabled={serverQuery.isPlaceholderData} onClick={() => openEntry(selected)}>Mở chứng từ</Button></div><Table rowKey={(_, index) => String(index)} size="small" dataSource={selected.lines} pagination={false} scroll={{ x: 850, y: 280 }} columns={[{ title: '#', width: 42, render: (_, __, index) => index + 1 }, { title: 'Diễn giải', dataIndex: 'description' }, { title: 'Tài khoản', dataIndex: 'accountId', width: 130 }, { title: 'Đối tượng', dataIndex: 'partnerId', width: 230, render: (id: string | undefined) => data.partners.find(item => item.id === id)?.name ?? '—' }, { title: 'Nợ (VND)', dataIndex: 'debit', align: 'right', width: 170, render: formatMoney }, { title: 'Có (VND)', dataIndex: 'credit', align: 'right', width: 170, render: formatMoney }]} summary={() => <Table.Summary.Row><Table.Summary.Cell index={0} colSpan={4}>Tổng</Table.Summary.Cell><Table.Summary.Cell index={4} align="right">{formatMoney(totals(selected.lines).debit)}</Table.Summary.Cell><Table.Summary.Cell index={5} align="right">{formatMoney(totals(selected.lines).credit)}</Table.Summary.Cell></Table.Summary.Row>} /></section>}
  </DataBoundary>;
}
