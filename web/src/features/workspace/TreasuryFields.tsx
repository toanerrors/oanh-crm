import { useState } from 'react';
import { Alert, AutoComplete, Button, Form, Input, Select } from 'antd';
import { useQuery } from '@tanstack/react-query';
import { useSearchParams } from 'react-router-dom';
import { api } from '../../shared/api/client';
import { workspaceKey } from '../../shared/data/workspace';
import type { InvoiceList, OperationInput } from '../../shared/data/operations';
import { formatMoney } from '../../shared/accounting/money';
import { useWorkspace } from '../../app/WorkspaceProvider';
import { MoneyInput } from '../../shared/ui/MoneyInput';
import { searchOptions } from '../../shared/ui/search';
import { PartnerSelect } from '../vouchers/PartnerSelect';
import styles from '../../app/App.module.css';

export function TreasuryFields({ kind }: { kind: OperationInput['kind'] }) {
  const { data } = useWorkspace();
  const [params] = useSearchParams();
  const form = Form.useFormInstance<OperationInput>();
  const date = Form.useWatch('date', form) as string | undefined;
  const partner = Form.useWatch('partnerId', form) as string | undefined;
  const [allocate, setAllocate] = useState(Boolean(params.get('invoice')));
  const queryParams = new URLSearchParams({ asOf: date ?? '', kind: kind === 'receipt' ? 'sale' : 'purchase', partner: partner ?? '', pageSize: '100' });
  const invoices = useQuery({ queryKey: [...workspaceKey(), 'invoices', queryParams.toString()], queryFn: ({ signal }) => api<InvoiceList>(`/trade-invoices?${queryParams}`, { signal }), enabled: allocate && Boolean(partner && date) && kind !== 'transfer', retry: false });
  const moneyOptions = data?.accounts.filter(item => item.active && ['111', '1121', '112'].includes(item.id)).map(item => ({ value: item.id, label: `${item.code} · ${item.name}` }));
  return <>
    <div className={styles.formGrid}>
      <Form.Item name="moneyAccount" label={kind === 'transfer' ? 'Tài khoản chuyển đi' : 'Tài khoản thu/chi'} rules={[{ required: true }]}><Select showSearch={searchOptions} options={moneyOptions} /></Form.Item>
      {kind === 'transfer' ? <Form.Item name="destinationAccount" label="Tài khoản nhận" rules={[{ required: true }]} preserve={false}><Select showSearch={searchOptions} options={moneyOptions} /></Form.Item> : <Form.Item label="Nội dung thanh toán"><Select aria-label="Nội dung thanh toán" value={allocate ? 'invoice' : 'other'} onChange={value => { setAllocate(value === 'invoice'); form.setFieldValue('allocations', undefined); form.setFieldValue('counterAccount', undefined); }} options={[{ value: 'other', label: 'Thu/chi khác' }, { value: 'invoice', label: kind === 'receipt' ? 'Thu công nợ theo chứng từ bán' : 'Trả công nợ theo chứng từ mua' }]} /></Form.Item>}
      <Form.Item name="amount" label="Số tiền thu/chi (VND)" rules={[{ required: true }]}><MoneyInput label="Số tiền thu/chi" /></Form.Item>
      {kind !== 'transfer' && !allocate && <Form.Item name="counterAccount" label="Tài khoản đối ứng" rules={[{ required: true }]} preserve={false}><Select showSearch={searchOptions} options={data?.accounts.filter(item => item.active && !['cash', 'inventory', 'asset', 'receivable', 'payable'].includes(item.group)).map(item => ({ value: item.id, label: `${item.code} · ${item.name}` }))} /></Form.Item>}
      {kind !== 'transfer' && <Form.Item name="partnerId" label={kind === 'receipt' ? 'Khách hàng / đối tượng nộp' : 'Nhà cung cấp / đối tượng nhận'} rules={[{ required: allocate }]} preserve={false}><PartnerSelect label={kind === 'receipt' ? 'Khách hàng / đối tượng nộp' : 'Nhà cung cấp / đối tượng nhận'} role={allocate ? kind === 'receipt' ? 'customer' : 'supplier' : 'all'} defaultRole={kind === 'receipt' ? 'customer' : 'supplier'} onSelected={() => form.setFieldValue('allocations', [])} /></Form.Item>}
    </div>
    {allocate && kind !== 'transfer' && <>
      {invoices.error && <Alert type="error" title={invoices.error.message} action={<Button onClick={() => void invoices.refetch()}>Thử lại</Button>} />}
      {(invoices.data?.total ?? 0) > 100 && <Alert type="info" title="Hiển thị 100 chứng từ gần nhất của đối tượng. Có thể nhập ID chứng từ từ màn hình Công nợ theo chứng từ." />}
      <Form.List name="allocations" rules={[{ validator: async (_, value: unknown[]) => { if (!value?.length) throw new Error('Thêm ít nhất một chứng từ để phân bổ.'); } }]}>{(fields, { add, remove }, { errors }) => <>
        {fields.map(field => <div className={styles.formGrid} key={field.key}>
          <Form.Item name={[field.name, 'documentId']} label="Chứng từ công nợ / ID nguồn" rules={[{ required: true }]}><AutoComplete showSearch={searchOptions} options={invoices.data?.invoices.filter(item => item.remaining !== '0' && !item.unallocated).map(item => ({ value: item.documentId, label: `${item.reference} · ${item.invoiceNumber || 'Chưa có số hóa đơn'} · Còn ${formatMoney(item.remaining)} VND` }))}><Input placeholder="Chọn chứng từ hoặc nhập ID nguồn" /></AutoComplete></Form.Item>
          <Form.Item name={[field.name, 'amount']} label="Số phân bổ (VND)" rules={[{ required: true }]}><MoneyInput label={`Số phân bổ ${field.name + 1}`} /></Form.Item>
          <Button danger onClick={() => remove(field.name)}>Xóa phân bổ {field.name + 1}</Button>
        </div>)}
        <Form.ErrorList errors={errors} /><Button type="dashed" onClick={() => add({ amount: '' })}>Thêm chứng từ thanh toán</Button>
      </>}</Form.List>
      <p className={styles.muted}>Tổng phân bổ phải bằng số tiền phiếu. Chứng từ có bút toán giảm công nợ chưa phân bổ cần được đối soát trước.</p>
    </>}
    {kind !== 'transfer' && !allocate && <Alert showIcon type="info" title="Người lập xác nhận tài khoản và số tiền đã chốt" description="Phiếu này không tự tách thuế. Nghiệp vụ mua/bán và tài sản cần lập tại phân hệ tương ứng trước khi thanh toán." />}
  </>;
}
