import { Alert, App, Button, Input, Select, Table, Tabs } from 'antd';
import { Link, useLocation, useSearchParams } from 'react-router-dom';
import { useWorkspace } from '../../app/WorkspaceProvider';
import { partnerBalances, trialBalance } from '../../shared/accounting/ledger';
import { formatDate, formatMoney, normalizeSearch } from '../../shared/accounting/money';
import { downloadCsv } from '../../shared/accounting/csv';
import { DataBoundary, PageHeading, ScopeNote } from '../../shared/ui/Page';
import { Icon } from '../../shared/ui/Icon';
import { searchOptions } from '../../shared/ui/search';
import styles from '../../app/App.module.css';
import { api, downloadApi, isDemo } from '../../shared/api/client';
import { useQuery } from '@tanstack/react-query';
import { workspaceKey } from '../../shared/data/workspace';
import { exactRow } from '../../shared/data/reports';
import { FinancialReports } from './FinancialReports';
import { OpeningBalances } from './OpeningBalances';
import { accountingModule } from '../../app/module-navigation';
import type { ServerReport } from '../../shared/data/reports';

interface ReportRow { key: string; code: string; name: string; debit: bigint; credit: bigint; openingDebit?: bigint; openingCredit?: bigint; closingDebit?: bigint; closingCredit?: bigint; date?: string; documentId?: string; description?: string; account?: string }

export function Component() {
 const { pathname } = useLocation();
 const [params] = useSearchParams();
 if (!isDemo && pathname !== "/receivables" && ["financial", "income"].includes(params.get("tab") ?? "")) return <FinancialReports />;
 return <BookReports />;
}

