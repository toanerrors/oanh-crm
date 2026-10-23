import { useState } from 'react';
import { Alert, App, Button, Form, Input, Modal, Select, Switch, Table } from 'antd';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useWorkspace } from '../../app/WorkspaceProvider';
import { apiScope, command } from '../../shared/api/client';
import { automationOptions, warehousesOptions } from '../../shared/data/operations';
import type { Warehouse } from '../../shared/data/operations';
import { workspaceKey } from '../../shared/data/workspace';
import { formatMoney, formatQuantity } from '../../shared/accounting/money';
import { ToolbarButton } from '../../shared/ui/ToolbarButton';
import { Icon } from '../../shared/ui/Icon';
import { ImportExcelButton } from '../imports/ExcelImport';
import { searchOptions } from '../../shared/ui/search';

export function WarehouseSelect({ value, onChange, label, disabled }: { value?: string; onChange?: (value: string) => void; label: string; disabled?: boolean }) {
  const query = useQuery(warehousesOptions());
  const [open, setOpen] = useState(false);
  return <div style={{ display: 'flex', gap: 4 }}><Select style={{ flex: 1, minWidth: 0 }} value={value} disabled={disabled} aria-label={label} showSearch={searchOptions} options={query.data?.warehouses.filter(w => w.active || w.id === value).map(w => ({ value: w.id, label: w.code, disabled: !w.active }))} onChange={onChange} /><Button disabled={disabled} aria-label={`Quản lý ${label}`} icon={<Icon name="plus" size={14} />} onClick={() => setOpen(true)} /><WarehouseDirectory open={open} onClose={() => setOpen(false)} onSelected={id => { onChange?.(id); setOpen(false); }} /></div>;
}
export function WarehousesButton() { const [open, setOpen] = useState(false); return <><ToolbarButton onClick={() => setOpen(true)} label="Danh mục và tồn theo kho" icon="box" /><WarehouseDirectory open={open} onClose={() => setOpen(false)} /></>; }
function WarehouseDirectory({ open, onClose, onSelected }: { open: boolean; onClose: () => void; onSelected?: (id: string) => void }) {
  const { message } = App.useApp();
  const { data } = useWorkspace();
  const query = useQuery({ ...warehousesOptions(), enabled: open });
  const items = useQuery({ ...automationOptions(), enabled: open && apiScope().permissions !== 'viewer' });
  const client = useQueryClient();
  const [editing, setEditing] = useState<Warehouse | null>(null);
  const [form] = Form.useForm<Warehouse>();
  const canWrite = apiScope().permissions !== 'viewer';
  const mutation = useMutation({ retry: false, mutationFn: ({ id, code, name, active }: Warehouse) => command<Warehouse>('/warehouses', { id, code, name, active }), onSuccess: async result => { setEditing(null); void message.success('Đã lưu kho.'); try { await client.invalidateQueries({ queryKey: workspaceKey() }); } catch { void message.warning('Đã lưu; danh mục chưa tải lại được.'); } onSelected?.(result.id); }, onError: error => void message.error(error.message) });
  function edit(w?: Warehouse) { const next = w ?? { id: crypto.randomUUID(), code: '', name: '', active: true }; setEditing(next); form.resetFields(); form.setFieldsValue(next); }
  const referenced = Boolean(editing?.hasReferences);
  return <><Modal open={open} onCancel={onClose} title={`Danh mục và tồn theo kho · Revision ${query.data?.revision ?? data?.revision ?? '—'}`} width={1000} footer={<Button onClick={onClose}>Đóng</Button>}>
    {query.error && <Alert type="error" title={query.error.message} />}
    <p>Tồn hiện tại theo từng kho; giá xuất kho tính độc lập cho mỗi mặt hàng trong mỗi kho. Dữ liệu trước khi bổ sung nhiều kho thuộc Kho mặc định.</p>
    {canWrite && <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}><ImportExcelButton kind="warehouses" /><ToolbarButton label="Thêm kho" icon="plus" onClick={() => edit()} /></div>}
    <Table<Warehouse> rowKey="id" dataSource={query.data?.warehouses} pagination={{ pageSize: 10 }} columns={[{ title: 'Mã kho', dataIndex: 'code' }, { title: 'Tên kho', dataIndex: 'name' }, { title: 'Trạng thái', dataIndex: 'active', render: active => active ? 'Đang sử dụng' : 'Ngừng sử dụng' }, { render: (_, w) => <>{canWrite && <Button type="text" onClick={() => edit(w)}>Sửa</Button>}{onSelected && w.active && <Button type="link" onClick={() => onSelected(w.id)}>Chọn</Button>}</> }]} />
    <Table rowKey={row => `${row.warehouseId}:${row.itemId}:${row.scale}`} dataSource={query.data?.stock} scroll={{ x: 700 }} pagination={{ pageSize: 10 }} columns={[{ title: 'Kho', dataIndex: 'warehouseId', render: id => query.data?.warehouses.find(w => w.id === id)?.name ?? id }, { title: 'Hàng hóa', dataIndex: 'itemId', render: id => items.data?.items.find(i => i.id === id)?.name ?? id }, { title: 'Số lượng', align: 'right', render: (_, row) => formatQuantity(row.quantity, row.scale) }, { title: 'Giá trị', align: 'right', dataIndex: 'value', render: value => formatMoney(value) }]} />
  </Modal><Modal open={Boolean(editing)} onCancel={() => setEditing(null)} title="Kho hàng" footer={null}><Form<Warehouse> form={form} layout="vertical" disabled={mutation.isPending} onFinish={values => mutation.mutate({ ...editing!, ...values })}>
    <Form.Item name="code" label="Mã kho" rules={[{ required: true }, { pattern: /^[A-Za-z0-9][A-Za-z0-9_.:/-]{0,99}$/, message: 'Dùng mã không dấu, tối đa 100 ký tự' }]}><Input disabled={referenced || mutation.isPending} maxLength={100} /></Form.Item><Form.Item name="name" label="Tên kho" rules={[{ required: true, whitespace: true }]}><Input maxLength={200} /></Form.Item><Form.Item name="active" label="Đang sử dụng" valuePropName="checked"><Switch /></Form.Item><p>{referenced ? 'Kho đã có chứng từ hoặc số dư: giữ mã kho; tên và trạng thái vẫn sửa được.' : 'Bạn có thể sửa mã và tên kho khi chưa có chứng từ hoặc số dư. Thay đổi được ghi nhật ký.'}</p><Button type="primary" htmlType="submit" loading={mutation.isPending}>Lưu kho</Button>
  </Form></Modal></>;
}
