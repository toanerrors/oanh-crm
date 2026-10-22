import { useState } from 'react';
import { Alert, Button, Select, Table, Tag } from 'antd';
import { useQuery } from '@tanstack/react-query';
import { Link, useLocation, useNavigate, useSearchParams } from 'react-router-dom';
import { useWorkspace } from '../../app/WorkspaceProvider';
import { api, apiScope, isDemo } from '../../shared/api/client';
import { workspaceKey } from '../../shared/data/workspace';
import type { InvoiceList, TradeInvoice } from '../../shared/data/operations';
import { businessToday, formatDate, formatMoney } from '../../shared/accounting/money';
import { DataBoundary, PageHeading, PageToolbar } from '../../shared/ui/Page';
import { searchOptions } from '../../shared/ui/search';
import { ToolbarButton } from '../../shared/ui/ToolbarButton';
import { DateInput } from '../../shared/ui/DateInput';
import styles from '../../app/App.module.css';
import { accountingModule } from '../../app/module-navigation';

export function Component() {
  const { period } = useWorkspace();
  return <SettlementPage key={period} />;
}

function SettlementPage() {
  const { data, period, openDocument } = useWorkspace();
  const [params, setParams] = useSearchParams();
  const navigate = useNavigate();
  const module = accountingModule(useLocation().pathname);
  const kind = module && 'tradeKind' in module ? module.tradeKind : params.get('kind') === 'purchase' ? 'purchase' : 'sale';
  const [asOf, setAsOf] = useState(businessToday().startsWith(period) ? businessToday() : `${period}-${new Date(Number(period.slice(0, 4)), Number(period.slice(5)), 0).getDate()}`);
  const [partner, setPartner] = useState('');
  const [page, setPage] = useState(1);
  const queryParams = new URLSearchParams({ asOf, kind, partner, page: String(page), pageSize: '10' });
  const query = useQuery({ queryKey: [...workspaceKey(), 'invoices', queryParams.toString()], queryFn: ({ signal }) => api<InvoiceList>(`/trade-invoices?${queryParams}`, { signal }), enabled: !isDemo && Boolean(data && asOf), retry: false });
  if (isDemo) return <Alert type="info" title="Công nợ theo chứng từ sử dụng backend và dữ liệu doanh nghiệp." />;
  const result = query.data;
  const pay = (invoice: TradeInvoice) => navigate(`/automation?${new URLSearchParams({ kind: kind === 'sale' ? 'receipt' : 'payment', invoice: invoice.documentId, partner: invoice.partnerId, amount: invoice.remaining })}`);
  const pageActions = <Link to={module ? `${module.path}/reports?tab=debt&debt=${kind === 'purchase' ? 'payable' : 'receivable'}` : '/receivables'}><ToolbarButton label="Tổng hợp công nợ theo đối tượng" icon="users" /></Link>;

  return <DataBoundary>
    <PageHeading title="Công nợ theo chứng từ" description="Giá trị gốc và số đã/chưa phân bổ theo từng chứng từ đến ngày tra cứu." />
    <section className={styles.panel}>
      <PageToolbar label="Công cụ công nợ" actions={pageActions}>
        <Select disabled={Boolean(module)} aria-label="Loại chứng từ công nợ" value={kind} onChange={value => { setParams({ kind: value }); setPage(1); setPartner(''); }} options={[{ value: 'sale', label: 'Phải thu · Chứng từ bán hàng' }, { value: 'purchase', label: 'Phải trả · Chứng từ mua hàng' }]} />
        <label className={styles.toolbarDateFilter}>Đến ngày <DateInput style={{ width: 180 }} aria-label="Ngày tra cứu công nợ" value={asOf} onChange={value => { setAsOf(value); setPage(1); }} /></label>
        <Select aria-label="Đối tượng công nợ" showSearch={searchOptions} value={partner} onChange={value => { setPartner(value); setPage(1); }} style={{ minWidth: 220 }} options={[{ value: '', label: 'Tất cả đối tượng' }, ...data?.partners.map(item => ({ value: item.id, label: `${item.code} · ${item.name}` })) ?? []]} />
      </PageToolbar>
      {query.error && <Alert type="error" title={query.error.message} action={<Button onClick={() => void query.refetch()}>Thử lại</Button>} />}
      <Table<TradeInvoice> rowKey="documentId" loading={query.isFetching} dataSource={result?.invoices ?? []} scroll={{ x: 1100 }} pagination={{ current: page, onChange: setPage, pageSize: 10, total: result?.total ?? 0, showSizeChanger: false, hideOnSinglePage: true }} columns={[
        { title: 'CHỨNG TỪ', dataIndex: 'reference', render: (value: string, row) => <button className={styles.tableLink} onClick={() => openDocument(row.documentId)}>{value}</button> },
        { title: 'NGÀY', dataIndex: 'date', render: formatDate },
        { title: 'SỐ HÓA ĐƠN', dataIndex: 'invoiceNumber', render: (value: string, row) => value ? `${row.invoiceSeries} ${value}`.trim() : 'Chưa khai báo' },
        { title: 'ĐỐI TƯỢNG', dataIndex: 'partnerName' },
        { title: 'HẠN THANH TOÁN', dataIndex: 'dueDate', render: (value: string, row) => value ? <>{formatDate(value)}{value < asOf && row.remaining !== '0' && <Tag color="orange">Quá hạn</Tag>}</> : 'Chưa khai báo' },
        { title: 'CÔNG NỢ GỐC', dataIndex: 'amount', align: 'right', render: formatMoney },
        { title: 'ĐÃ PHÂN BỔ', dataIndex: 'paid', align: 'right', render: formatMoney },
        { title: 'CHƯA PHÂN BỔ', dataIndex: 'remaining', align: 'right', render: formatMoney },
        { title: 'THANH TOÁN', render: (_, row) => row.unallocated ? <Tag color="orange">Cần đối soát</Tag> : row.remaining === '0' ? <Tag color="green">Đã thanh toán</Tag> : apiScope().permissions !== 'viewer' ? <Button onClick={() => pay(row)}>{kind === 'sale' ? 'Thu tiền' : 'Trả tiền'}</Button> : <Tag>Chưa thanh toán đủ</Tag> },
      ]} summary={() => result && <Table.Summary.Row><Table.Summary.Cell index={0} colSpan={5}><strong>Tổng toàn bộ tập lọc (VND)</strong></Table.Summary.Cell>{[result.amount, result.paid, result.remaining].map((value, index) => <Table.Summary.Cell key={index} index={index + 5} align="right"><strong>{formatMoney(value)}</strong></Table.Summary.Cell>)}<Table.Summary.Cell index={8} /></Table.Summary.Row>} />
      <div className={styles.adminContent}><p className={styles.muted}>Công nợ từ toàn bộ chứng từ mua/bán đến {asOf || 'ngày đã chọn'} · Phiên bản {result?.revision ?? '—'} · VND.</p><p className={styles.muted}>Chứng từ mua/bán thủ công, bút toán giảm công nợ ngoài luồng phân bổ hoặc thanh toán sau ngày tra cứu được đánh dấu cần đối soát. Số còn lại của các dòng này chưa phản ánh đầy đủ thanh toán và không dùng để lập phiếu tự động. Công nợ ghi bằng chứng từ khác xem tại bảng tổng hợp đối tượng.</p></div>
    </section>
  </DataBoundary>;
}
