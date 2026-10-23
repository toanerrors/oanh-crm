import { Alert, Button, Empty, Progress, Table, Tag, Tooltip } from 'antd';
import { Link, useNavigate } from 'react-router-dom';
import { useWorkspace } from '../../app/WorkspaceProvider';
import { formatDate, formatMoney } from '../../shared/accounting/money';
import { totals } from '../../shared/accounting/ledger';
import { documentTypes } from '../../shared/data/types';
import type { AccountingDocument, DocumentType } from '../../shared/data/types';
import { Icon } from '../../shared/ui/Icon';
import type { IconName } from '../../shared/ui/Icon';
import { ImportExcelButton } from '../imports/ExcelImport';
import { DataBoundary, PageHeading, StatusTag } from '../../shared/ui/Page';
import { ToolbarActionMenu } from '../../shared/ui/ToolbarActionMenu';
import { ToolbarButton } from '../../shared/ui/ToolbarButton';
import { apiScope, isDemo } from '../../shared/api/client';
import { chartBar, chartDomain, dashboardModel } from './dashboard-model';
import styles from './Dashboard.module.css';

const monthLabel = (period: string) => `${period.slice(5)}/${period.slice(0, 4)}`;
export function Component() {
  const { data, period, setPeriod, newDraft, openDocument, refresh, isFetching } = useWorkspace();
  const navigate = useNavigate();
  if (!data) return <DataBoundary><></></DataBoundary>;
  const startPeriod = isDemo ? undefined : data.conversionDate?.slice(0, 7);
  if (startPeriod && period < startPeriod) return <DataBoundary><PageHeading title="Tổng quan" description="Phạm vi dữ liệu kế toán" /><Alert type="info" showIcon title="Kỳ đã chọn nằm trước mốc dữ liệu" description={`Dữ liệu kế toán bắt đầu từ ${formatDate(data.conversionDate!)}. Chọn kỳ từ mốc này để xem số liệu trên sổ.`} action={<Button onClick={() => setPeriod(startPeriod)}>Về kỳ bắt đầu dữ liệu</Button>} /></DataBoundary>;
  const m = dashboardModel(data, period);
  const canWrite = isDemo || apiScope().permissions !== 'viewer';
  const owner = !isDemo && apiScope().permissions === 'owner';
  const closed = data.periods?.find(p => p.period === period)?.closed;
  const periodStatus = closed === undefined && !isDemo ? <Tag>Chưa mở kỳ</Tag> : <Tag color={closed ? 'default' : 'green'}>{closed ? 'Kỳ đã khóa' : 'Kỳ đang mở'}</Tag>;
  const quick: { label: string; icon: IconName; type: DocumentType; to: string }[] = [
    { label: 'Phiếu thu', icon: 'arrowDown', type: 'receipt', to: '/vouchers/new?kind=receipt&account=111' },
    { label: 'Phiếu chi', icon: 'arrowUp', type: 'payment', to: '/vouchers/new?kind=payment&account=111' },
    { label: 'Bán hàng', icon: 'cart', type: 'sale', to: '/vouchers/new?kind=sale' },
    { label: 'Mua hàng', icon: 'bag', type: 'purchase', to: '/vouchers/new?kind=purchase' },
    { label: 'Chứng từ khác', icon: 'book', type: 'journal', to: '/documents' },
  ];
  const create = (item: typeof quick[number]) => { if (isDemo || item.type === 'journal') newDraft(item.type); else navigate(item.to); };
  const metrics: { label: string; value: bigint | null; icon: IconName; note: string; to: string; color: string }[] = [
    { label: 'Doanh thu trong kỳ', value: m.revenue, icon: 'chart', note: 'Phát sinh doanh thu đã ghi sổ', to: '/sales', color: 'green' },
    { label: 'Chi phí trong kỳ', value: m.expense, icon: 'wallet', note: 'Phát sinh chi phí đã ghi sổ', to: '/reports?tab=trial', color: 'orange' },
    { label: 'Tiền & tiền gửi', value: m.money, icon: 'building', note: 'Số dư nhóm tài khoản tiền đến cuối kỳ', to: '/bank', color: 'blue' },
    { label: 'Phải thu khách hàng', value: m.receivable, icon: 'users', note: 'Tổng dư Nợ theo đối tượng', to: '/receivables?debt=receivable', color: 'blue' },
    { label: 'Phải trả người bán', value: m.payable, icon: 'bag', note: 'Tổng dư Có theo đối tượng', to: '/receivables?debt=payable', color: 'orange' },
    { label: 'Hàng tồn kho trên sổ', value: m.inventory, icon: 'box', note: 'Theo nhóm tài khoản hàng tồn kho', to: '/inventory', color: 'green' },
  ];
  const domain = chartDomain(m.months.flatMap(month => [month.revenue, month.expense]));
  const ticks = domain.negative && domain.positive ? [{ value: domain.positive, y: 8 }, { value: 0n, y: domain.zero }, { value: -domain.negative, y: 128 }] : domain.negative ? [{ value: 0n, y: 8 }, { value: -domain.negative, y: 128 }] : [{ value: domain.positive, y: 8 }, { value: 0n, y: 128 }];
  return <DataBoundary>
    <PageHeading title="Tổng quan" description="Tình hình tài chính, chứng từ và công việc trong kỳ kế toán." />
    <div className={styles.header}>
      <div><h2>Toàn cảnh tài chính</h2><p><span className={styles.scopeDot} />Kỳ {monthLabel(period)}<span>·</span>{isDemo ? 'Dữ liệu mẫu' : 'Số liệu đã ghi sổ'}<span>·</span>VND{isFetching && <span role="status">Đang cập nhật…</span>}</p></div>
      <div className={styles.actions} role="group" aria-label="Công cụ tổng quan"><ToolbarButton label="Tải lại tổng quan" icon="refresh" loading={isFetching} onClick={refresh} />{canWrite && <><ImportExcelButton /><ToolbarActionMenu label="Tạo chứng từ" icon="plus" onClick={() => newDraft()} menu={{ items: quick.map(item => ({ key: item.type, label: item.label, icon: <Icon name={item.icon} size={16} />, onClick: () => create(item) })) }} /></>}</div>
    </div>
    <section className={styles.metrics} aria-label="Chỉ số tài chính">{metrics.map(metric => <Link key={metric.label} to={isDemo && metric.to === '/bank' ? '/cash' : metric.to} className={styles.metric} aria-label={`${metric.label}: ${metric.value === null ? 'chưa có dữ liệu' : `${formatMoney(metric.value)} đồng`}`}>
      <div className={styles.metricLabel}><span>{metric.label}</span><span className={`${styles.metricIcon} ${styles[metric.color]}`}><Icon name={metric.icon} size={17} /></span></div>
      <strong className={`${styles.amount} ${metric.value !== null && metric.value < 0n ? styles.negative : ''}`}>{metric.value === null ? 'Chưa có dữ liệu' : formatMoney(metric.value)}<small>{metric.value === null ? '' : 'đ'}</small></strong>
      <span className={styles.metricNote}>{metric.value === null ? 'Tải lại để cập nhật chỉ số' : metric.note}<Icon name="upRight" size={13} /></span>
    </Link>)}</section>
    {canWrite && <nav className={styles.shortcuts} aria-label="Thao tác nhanh tổng quan"><span>Tạo nhanh</span>{quick.map(item => <Button key={item.type} type="text" icon={<Icon name={item.icon} size={15} />} onClick={() => create(item)}>{item.label}</Button>)}</nav>}
    <div className={styles.middle}>
      <section className={styles.panel} aria-labelledby="business-chart-title">
        <div className={styles.panelHeader}><div><h2 id="business-chart-title">Doanh thu & chi phí</h2><p>6 tháng đến {monthLabel(period)}{data.conversionDate && !isDemo && ` · Dữ liệu từ ${formatDate(data.conversionDate)}`}</p></div><Link to="/reports?tab=trial" className={styles.textLink}>Chi tiết <Icon name="arrow" size={14} /></Link></div>
        <div className={styles.chartSummary}><div><Tooltip title="Doanh thu trừ chi phí của các nhóm tài khoản trong kỳ, không thay cho báo cáo kết quả kinh doanh."><span>Chênh lệch trong kỳ</span></Tooltip><strong className={m.difference < 0n ? styles.negative : styles.positive}>{formatMoney(m.difference)}<small>đ</small></strong></div><div className={styles.legend}><span><i />Doanh thu</span><span><i />Chi phí</span></div></div>
        {domain.empty ? <div className={styles.chartEmpty}><Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="Chưa có doanh thu hoặc chi phí trong 6 tháng này" /></div> : <div className={styles.chart}>
          <div className={styles.axis} aria-hidden="true">{ticks.map((tick, i) => <span key={i} style={{ top: `${tick.y / 136 * 100}%` }}>{formatMoney(tick.value)}</span>)}</div>
          <svg viewBox="0 0 600 136" preserveAspectRatio="none" role="img" aria-label="Biểu đồ doanh thu và chi phí; số tiền chính xác có ở từng nút tháng bên dưới.">
            {ticks.map((tick, i) => <line key={i} x1="0" x2="600" y1={tick.y} y2={tick.y} stroke="#e8ede9" strokeDasharray="3 4" />)}
            <line x1="0" x2="600" y1={domain.zero} y2={domain.zero} stroke="#c7d4cd" />
            {m.months.map((month, i) => <g key={month.key}>{([month.revenue, month.expense]).map((value, j) => { const bar = chartBar(value, domain); return <rect key={j} x={i * 100 + 24 + j * 25} y={bar.y} width="20" height={bar.height} rx="2" fill={j ? '#dfaa68' : '#3f9673'}><title>{monthLabel(month.key)} · {j ? 'Chi phí' : 'Doanh thu'}: {formatMoney(value)} đ</title></rect>; })}</g>)}
          </svg>
        </div>}
        <div className={`${styles.months} ${domain.empty ? styles.emptyMonths : ''}`}>{m.months.map(month => <Tooltip key={month.key} title={startPeriod && month.key < startPeriod ? 'Ngoài phạm vi dữ liệu' : `Doanh thu ${formatMoney(month.revenue)} đ · Chi phí ${formatMoney(month.expense)} đ`} trigger={['hover', 'focus']}><button className={month.key === period ? styles.selectedMonth : ''} aria-pressed={month.key === period} disabled={!!startPeriod && month.key < startPeriod} aria-label={startPeriod && month.key < startPeriod ? `${monthLabel(month.key)}: ngoài phạm vi dữ liệu` : `${monthLabel(month.key)}: doanh thu ${formatMoney(month.revenue)} đồng, chi phí ${formatMoney(month.expense)} đồng`} onClick={() => setPeriod(month.key)}>{monthLabel(month.key)}</button></Tooltip>)}</div>
      </section>
      <section className={styles.panel} aria-labelledby="dashboard-work-title"><div className={styles.panelHeader}><div><h2 id="dashboard-work-title">Việc cần xử lý</h2><p>Trên toàn bộ kỳ đã chọn</p></div>{owner ? <Tooltip title="Quản lý kỳ kế toán"><Link to="/settings?tab=periods" aria-label="Quản lý kỳ kế toán">{periodStatus}</Link></Tooltip> : periodStatus}</div>
        <Link className={styles.task} to="/documents?status=draft"><span className={`${styles.taskIcon} ${styles.orange}`}><Icon name="file" size={18} /></span><span><strong>{m.draftCount} chứng từ nháp</strong><small>{m.draftCount ? 'Tiếp tục kiểm tra và hoàn thiện' : 'Chưa có chứng từ chờ ghi sổ'}</small></span><Icon name="chevron" size={14} /></Link>
        <Link className={styles.task} to="/documents?status=draft"><span className={`${styles.taskIcon} ${m.unbalanced === 0 ? styles.green : styles.orange}`}><Icon name={m.unbalanced === 0 ? 'check' : 'book'} size={18} /></span><span><strong>{m.unbalanced === null ? 'Kiểm tra cân đối nháp' : m.unbalanced ? `${m.unbalanced} nháp lệch Nợ/Có` : 'Không có nháp lệch Nợ/Có'}</strong><small>Kiểm tra dòng hạch toán trước khi ghi sổ</small></span><Icon name="chevron" size={14} /></Link>
        {owner && !data.conversionDate && <Link className={styles.task} to="/reports?tab=trial"><span className={`${styles.taskIcon} ${styles.blue}`}><Icon name="wallet" size={18} /></span><span><strong>Thiết lập số dư đầu kỳ</strong><small>Nhập mốc chuyển đổi và số dư ban đầu</small></span><Icon name="chevron" size={14} /></Link>}
        <div className={styles.progress}><div><span>Đã ghi sổ</span><strong>{m.posted} / {m.count} chứng từ</strong></div><Progress percent={m.count ? Math.round(m.posted * 100 / m.count) : 0} showInfo={false} strokeColor="#3f9673" railColor="#edf1ee" size="small" /><div className={styles.reportLinks}><Link to="/reports?tab=trial">Cân đối phát sinh</Link>{!isDemo && <Link to="/reports?tab=financial">Báo cáo tài chính</Link>}</div></div>
      </section>
    </div>
    <div className={styles.bottom}>
      <section className={styles.panel} aria-labelledby="recent-documents-title"><div className={styles.panelHeader}><div><h2 id="recent-documents-title">Chứng từ gần đây <span className={styles.count}>{m.count}</span></h2><p>Theo ngày kế toán · Có cả bản nháp</p></div><Link to="/documents" className={styles.textLink}>Xem tất cả <Icon name="arrow" size={14} /></Link></div>
        <Table<AccountingDocument> rowKey="id" tableLayout="fixed" dataSource={m.documents.slice(0, 4)} pagination={false} size="small" scroll={{ x: 650 }} locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="Chưa có chứng từ trong kỳ">{canWrite && <Button onClick={() => newDraft()}>Tạo chứng từ đầu tiên</Button>}</Empty> }} columns={[
          { title: 'SỐ CHỨNG TỪ', dataIndex: 'reference', width: 165, ellipsis: true, render: (value: string, record) => <button title={value} className={styles.documentLink} onClick={() => openDocument(record.id)}>{value}</button> },
          { title: 'DIỄN GIẢI', dataIndex: 'description', ellipsis: true, render: (value: string, record) => <><span title={value}>{value}</span><small className={styles.cellSub}>{formatDate(record.date)} · {documentTypes[record.type]}</small></> },
          { title: 'SỐ TIỀN (VND)', align: 'right', width: 180, render: (_, record) => <strong className={styles.tableAmount}>{formatMoney(totals(record.lines).debit)}</strong> },
          { title: 'TRẠNG THÁI', dataIndex: 'status', width: 110, render: value => <StatusTag status={value} /> },
        ]} />
      </section>
      <section className={`${styles.panel} ${styles.moneyPanel}`} aria-labelledby="money-title"><div className={styles.panelHeader}><div><h2 id="money-title">Tiền cuối kỳ</h2><p>Số dư tại {monthLabel(period)}</p></div><Icon name="wallet" size={18} /></div>
        <Link className={styles.moneyRow} to={isDemo ? '/cash' : '/bank'}><span>Tiền gửi ngân hàng</span><strong>{m.bank === null ? 'Chưa có dữ liệu' : formatMoney(m.bank)}<small>đ</small></strong></Link>
        <Link className={styles.moneyRow} to="/cash"><span>Tiền mặt & tiền khác</span><strong>{m.money === null || m.bank === null ? 'Chưa có dữ liệu' : formatMoney(m.money - m.bank)}<small>đ</small></strong></Link>
        <div className={styles.moneyTotal}><span>Tổng tiền trên sổ</span><strong>{m.money === null ? 'Chưa có dữ liệu' : formatMoney(m.money)}<small>đ</small></strong></div>
        <div className={styles.moneyNote}>Theo nhóm tài khoản tiền. Nhấp từng khoản để tra cứu chi tiết.</div>
      </section>
    </div>
  </DataBoundary>;
}
