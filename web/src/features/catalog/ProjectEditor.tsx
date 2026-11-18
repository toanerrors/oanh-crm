import { App, Button, Form, Input, Modal, Switch } from 'antd';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { isDemo } from '../../shared/api/client';
import type { Project, Workspace } from '../../shared/data/types';
import { saveDemoProjects, workspaceKey } from '../../shared/data/workspace';
import styles from '../../app/App.module.css';

export function ProjectEditor({ project, existing = false, onClose, onSaved }: { project: Project; existing?: boolean; onClose: () => void; onSaved?: (project: Project) => void }) {
  const { message } = App.useApp();
  const client = useQueryClient();
  const [form] = Form.useForm<Project>();
  const mutation = useMutation({
    retry: false,
    mutationFn: (value: Project) => saveDemoProjects([value]),
    onSuccess: async ([saved]) => {
      if (saved) {
        client.setQueriesData<Workspace>({ queryKey: workspaceKey() }, current => current ? { ...current, projects: [...(current.projects ?? []).filter(item => item.id !== saved.id), saved] } : current);
      }
      void message.success(isDemo ? 'Đã cập nhật công trình trong phiên mẫu.' : 'Đã lưu công trình vào cơ sở dữ liệu.');
      try { await client.invalidateQueries({ queryKey: workspaceKey() }); } catch { void message.warning('Đã lưu; danh mục chưa tải lại được.'); }
      if (saved) onSaved?.(saved);
      onClose();
    },
    onError: error => void message.error(error.message),
  });
  return <Modal open title={existing ? 'Thông tin công trình' : isDemo ? 'Thêm công trình mẫu' : 'Thêm công trình'} onCancel={() => { if (!mutation.isPending) onClose(); }} footer={null} closable={!mutation.isPending} mask={{ closable: !mutation.isPending }}>
    <p className={styles.muted}>{isDemo ? 'Thay đổi chỉ tồn tại trong phiên mẫu.' : 'Thay đổi được lưu vào cơ sở dữ liệu của doanh nghiệp.'} Dùng công trình để tập hợp doanh thu và chi phí theo từng dự án, hợp đồng hoặc vụ việc.</p>
    <Form<Project> name="projectEditor" form={form} layout="vertical" onKeyDown={event => { if ((event.ctrlKey || event.metaKey) && (event.key.toLowerCase() === 's' || event.key === 'Enter')) { event.preventDefault(); event.stopPropagation(); if (!mutation.isPending) form.submit(); } }} initialValues={project} disabled={mutation.isPending} onFinish={values => mutation.mutate({ ...project, ...values })}>
      <div className={styles.formGrid}>
        <Form.Item name="code" label="Mã công trình" rules={[{ required: true, whitespace: true, message: 'Nhập mã công trình' }, { pattern: /^[A-Za-z0-9_-]+$/, message: 'Dùng chữ không dấu, chữ số, dấu gạch ngang hoặc gạch dưới' }]}><Input maxLength={40} disabled={mutation.isPending} placeholder="VD: CT001" /></Form.Item>
        <Form.Item name="active" label="Đang sử dụng" valuePropName="checked"><Switch /></Form.Item>
      </div>
      <Form.Item name="name" label="Tên công trình" rules={[{ required: true, whitespace: true, message: 'Nhập tên công trình' }]}><Input maxLength={300} disabled={mutation.isPending} placeholder="VD: Cho thuê lao động dự án Nhà máy A" /></Form.Item>
      <Form.Item name="description" label="Mô tả / ghi chú"><Input maxLength={500} placeholder="Chủ đầu tư, địa điểm, số hợp đồng hoặc giai đoạn" /></Form.Item>
      <div className={styles.modalActions}><Button disabled={mutation.isPending} onClick={onClose}>Đóng</Button><Button type="primary" htmlType="submit" loading={mutation.isPending}>{isDemo ? 'Lưu trong phiên mẫu' : 'Lưu công trình'}</Button></div>
    </Form>
  </Modal>;
}
