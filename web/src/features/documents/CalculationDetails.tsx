import { useState } from 'react';
import { Alert, Button, Descriptions, Modal, Table } from 'antd';
import { useQuery } from '@tanstack/react-query';
import { api } from '../../shared/api/client';
import { workspaceKey } from '../../shared/data/workspace';
import { amountLabels } from '../../shared/data/operations';
import type { Calculation, OperationInput, Policy } from '../../shared/data/operations';
import { formatDate, formatMoney } from '../../shared/accounting/money';
import { useWorkspace } from '../../app/WorkspaceProvider';

interface Trace { input: OperationInput; calculation: Calculation; policy: Policy }
export function CalculationDetails({ id }: { id: string }) {
  const { openDocument } = useWorkspace();
  const [open, setOpen] = useState(false);
  const query = useQuery({ queryKey: [...workspaceKey(), 'calculation', id], queryFn: ({ signal }) => api<Trace>(`/documents/${id}/calculation`, { signal }), enabled: open, retry: false });
  const trace = query.data;
  return <><Button onClick={() => setOpen(true)}>Xem cơ sở tính và chính sách</Button><Modal open={open} onCancel={() => setOpen(false)} title="Cơ sở tính đã lưu cùng chứng từ" width={800} footer={<Button onClick={() => setOpen(false)}>Đóng</Button>} loading={query.isPending}>
    {query.error && <Alert type="error" title={query.error.message} />}
    {trace && <><Descriptions column={2} items={[
      { key: 'source', label: 'Mã nguồn đã nhập', children: trace.input.sourceId }, { key: 'date', label: 'Ngày kế toán', children: formatDate(trace.input.date) },
      ...(trace.input.invoiceNumber ? [{ key: 'invoice', label: 'Hóa đơn', children: `${trace.input.invoiceSeries ?? ''} ${trace.input.invoiceNumber}`.trim() }, { key: 'invoiceDate', label: 'Ngày hóa đơn', children: formatDate(trace.input.invoiceDate ?? '') }] : []),
      ...(trace.input.dueDate ? [{ key: 'due', label: 'Hạn thanh toán', children: formatDate(trace.input.dueDate) }] : []),
      ...(trace.input.moneyAccount ? [{ key: 'money', label: 'Tài khoản tiền', children: trace.input.moneyAccount }] : []),
      { key: 'policy', label: 'Phiên bản chính sách', children: trace.policy.version }, { key: 'effective', label: 'Hiệu lực từ', children: formatDate(trace.policy.effectiveFrom) },
      { key: 'round', label: 'Làm tròn', children: { half_up: 'Nửa đơn vị lên', half_even: 'Nửa đơn vị về số chẵn', down: 'Bỏ phần lẻ' }[trace.policy.rounding] },
      { key: 'inventory', label: 'Giá xuất kho', children: trace.policy.inventoryMethod === 'fifo' ? 'FIFO' : 'Bình quân sau lần nhập' },
      ...(trace.input.workedDays ? [{ key: 'days', label: 'Ngày công / ngày chuẩn', children: `${trace.input.workedDays} / ${trace.policy.standardDays}` }, { key: 'dependents', label: 'Người phụ thuộc đã áp dụng', children: trace.calculation.amounts.dependentCount }] : []),
    ]} /><Table rowKey="key" size="small" pagination={false} dataSource={Object.entries(trace.calculation.amounts).filter(([key]) => key !== 'dependentCount').map(([key, value]) => ({ key, label: amountLabels[key] ?? key.replace('insurance.employee.', 'BH người lao động · ').replace('insurance.employer.', 'BH doanh nghiệp · '), value }))} columns={[{ title: 'CHỈ TIÊU', dataIndex: 'label' }, { title: 'SỐ TIỀN (VND)', dataIndex: 'value', align: 'right', render: formatMoney }]} />
      {trace.input.lines && <Table rowKey={(_, index) => String(index)} size="small" pagination={false} dataSource={trace.input.lines} style={{ marginTop: 20 }} columns={[{ title: 'MÃ HÀNG', dataIndex: 'itemId' }, { title: 'SỐ LƯỢNG', dataIndex: 'quantity' }, { title: 'ĐƠN GIÁ ĐÃ NHẬP', dataIndex: 'unitPrice' }, { title: 'VAT (%)', dataIndex: 'vatRate' }]} />}
      {trace.input.allocations?.length ? <Table rowKey="documentId" size="small" pagination={false} dataSource={trace.input.allocations} columns={[{ title: 'CHỨNG TỪ NGUỒN', dataIndex: 'documentId', render: (value: string) => <Button type="link" onClick={() => { setOpen(false); openDocument(value); }}>{value}</Button> }, { title: 'PHÂN BỔ (VND)', dataIndex: 'amount', align: 'right', render: formatMoney }]} /> : null}
      {trace.calculation.notes.map(note => <p key={note}>{note}</p>)}<p>Thông số và kết quả này được lưu khi ghi nhận, giữ nguyên khi danh mục hoặc chính sách mới thay đổi.</p>
    </>}
  </Modal></>;
}
