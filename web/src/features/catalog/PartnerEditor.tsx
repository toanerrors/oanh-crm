import { App, Button, Form, Input, Modal, Select, Switch } from 'antd';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { isDemo } from '../../shared/api/client';
import { partnerRoles } from '../../shared/data/partner-catalog';
import type { PartnerRoleFilter } from '../../shared/data/partner-catalog';
import type { Partner, Workspace } from '../../shared/data/types';
import { saveDemoPartners, workspaceKey } from '../../shared/data/workspace';
import styles from '../../app/App.module.css';

export function PartnerEditor({ partner, existing = false, allowedRole = 'all', onClose, onSaved }: { partner: Partner; existing?: boolean; referenced?: boolean; allowedRole?: PartnerRoleFilter; onClose: () => void; onSaved?: (partner: Partner) => void }) {
  const { message } = App.useApp();
  const client = useQueryClient();
  const [form] = Form.useForm<Partner>();
  const mutation = useMutation({
    retry: false,
    mutationFn: (value: Partner) => saveDemoPartners([value]),
    onSuccess: async ([saved]) => {
      if (saved) {
        client.setQueriesData<Workspace>({ queryKey: workspaceKey() }, current => current && Array.isArray(current.partners) ? { ...current, partners: [...current.partners.filter(item => item.id !== saved.id), saved] } : current);
      }
      void message.success(isDemo ? 'Đã cập nhật danh mục trong phiên mẫu.' : 'Đã lưu danh mục vào cơ sở dữ liệu.');
      try { await client.invalidateQueries({ queryKey: workspaceKey() }); } catch { void message.warning('Đã lưu; danh mục chưa tải lại được.'); }
      if (saved) onSaved?.(saved);
      onClose();
    },
    onError: error => void message.error(error.message),
  });
  return <Modal open title={existing ? 'Thông tin đối tượng' : isDemo ? 'Thêm đối tượng mẫu' : 'Thêm đối tượng'} onCancel={() => { if (!mutation.isPending) onClose(); }} footer={null} closable={!mutation.isPending} mask={{ closable: !mutation.isPending }}>
    <p className={styles.muted}>{isDemo ? 'Thay đổi chỉ tồn tại trong phiên mẫu.' : 'Thay đổi được lưu vào cơ sở dữ liệu của doanh nghiệp.'} Bạn có thể sửa mã, tên, vai trò, thông tin liên hệ và trạng thái sử dụng.</p>
    <Form<Partner> form={form} layout="vertical" onKeyDown={event => { if ((event.ctrlKey || event.metaKey) && (event.key.toLowerCase() === 's' || event.key === 'Enter')) { event.preventDefault(); event.stopPropagation(); if (!mutation.isPending) form.submit(); } }} initialValues={partner} disabled={mutation.isPending} onFinish={values => mutation.mutate({ ...partner, ...values })}>
      <div className={styles.formGrid}>
        <Form.Item name="code" label="Mã đối tượng" rules={[{ required: true, whitespace: true, message: 'Nhập mã đối tượng' }, { pattern: /^[A-Za-z0-9_-]+$/, message: 'Dùng chữ không dấu, chữ số, dấu gạch ngang hoặc gạch dưới' }]}><Input maxLength={40} disabled={mutation.isPending} placeholder={partner.role === 'supplier' ? 'VD: NCC005' : 'VD: KH005'} /></Form.Item>
        <Form.Item name="role" label="Vai trò" rules={[{ required: true }]}><Select options={Object.entries(partnerRoles).map(([value, label]) => ({ value, label, disabled: allowedRole !== 'all' && value !== 'both' && value !== allowedRole }))} disabled={mutation.isPending} /></Form.Item>
      </div>
      <Form.Item name="name" label="Tên đối tượng" rules={[{ required: true, whitespace: true, message: 'Nhập tên đối tượng' }]}><Input maxLength={200} disabled={mutation.isPending} /></Form.Item>
      <div className={styles.formGrid}>
        <Form.Item name="email" label="Email" rules={[{ type: 'email', message: 'Email không đúng định dạng' }]}><Input type="email" autoComplete="email" spellCheck={false} maxLength={254} /></Form.Item>
        <Form.Item name="phone" label="Điện thoại" rules={[{ pattern: /^[+\d ()-]*$/, message: 'Số điện thoại không hợp lệ' }]}><Input type="tel" autoComplete="tel" maxLength={30} /></Form.Item>
      </div>
      <Form.Item name="active" label="Đang sử dụng" valuePropName="checked"><Switch /></Form.Item>
      <div className={styles.modalActions}><Button disabled={mutation.isPending} onClick={onClose}>Đóng</Button><Button type="primary" htmlType="submit" loading={mutation.isPending}>{isDemo ? 'Lưu trong phiên mẫu' : 'Lưu đối tượng'}</Button></div>
    </Form>
  </Modal>;
}
