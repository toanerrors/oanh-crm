import { App, AutoComplete, Button, Form, Input, Modal, Radio, Switch } from 'antd';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { command } from '../../shared/api/client';
import type { Item } from '../../shared/data/operations';
import { workspaceKey } from '../../shared/data/workspace';
import { searchOptions, suggestionOptions } from '../../shared/ui/search';

export function ItemEditor({ item, items, onClose, onSaved }: { item: Item; items: Item[]; onClose: () => void; onSaved?: (item: Item) => void }) {
  const { message } = App.useApp();
  const client = useQueryClient();
  const [form] = Form.useForm<Item>();
  const existing = items.some(row => row.id === item.id);
  const mutation = useMutation({ retry: false, mutationFn: ({ id, code, name, unit, tracked, active }: Item) => command<Item>('/items', { id, code, name, unit, tracked, active }), onSuccess: async result => {
    void message.success('Đã lưu danh mục dùng chung cho mua và bán.');
    try { await client.invalidateQueries({ queryKey: workspaceKey() }); } catch { void message.warning('Đã lưu; danh mục chưa tải lại được.'); }
    onSaved?.(result); onClose();
  }, onError: error => void message.error(error.message) });
  return <Modal open title={existing ? 'Sửa hàng hóa / dịch vụ' : 'Thêm hàng hóa / dịch vụ'} onCancel={() => { if (!mutation.isPending) onClose(); }} footer={null} closable={!mutation.isPending} maskClosable={!mutation.isPending}>
    <Form<Item> form={form} layout="vertical" onKeyDown={event => { if ((event.ctrlKey || event.metaKey) && (event.key.toLowerCase() === 's' || event.key === 'Enter')) { event.preventDefault(); event.stopPropagation(); if (!mutation.isPending) form.submit(); } }} initialValues={item} disabled={mutation.isPending} onFinish={values => mutation.mutate({ ...item, ...values })}>
      <Form.Item name="tracked" label="Loại danh mục" rules={[{ required: true, message: 'Chọn hàng hóa hoặc dịch vụ' }]} extra="Hàng hóa có quản lý kho. Dịch vụ không cần kho. Mỗi mã dùng chung cho cả mua và bán."><Radio.Group optionType="button" buttonStyle="solid" options={[{ value: true, label: 'Hàng hóa' }, { value: false, label: 'Dịch vụ' }]} /></Form.Item>
      <Form.Item name="code" label="Mã" rules={[{ required: true, whitespace: true }, { pattern: /^[A-Za-z0-9][A-Za-z0-9_.:/-]{0,99}$/, message: 'Dùng mã không dấu, tối đa 100 ký tự' }]}><Input disabled={existing} maxLength={100} /></Form.Item>
      <Form.Item name="name" label="Tên" rules={[{ required: true, whitespace: true }]}><Input maxLength={200} /></Form.Item>
      <Form.Item name="unit" label="Đơn vị" rules={[{ required: true, whitespace: true }]}><AutoComplete disabled={existing} options={suggestionOptions(items.map(row => row.unit))} showSearch={searchOptions} defaultActiveFirstOption={false}><Input placeholder="Nhập hoặc chọn đơn vị đã dùng" maxLength={100} /></AutoComplete></Form.Item>
      <Form.Item name="active" label="Đang sử dụng" valuePropName="checked"><Switch /></Form.Item>
      {existing && <p>Có thể sửa loại khi mã chưa có chứng từ hoặc số dư kho. Mã và đơn vị được giữ nguyên.</p>}
      <Button type="primary" htmlType="submit" loading={mutation.isPending}>Lưu danh mục</Button>
    </Form>
  </Modal>;
}
