import { useState } from 'react';
import { Alert, App, Button, Form, Input, Modal, Select, Switch, Table } from 'antd';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useWorkspace } from '../../app/WorkspaceProvider';
import { apiScope, command } from '../../shared/api/client';
import { banksOptions } from '../../shared/data/operations';
import type { BankAccount } from '../../shared/data/operations';
import { workspaceKey } from '../../shared/data/workspace';
import { formatMoney } from '../../shared/accounting/money';
import { ToolbarButton } from '../../shared/ui/ToolbarButton';
import { Icon } from '../../shared/ui/Icon';
import { ImportExcelButton } from '../imports/ExcelImport';
import { searchOptions } from '../../shared/ui/search';

export function BankSelect({ value, onChange, onSelected, label, disabled }: { value?: string; onChange?: (value?: string) => void; onSelected?: (account?: BankAccount) => void; label: string; disabled?: boolean }) {
  const { period } = useWorkspace();
  const query = useQuery(banksOptions(period));
  const [open, setOpen] = useState(false);
  const choose = (id?: string) => { onChange?.(id); onSelected?.(query.data?.accounts.find(item => item.id === id)); };
  return <div style={{ display: 'flex', gap: 4, minWidth: 0 }}><Select style={{ minWidth: 0, flex: 1 }} value={value} disabled={disabled} aria-label={label} loading={query.isPending} showSearch={searchOptions} placeholder="Chọn tài khoản ngân hàng" options={query.data?.accounts.filter(item => item.active || item.id === value).map(item => ({ value: item.id, label: `${item.number} · ${item.bankName}`, disabled: !item.active }))} onChange={choose} /><Button disabled={disabled} icon={<Icon name="plus" size={14} />} aria-label={`Quản lý ${label}`} onClick={() => setOpen(true)} /><BankDirectory open={open} onClose={() => setOpen(false)} onSelected={account => { onChange?.(account.id); onSelected?.(account); setOpen(false); }} /></div>;
}

export function BankAccountsButton() {
  const [open, setOpen] = useState(false);
  return <><ToolbarButton onClick={() => setOpen(true)} label="Tài khoản ngân hàng" icon="building" /><BankDirectory open={open} onClose={() => setOpen(false)} /></>;
}

function BankDirectory({ open, onClose, onSelected }: { open: boolean; onClose: () => void; onSelected?: (account: BankAccount) => void }) {
  const { period, data } = useWorkspace();
  const { message } = App.useApp();
  const client = useQueryClient();
  const query = useQuery({ ...banksOptions(period), enabled: open });
  const [editing, setEditing] = useState<BankAccount | null>(null);
  const [form] = Form.useForm<BankAccount>();
  const canWrite = apiScope().permissions !== 'viewer';
  const mutation = useMutation({ retry: false, mutationFn: ({ id, number, bankName, accountId, active }: BankAccount) => command<BankAccount>('/bank-accounts', { id, number, bankName, accountId, active }), onSuccess: async result => {
    setEditing(null); void message.success('Đã lưu tài khoản ngân hàng.');
    try { await client.invalidateQueries({ queryKey: workspaceKey() }); } catch { void message.warning('Đã lưu; danh mục chưa tải lại được.'); }
    onSelected?.(result);
  }, onError: error => void message.error(error.message) });
  function edit(account?: BankAccount) {
    const next = account ?? { id: crypto.randomUUID(), number: '', bankName: '', accountId: '112', active: true };
    setEditing(next); form.resetFields(); form.setFieldsValue(next);
  }
  const existing = query.data?.accounts.some(item => item.id === editing?.id);
  return <><Modal open={open} onCancel={onClose} width={1100} title={`Tài khoản ngân hàng · Kỳ ${period}`} footer={<Button onClick={onClose}>Đóng</Button>}>
    {query.error && <Alert type="error" title={query.error.message} />}
    <p>Số dư chi tiết lấy từ chứng từ đã gắn tài khoản ngân hàng. Giao dịch cũ chưa gắn tài khoản vẫn nằm trong tổng sổ tiền gửi.</p>
    {canWrite && <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}><ImportExcelButton kind="banks" /><ToolbarButton label="Thêm tài khoản ngân hàng" icon="plus" onClick={() => edit()} /></div>}
    <Table<BankAccount> rowKey="id" loading={query.isPending} dataSource={query.data?.accounts} scroll={{ x: 950 }} pagination={{ pageSize: 10 }} columns={[
      { title: 'Số tài khoản', dataIndex: 'number' }, { title: 'Ngân hàng', dataIndex: 'bankName' }, { title: 'TK kế toán', dataIndex: 'accountId', width: 100 },
      { title: 'Dư đầu', dataIndex: 'opening', align: 'right', render: value => value === undefined ? '—' : formatMoney(value) },
      { title: 'Thu', dataIndex: 'receipts', align: 'right', render: value => value === undefined ? '—' : formatMoney(value) },
      { title: 'Chi', dataIndex: 'payments', align: 'right', render: value => value === undefined ? '—' : formatMoney(value) },
      { title: 'Dư cuối', dataIndex: 'closing', align: 'right', render: value => value === undefined ? '—' : formatMoney(value) },
      { title: 'Trạng thái', dataIndex: 'active', render: value => value ? 'Đang sử dụng' : 'Ngừng sử dụng' },
      { title: '', width: 130, render: (_, row) => <>{canWrite && <Button type="text" onClick={() => edit(row)}>Sửa</Button>}{onSelected && row.active && <Button type="link" onClick={() => onSelected(row)}>Chọn</Button>}</> },
    ]} />
  </Modal><Modal open={Boolean(editing)} onCancel={() => setEditing(null)} title="Tài khoản ngân hàng" footer={null}><Form<BankAccount> form={form} layout="vertical" onFinish={values => mutation.mutate({ ...editing!, ...values })}>
    <Form.Item name="number" label="Số tài khoản ngân hàng" rules={[{ required: true, whitespace: true }]}><Input maxLength={100} disabled={existing} /></Form.Item>
    <Form.Item name="bankName" label="Tên ngân hàng" rules={[{ required: true, whitespace: true }]}><Input maxLength={200} disabled={existing} /></Form.Item>
    <Form.Item name="accountId" label="Tài khoản kế toán tiền gửi" rules={[{ required: true }]}><Select disabled={existing} showSearch={searchOptions} options={data?.accounts.filter(item => item.active && item.group === 'cash' && item.id !== '111').map(item => ({ value: item.id, label: `${item.code} · ${item.name}` }))} /></Form.Item>
    <Form.Item name="active" label="Đang sử dụng" valuePropName="checked"><Switch /></Form.Item>
    <p>Giữ số tài khoản, tên ngân hàng và tài khoản kế toán đã tạo để bảo toàn lịch sử. Mỗi số tài khoản được khai báo một lần.</p>
    <Button type="primary" htmlType="submit" loading={mutation.isPending}>Lưu tài khoản ngân hàng</Button>
  </Form></Modal></>;
}