function BookReports() {
  const { data, period, openDocument, refresh } = useWorkspace();
  const { message } = App.useApp();
  const { pathname } = useLocation();
  const [params, setParams] = useSearchParams();
  const module = accountingModule(pathname);
  const isDebt = pathname === '/receivables' || params.get('tab') === 'debt';
  const tab = isDebt ? 'debt' : params.get('tab') === 'trial' ? 'trial' : params.get('tab') === 'ledger' ? 'ledger' : 'journal';
  const account = params.get('account') ?? 'all';
  const search = params.get('q') ?? '';
  const requestedPage = Number(params.get('page'));
  const page = Number.isSafeInteger(requestedPage) && requestedPage > 0 ? requestedPage : 1;
  const setFilter = (key: string, value: string) => setParams(previous => { const next = new URLSearchParams(previous); if (value) next.set(key, value); else next.delete(key); if (key !== 'page') next.delete('page'); return next; }, { replace: true });
  const setAccount = (value: string) => setFilter('account', value);
  const setSearch = (value: string) => setFilter('q', value);
  const setPage = (value: number) => setFilter('page', String(value));
  const debtType = params.get('debt') === 'payable' ? 'payable' : 'receivable';
  const setDebtType = (value: string) => setFilter('debt', value);
  const serverParams = new URLSearchParams({ period, kind: tab, account: isDebt ? debtType : account, q: search, page: String(page), pageSize: '10', revision: String(data?.revision ?? 0) });
  const server = useQuery({ queryKey: [...workspaceKey(), 'report', serverParams.toString()], queryFn: ({ signal }) => api<ServerReport>(`/reports?${serverParams}`, { signal }), enabled: !isDemo && Boolean(data), retry: false });
  if (!data) return <DataBoundary><></></DataBoundary>;
  const posted = data.documents.filter(item => item.status === 'posted' && item.date.slice(0, 7) <= period);
  const current = posted.filter(item => item.date.startsWith(`${period}-`));
  const rows: ReportRow[] = tab === 'trial' ? trialBalance(data, period) : tab === 'debt' ? partnerBalances(data, period, debtType).map(item => ({ key: item.id, code: item.code, name: item.name, debit: item.balance > 0n ? item.balance : 0n, credit: item.balance < 0n ? -item.balance : 0n })).filter(item => item.debit || item.credit) : current.sort((a, b) => a.date.localeCompare(b.date) || a.id.localeCompare(b.id)).flatMap(doc => doc.lines.map((line, index) => ({ key: `${doc.id}-${index}`, code: doc.reference, name: data.accounts.find(item => item.id === line.accountId)?.name ?? 'Tài khoản không tồn tại', account: line.accountId, date: doc.date, documentId: doc.id, description: line.description || doc.description, debit: BigInt(line.debit), credit: BigInt(line.credit) })));
  const filtered = !isDemo ? server.data?.rows.map(exactRow) ?? [] : rows.filter(row => (tab !== 'ledger' || account === 'all' || row.account === account) && normalizeSearch(`${row.code} ${row.name} ${row.description ?? ''}`).includes(normalizeSearch(search)));
  const debit = !isDemo ? BigInt(server.data?.debit ?? '0') : filtered.reduce((sum, row) => sum + row.debit, 0n);
  const credit = !isDemo ? BigInt(server.data?.credit ?? '0') : filtered.reduce((sum, row) => sum + row.credit, 0n);
  const sumField = (field: keyof Pick<ReportRow, 'openingDebit' | 'openingCredit' | 'closingDebit' | 'closingCredit'>) => !isDemo ? BigInt(server.data?.[field] ?? '0') : filtered.reduce((sum, row) => sum + (row[field] ?? 0n), 0n);
  const amountColumn = (title: string, key: keyof ReportRow) => ({ title, dataIndex: key, align: 'right' as const, width: tab === 'trial' ? 185 : 145, render: (value: bigint) => <span className={styles.tableAmount}>{formatMoney(value)}</span> });
  const titles = { journal: 'Nhật ký chung', ledger: 'Sổ cái', trial: 'Cân đối phát sinh', debt: 'Chi tiết công nợ' };
  const ledgerBalance = !isDemo ? server.data?.accountBalance ? exactRow(server.data.accountBalance) : null : tab === 'ledger' && account !== 'all' ? trialBalance(data, period).find(item => item.key === account) : null;
  const exportReport = () => { if (!isDemo) { void downloadApi(`/reports/export?${serverParams}`, `so-${tab}-${period}.csv`).catch(error => void message.error((error as Error).message)); return; } downloadCsv(`so-mau-${tab}-${period}.csv`, [
    ['DỮ LIỆU MẪU', titles[tab], `Kỳ ${period}`, `Phiên bản ${data.revision}`, `Tài khoản: ${tab === 'ledger' ? account : 'Tất cả'}`],
    ...(tab === 'trial' ? [['Mã', 'Tên', 'Dư đầu Nợ', 'Dư đầu Có', 'Phát sinh Nợ', 'Phát sinh Có', 'Dư cuối Nợ', 'Dư cuối Có'], ...filtered.map(row => [row.code, row.name, ...[row.openingDebit, row.openingCredit, row.debit, row.credit, row.closingDebit, row.closingCredit].map(value => `${formatMoney(value ?? 0n)} VND`)])] : [['Mã chứng từ / đối tượng', 'Ngày', 'Diễn giải / tên', 'Tài khoản', 'Nợ', 'Có'], ...filtered.map(row => [row.code, row.date ?? '', row.description ?? row.name, row.account ?? '', `${formatMoney(row.debit)} VND`, `${formatMoney(row.credit)} VND`])]),
  ]); };

  return <DataBoundary>
    <PageHeading title={isDebt ? 'Công nợ' : 'Sổ sách & báo cáo'} description={isDebt ? 'Theo dõi số dư hai bên theo từng khách hàng và nhà cung cấp.' : 'Từ số tổng hợp đến từng dòng chứng từ, luôn có thể kiểm chứng.'} actions={<>{!isDemo && tab === 'trial' && <OpeningBalances />}{!isDemo && isDebt && <Link to={module && 'tradeKind' in module ? `${module.path}/settlements` : '/settlements'}><Button>Công nợ theo chứng từ</Button></Link>}{!isDemo && <Button icon={<Icon name="print" size={16} />} onClick={() => { const preview = window.open(`/api/v1/reports/print?${serverParams}`, '_blank'); if (preview) preview.addEventListener('load', () => preview.print(), { once: true }); }}>In báo cáo</Button>}<Button icon={<Icon name="download" size={16} />} onClick={exportReport} disabled={!filtered.length}>Xuất CSV</Button></>} />
    <div className={styles.reportIntro}><span className={styles.metricIcon}><Icon name={isDebt ? 'users' : 'book'} /></span><div><strong>{titles[tab]} · Kỳ {period.split('-').reverse().join('/')}</strong><p>{isDemo ? 'Dữ liệu mẫu đã ghi sổ' : 'Chứng từ đã ghi sổ'} · VND · Phiên bản {data.revision} · {isDebt ? 'Giữ riêng số dư Nợ và Có của từng đối tượng' : 'Bản nháp không tham gia số liệu'}</p></div><span className={styles.demoPill}>Bản xem trước</span></div>
    {server.error && <Alert type="error" title={server.error.message} action={<Button onClick={refresh}>Tải lại dữ liệu</Button>} />}
    <section className={styles.panel}>
      {(!isDebt || module) && <div className={styles.listTabs}><Tabs activeKey={tab} onChange={value => { setParams({ tab: value, ...(value === 'debt' ? { debt: module && 'tradeKind' in module && module.tradeKind === 'purchase' ? 'payable' : 'receivable' } : {}) }); }} items={[{ key: 'journal', label: 'Nhật ký chung' }, { key: 'ledger', label: 'Sổ cái' }, { key: 'trial', label: 'Cân đối phát sinh' }, ...(module && 'tradeKind' in module ? [{ key: 'debt', label: 'Công nợ theo đối tượng' }] : []), ...(!isDemo ? [{ key: 'financial', label: 'Báo cáo tài chính' }] : [])]} /></div>}
      <div className={styles.tableToolbar}><Input prefix={<Icon name="search" size={17} />} value={search} onChange={event => { setSearch(event.target.value); }} aria-label="Tìm trong báo cáo" placeholder="Tìm mã, tên hoặc diễn giải…" allowClear className={styles.filterSearch} />{tab === 'ledger' && <Select aria-label="Chọn tài khoản sổ cái" showSearch={searchOptions} value={account} onChange={value => { setAccount(value); }} style={{ minWidth: 240 }} options={[{ value: 'all', label: 'Tất cả tài khoản' }, ...data.accounts.map(item => ({ value: item.id, label: `${item.code} · ${item.name}` }))]} />}{isDebt && <Select aria-label="Loại công nợ" value={debtType} onChange={value => { setDebtType(value); }} style={{ minWidth: 210 }} options={[{ value: 'receivable', label: 'Phải thu khách hàng · 131' }, { value: 'payable', label: 'Phải trả người bán · 331' }]} />}</div>
      {ledgerBalance && <div className={styles.ledgerBalances}>{[['Dư đầu Nợ', ledgerBalance.openingDebit], ['Dư đầu Có', ledgerBalance.openingCredit], ['Dư cuối Nợ', ledgerBalance.closingDebit], ['Dư cuối Có', ledgerBalance.closingCredit]].map(([label, value]) => <span key={String(label)}>{label}<strong>{formatMoney(value as bigint)}<small>VND</small></strong></span>)}</div>}
      <Table<ReportRow> loading={!isDemo && server.isPending} key={`${tab}-${period}-${account}`} rowKey="key" dataSource={filtered} scroll={{ x: tab === 'trial' ? 1300 : 850 }} pagination={{ current: page, onChange: setPage, total: isDemo ? filtered.length : server.data?.total ?? 0, pageSize: 10, hideOnSinglePage: true, showSizeChanger: false, showTotal: total => `${total} dòng` }} columns={tab === 'trial' ? [
        { title: 'TÀI KHOẢN', dataIndex: 'code', width: 110, fixed: 'left' }, { title: 'TÊN TÀI KHOẢN', dataIndex: 'name', width: 220 },
        { title: 'DƯ ĐẦU KỲ', children: [amountColumn('Nợ', 'openingDebit'), amountColumn('Có', 'openingCredit')] },
        { title: 'PHÁT SINH TRONG KỲ', children: [amountColumn('Nợ', 'debit'), amountColumn('Có', 'credit')] },
        { title: 'DƯ CUỐI KỲ', children: [amountColumn('Nợ', 'closingDebit'), amountColumn('Có', 'closingCredit')] },
      ] : isDebt ? [{ title: 'MÃ ĐỐI TƯỢNG', dataIndex: 'code', width: 150 }, { title: 'TÊN ĐỐI TƯỢNG', dataIndex: 'name' }, amountColumn('DƯ NỢ (VND)', 'debit'), amountColumn('DƯ CÓ (VND)', 'credit')] : [
        { title: 'NGÀY', dataIndex: 'date', width: 105, render: formatDate },
        { title: 'CHỨNG TỪ', dataIndex: 'code', width: 135, render: (value: string, row) => <button className={styles.tableLink} onClick={() => openDocument(row.documentId ?? null)}>{value}</button> },
        { title: 'DIỄN GIẢI', dataIndex: 'description', render: (value: string, row) => <>{value}<small className={styles.cellSub}>{row.name}</small></> },
        { title: 'TK', dataIndex: 'account', width: 65 }, amountColumn('NỢ (VND)', 'debit'), amountColumn('CÓ (VND)', 'credit'),
      ]} summary={() => !isDemo && !server.data ? null : tab === 'trial' ? <Table.Summary.Row><Table.Summary.Cell index={0} colSpan={2}><strong>Tổng toàn bộ tập lọc</strong></Table.Summary.Cell>{[sumField('openingDebit'), sumField('openingCredit'), debit, credit, sumField('closingDebit'), sumField('closingCredit')].map((value, index) => <Table.Summary.Cell index={index + 2} align="right" key={index}><strong className={styles.tableAmount}>{formatMoney(value)}</strong></Table.Summary.Cell>)}</Table.Summary.Row> : <Table.Summary.Row><Table.Summary.Cell index={0} colSpan={isDebt ? 2 : 4}><strong>Tổng toàn bộ tập lọc</strong></Table.Summary.Cell><Table.Summary.Cell index={isDebt ? 2 : 4} align="right"><strong>{formatMoney(debit)}</strong></Table.Summary.Cell><Table.Summary.Cell index={isDebt ? 3 : 5} align="right"><strong>{formatMoney(credit)}</strong></Table.Summary.Cell></Table.Summary.Row>} />
    </section>
    <div className={styles.inlineNotice}><Icon name="shield" size={16} />{isDebt ? 'Dư Có khách hàng và dư Nợ nhà cung cấp được giữ riêng, không bù trừ giữa các đối tượng.' : 'Cân đối Nợ/Có chỉ là một bước kiểm tra. Tài khoản, đối tượng và kỳ cần được đối chiếu với chứng từ nguồn.'}</div><ScopeNote />
  </DataBoundary>;
}
