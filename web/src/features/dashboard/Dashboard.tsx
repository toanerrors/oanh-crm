import { Button, Dropdown, Progress, Table, Tooltip } from 'antd';
import { Link, useNavigate } from 'react-router-dom';
import { useWorkspace } from '../../app/WorkspaceProvider';
import { accountBalance, partnerBalances, periodDocuments, periodMovement, totals } from '../../shared/accounting/ledger';
import { chartRatio, compactMoney, formatDate, formatMoney } from '../../shared/accounting/money';
import { documentTypes } from '../../shared/data/types';
import type { AccountingDocument } from '../../shared/data/types';
import { Icon } from '../../shared/ui/Icon';
import type { IconName } from '../../shared/ui/Icon';
import { DataBoundary, PageHeading, ScopeNote, StatusTag } from '../../shared/ui/Page';
import styles from '../../app/App.module.css';
import { apiScope, isDemo } from '../../shared/api/client';

export function Component() {
  const { data, period, setPeriod, newDraft, openDocument, refresh, isFetching } = useWorkspace();
  const navigate = useNavigate();
  const periodLabel = period.split('-').reverse().join('/');
  if (!data) return <DataBoundary><></></DataBoundary>;
  const documents = periodDocuments(data.documents, period);
  const revenue = data.server ? BigInt(data.server.revenue) : periodMovement(data.documents, data.accounts, period, 'revenue');
  const expense = data.server ? BigInt(data.server.expense) : periodMovement(data.documents, data.accounts, period, 'expense');
  const cash = data.server ? data.accounts.filter(item => item.group === 'cash').reduce((sum, item) => sum + BigInt(data.server!.balances[item.id]), 0n) : accountBalance(data.documents, data.accounts.filter(item => item.group === 'cash').map(item => item.id), period);
  const receivable = data.server ? BigInt(data.server.receivable) : partnerBalances(data, period, 'receivable').reduce((sum, item) => sum + (item.balance > 0n ? item.balance : 0n), 0n);
  const metrics: { label: string; value: bigint; icon: IconName; color: string; note: string; link: string }[] = [
    { label: 'Doanh thu trong kỳ', value: revenue, icon: 'chart', color: 'green', note: 'Từ các chứng từ đã ghi sổ', link: '/sales' },
    { label: 'Chi phí trong kỳ', value: expense, icon: 'wallet', color: 'orange', note: 'Từ các tài khoản chi phí', link: '/reports?tab=trial' },
    { label: 'Tiền mặt & tiền gửi', value: cash, icon: 'bag', color: 'blue', note: 'Số dư đến cuối kỳ đã chọn', link: '/cash' },
    { label: 'Khách hàng còn phải trả', value: receivable, icon: 'users', color: 'purple', note: 'Tổng dư Nợ theo đối tượng', link: '/receivables' },
  ];
  const monthIndex = Number(period.slice(5)) - 1;
  const months = data.server ? data.server.months.map(month => ({ ...month, label: `Thg ${month.key.slice(5).replace(/^0/, '')}`, revenue: BigInt(month.revenue), expense: BigInt(month.expense) })) : Array.from({ length: 6 }, (_, index) => {
    const date = new Date(Date.UTC(Number(period.slice(0, 4)), monthIndex - 5 + index, 1));
    const key = date.toISOString().slice(0, 7);
    return { key, label: `Thg ${date.getUTCMonth() + 1}`, revenue: periodMovement(data.documents, data.accounts, key, 'revenue'), expense: periodMovement(data.documents, data.accounts, key, 'expense') };
  });
  const maximum = months.reduce((max, item) => { const largest = item.revenue > item.expense ? item.revenue : item.expense; return largest > max ? largest : max; }, 1n);
  const bank = data.server ? data.server.bankBalance !== undefined ? BigInt(data.server.bankBalance) : BigInt(data.server.balances['1121']) + BigInt(data.server.balances['112'] ?? '0') : accountBalance(data.documents, ['1121', '112'], period);
  const pocket = data.server ? BigInt(data.server.balances['111']) : accountBalance(data.documents, ['111'], period);
  const canWrite = isDemo || apiScope().permissions !== 'viewer';
  const drafts = documents.filter(item => item.status === 'draft');
  const posted = data.server?.postedCount ?? documents.filter(item => item.status === 'posted').length;
  const documentCount = data.server?.documentCount ?? documents.length;
  const draftCount = data.server?.draftCount ?? drafts.length;
  const unbalanced = drafts.filter(item => { const total = totals(item.lines); return total.debit !== total.credit; });
  const ratio = chartRatio(bank > 0n ? bank : 0n, (bank > 0n ? bank : 0n) + (pocket > 0n ? pocket : 0n));

  return <DataBoundary>
    <PageHeading title="Tổng quan" description="Một góc nhìn rõ ràng về tình hình kinh doanh của bạn." actions={<><Tooltip title="Tải lại dữ liệu trong phiên"><Button icon={<Icon name="refresh" size={16} />} loading={isFetching} onClick={refresh} aria-label="Tải lại tổng quan" /></Tooltip>{canWrite && <Dropdown trigger={['click']} menu={{ items: Object.entries(documentTypes).map(([key, label]) => ({ key, label, onClick: () => newDraft(key as AccountingDocument['type']) })) }}><Button type="primary" icon={<Icon name="plus" size={17} />}>Tạo chứng từ <Icon name="down" size={13} /></Button></Dropdown>}</>} />
    <div className={styles.welcomeStrip}><span className={styles.welcomeSpark}><Icon name="spark" size={19} /></span><p><strong>Chào {isDemo ? 'Minh Anh' : apiScope().name}, chúc một ngày làm việc hiệu quả!</strong><span>Đây là bức tranh tài chính của bạn trong tháng {periodLabel}.</span></p><span className={styles.demoPill}>{isDemo ? 'Dữ liệu mẫu' : 'VND · Sổ đã ghi nhận'}</span></div>
    {canWrite && <section className={styles.quickActions}><span>Thao tác nhanh</span>{([{ title: 'Phiếu thu', type: 'receipt', icon: 'arrowDown' }, { title: 'Phiếu chi', type: 'payment', icon: 'arrowUp' }, { title: 'Bán hàng', type: 'sale', icon: 'cart' }] as const).map(item => <button key={item.type} onClick={() => newDraft(item.type)}><Icon name={item.icon} size={16} />{item.title}<Icon name="plus" size={14} /></button>)}<Link to="/catalog"><Icon name="users" size={16} />Đối tượng<Icon name="plus" size={14} /></Link></section>}
    <section className={styles.metrics} aria-label="Chỉ số tài chính">{metrics.map(metric => { const value = compactMoney(metric.value); return <Tooltip key={metric.label} title={`${formatMoney(metric.value)} VND`} trigger={['hover', 'focus']}><Link to={metric.link} className={`${styles.metricCard} ${styles[metric.color]}`}><div className={styles.metricTop}><span>{metric.label}</span><span className={`${styles.metricIcon} ${styles[metric.color]}`}><Icon name={metric.icon} size={19} /></span></div><div className={styles.metricAmount}><strong>{value.amount}</strong><span>{value.unit}</span></div><div className={styles.metricBottom}><span>{metric.note}</span><Icon name="upRight" size={15} /></div></Link></Tooltip>; })}</section>
    <div className={styles.chartRow}>
      <section className={`${styles.panel} ${styles.revenuePanel}`}><div className={styles.panelHeader}><div><h2>Doanh thu & chi phí</h2><p>Theo dõi hiệu quả kinh doanh qua từng tháng</p></div><span className={styles.softLabel}>6 tháng gần nhất</span></div>
        <div className={styles.chartSummary}><div><span>Doanh thu tháng {period.slice(5)}</span><strong>{formatMoney(revenue)}<small>đ</small></strong></div><div className={styles.chartLegend}><span><i className={styles.revenueDot} />Doanh thu</span><span><i className={styles.expenseDot} />Chi phí</span></div></div>
        <div className={styles.barChart} role="img" aria-label={`Biểu đồ doanh thu và chi phí 6 tháng, đến ${periodLabel}. Số tiền chính xác có trong các nút tháng bên dưới.`}>
          <div className={styles.chartAxis}>{[4n, 3n, 2n, 1n, 0n].map(tick => <span key={String(tick)}>{compactMoney(maximum * tick / 4n).amount}<small>{tick ? compactMoney(maximum * tick / 4n).unit : ''}</small></span>)}</div>
          <div className={styles.plot}><div className={styles.gridLines}>{[0, 1, 2, 3, 4].map(index => <i key={index} />)}</div><div className={styles.barGroups}>{months.map(month => <div className={styles.barGroup} key={month.key}><div className={styles.barPair}><div className={styles.revenueBar} style={{ height: `${chartRatio(month.revenue, maximum) * 100}%` }} /><div className={styles.expenseBar} style={{ height: `${chartRatio(month.expense, maximum) * 100}%` }} /></div></div>)}</div></div>
        </div><div className={styles.chartMonths}>{months.map(month => <Tooltip key={month.key} title={`Doanh thu: ${formatMoney(month.revenue)} đ · Chi phí: ${formatMoney(month.expense)} đ`}><button className={month.key === period ? styles.selectedMonth : ''} onClick={() => setPeriod(month.key)} aria-label={`${month.label}: doanh thu ${formatMoney(month.revenue)} đồng, chi phí ${formatMoney(month.expense)} đồng`}>{month.label}</button></Tooltip>)}</div>
        <div className={styles.chartFooter}><Icon name="shield" size={14} /><span>{isDemo ? 'Chỉ bao gồm chứng từ mẫu đã ghi sổ' : 'Chỉ bao gồm chứng từ đã ghi sổ'}</span><Link to="/reports">Xem sổ chi tiết <Icon name="arrow" size={14} /></Link></div>
      </section>
      <section className={`${styles.panel} ${styles.cashPanel}`}><div className={styles.panelHeader}><div><h2>Cơ cấu tiền</h2><p>Số dư đến cuối kỳ {periodLabel}</p></div><Tooltip title="Xem sổ tiền mặt và ngân hàng"><Button type="text" size="small" icon={<Icon name="upRight" size={17} />} aria-label="Xem sổ tiền" onClick={() => navigate('/cash')} /></Tooltip></div>
        <div className={styles.donutWrap}><svg viewBox="0 0 200 200" className={styles.donut} aria-hidden="true"><circle cx="100" cy="100" r="76" fill="none" stroke="#e6f3ed" strokeWidth="18" /><circle cx="100" cy="100" r="76" fill="none" stroke="#119478" strokeWidth="18" strokeDasharray={`${ratio * 477.5} 477.5`} strokeLinecap="round" transform="rotate(-90 100 100)" /></svg><div className={styles.donutText}><span>Tổng số dư</span><strong>{compactMoney(cash).amount}<small>{compactMoney(cash).unit}</small></strong><span>VND</span></div></div>
        <div className={styles.cashLegend}><span><i className={styles.revenueDot} />Tiền gửi ngân hàng<strong>{formatMoney(bank)}<small>đ</small></strong></span><span><i className={styles.expenseDot} />Tiền mặt<strong>{formatMoney(pocket)}<small>đ</small></strong></span></div><Link to="/cash" className={styles.panelBottomLink}>Quản lý dòng tiền <Icon name="arrow" size={16} /></Link>
      </section>
    </div>
    <div className={styles.lowerRow}>
      <section className={`${styles.panel} ${styles.recentPanel}`}><div className={styles.panelHeader}><div><h2>Chứng từ gần đây <span className={styles.countBadge}>{documentCount}</span></h2><p>Các nghiệp vụ phát sinh trong kỳ</p></div><Link className={styles.textLink} to="/documents">Xem tất cả <Icon name="arrow" size={15} /></Link></div><Table<AccountingDocument> rowKey="id" dataSource={documents.slice(0, 5)} pagination={false} size="small" scroll={{ x: 750 }} locale={{ emptyText: 'Chưa có chứng từ trong kỳ' }} columns={[
        { title: 'CHỨNG TỪ', dataIndex: 'reference', width: 120, render: (value: string, record) => <button className={styles.tableLink} onClick={() => openDocument(record.id)}>{value}</button> },
        { title: 'DIỄN GIẢI', dataIndex: 'description', ellipsis: true, render: (value: string, record) => <><span>{value}</span><small className={styles.cellSub}>{formatDate(record.date)} · {documentTypes[record.type]}</small></> },
        { title: 'SỐ TIỀN', align: 'right', width: 200, render: (_, record) => <strong className={styles.tableAmount}>{formatMoney(totals(record.lines).debit)}</strong> },
        { title: 'TRẠNG THÁI', dataIndex: 'status', width: 120, render: value => <StatusTag status={value} /> },
      ]} /></section>
      <section className={`${styles.panel} ${styles.todoPanel}`}><div className={styles.panelHeader}><div><h2>Việc cần chú ý <span className={styles.countBadge}>{draftCount}</span></h2><p>Một chút kiểm tra, an tâm mỗi ngày</p></div><Icon name="clock" size={18} /></div>
        <button className={styles.todoItem} onClick={() => navigate('/documents?status=draft')}><span className={`${styles.todoIcon} ${styles.orange}`}><Icon name="file" size={19} /></span><span><strong>{draftCount} chứng từ đang ở bản nháp</strong><small>Kiểm tra nội dung trước khi ghi sổ</small></span><Icon name="chevron" size={15} /></button>
        <button className={styles.todoItem} onClick={() => navigate('/reports?tab=trial')}><span className={`${styles.todoIcon} ${styles.blue}`}><Icon name="book" size={19} /></span><span><strong>Đối chiếu số phát sinh</strong><small>{unbalanced.length ? `${unbalanced.length} nháp đang lệch Nợ/Có` : 'Tra cứu cân đối Nợ/Có trong kỳ'}</small></span><Icon name="chevron" size={15} /></button>
        <div className={styles.progressCard}><div><span>{isDemo ? 'Chứng từ mẫu đã ghi sổ' : 'Chứng từ đã ghi sổ'}</span><strong>{posted}/{documentCount}</strong></div><Progress percent={documentCount ? Math.round(posted * 100 / documentCount) : 0} showInfo={false} strokeColor="#119478" railColor="#e4ede7" size="small" /><p><Icon name="check" size={14} />Nháp được tách riêng khỏi số liệu trên sổ</p></div>
      </section>
    </div>
    <ScopeNote />
  </DataBoundary>;
}
