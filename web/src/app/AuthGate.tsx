import { useEffect, useRef } from 'react';
import type { ReactNode } from 'react';
import { Alert, App, Button, Checkbox, Form, Input, Spin } from 'antd';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, isDemo, loadSession } from '../shared/api/client';
import { WorkspaceProvider } from './WorkspaceProvider';
import styles from './App.module.css';

interface LoginValues { username: string; password: string; name?: string; companyName?: string; regime?: string; confirmAccounts?: boolean; remember?: boolean }
export function AuthGate({ children }: { children: ReactNode }) {
  const client = useQueryClient();
  const { message } = App.useApp();
  const query = useQuery({ queryKey: ['session'], queryFn: ({ signal }) => loadSession(signal), enabled: !isDemo, retry: false, staleTime: 60_000 });
  const previousScope = useRef<{ actor: string; role: string; epoch: string } | null>(null);
  const identity = query.data?.user?.id;
  const role = query.data?.user?.role;
  const epoch = query.data?.context?.epoch;
  useEffect(() => {
    const previous = previousScope.current;
    if (previous && (previous.actor !== identity || previous.role !== role || previous.epoch !== epoch)) {
      const matches = (item: { queryKey: readonly unknown[] }) => item.queryKey[0] === 'workspace' && item.queryKey[1] === previous.actor && item.queryKey[2] === previous.role && item.queryKey[4] === previous.epoch;
      void client.cancelQueries({ predicate: matches });
      client.removeQueries({ predicate: matches });
    }
    previousScope.current = identity && role && epoch ? { actor: identity, role, epoch } : null;
  }, [client, identity, role, epoch]);
  const mutation = useMutation({ retry: false, mutationFn: async (values: LoginValues) => {
    const { remember, ...setupValues } = values;
    if (query.data?.needsSetup) await api('/setup', { method: 'POST', body: JSON.stringify(setupValues) });
    await api('/auth/login', { method: 'POST', body: JSON.stringify({ username: values.username, password: values.password, remember: !query.data?.needsSetup && remember === true }) });
  }, onSuccess: async () => { await client.invalidateQueries({ queryKey: ['session'] }); } });
  useEffect(() => {
    if (isDemo) return;
    const onSessionChanged = () => {
      client.setQueryData(['session'], { needsSetup: false });
      void client.cancelQueries({ predicate: item => item.queryKey[0] !== 'session' });
      client.removeQueries({ predicate: item => item.queryKey[0] !== 'session' });
      void message.warning('Phiên đăng nhập hoặc dữ liệu đã thay đổi. Đang tải lại thông tin truy cập.');
      void client.invalidateQueries({ queryKey: ['session'] });
    };
    window.addEventListener('oanh:session-changed', onSessionChanged);
    return () => window.removeEventListener('oanh:session-changed', onSessionChanged);
  }, [client, message]);
  if (isDemo) return <WorkspaceProvider>{children}</WorkspaceProvider>;
  if (query.isPending) return <div className={styles.authScreen}><Spin description="Đang kết nối không gian kế toán…" /></div>;
  if (query.error) return <div className={styles.authScreen}><Alert type="error" showIcon title="Không kết nối được máy chủ" description="Kiểm tra kết nối rồi thử lại." action={<Button onClick={() => void query.refetch()}>Thử lại</Button>} /></div>;
  if (query.data?.user && query.data.context) return <WorkspaceProvider key={`${query.data.user.id}:${query.data.user.role}:${query.data.context.epoch}`}>{children}</WorkspaceProvider>;
  const setup = query.data?.needsSetup;
  return <main className={styles.authScreen}><section className={styles.authCard}><div className={styles.authBrand}>oanh<span>.</span></div><h1>{setup ? 'Khởi tạo không gian kế toán' : 'Chào mừng trở lại'}</h1><p>{setup ? 'Tạo doanh nghiệp và tài khoản chủ sở hữu đầu tiên.' : 'Đăng nhập để tiếp tục làm việc với số liệu của doanh nghiệp.'}</p>
    {!setup && new URLSearchParams(window.location.search).get('restored') === '1' && <Alert showIcon type="success" title="Dữ liệu đã được khôi phục" description="Đăng nhập bằng tài khoản có trong bản sao lưu để tiếp tục." />}
    <Form<LoginValues> layout="vertical" initialValues={{ remember: true }} onFinish={values => mutation.mutate(values)} scrollToFirstError={{ focus: true }}>
      {setup && <><Form.Item name="companyName" label="Tên doanh nghiệp" rules={[{ required: true, whitespace: true }]}><Input maxLength={200} autoComplete="organization" /></Form.Item><Form.Item name="name" label="Tên chủ sở hữu" rules={[{ required: true, whitespace: true }]}><Input maxLength={150} autoComplete="name" /></Form.Item><Form.Item name="regime" label="Chế độ kế toán / phiên bản áp dụng" rules={[{ required: true, whitespace: true }]}><Input placeholder="Nhập chế độ kế toán doanh nghiệp đang áp dụng" maxLength={200} /></Form.Item></>}
      <Form.Item name="username" label="Tên đăng nhập" rules={[{ required: true }, { pattern: /^[A-Za-z0-9][A-Za-z0-9_.:/-]{0,63}$/, message: 'Dùng từ 1–64 ký tự không dấu, không có khoảng trắng.' }]}><Input autoComplete="username" spellCheck={false} /></Form.Item>
      <Form.Item name="password" label="Mật khẩu" rules={[{ required: true }, ...(setup ? [{ min: 12, message: 'Mật khẩu cần ít nhất 12 ký tự.' }] : [])]}><Input.Password autoComplete={setup ? 'new-password' : 'current-password'} maxLength={256} /></Form.Item>
      {setup && <Form.Item name="confirmAccounts" valuePropName="checked" rules={[{ validator: async (_, value: unknown) => { if (!value) throw new Error('Cần xác nhận danh mục khởi tạo.'); } }]}><Checkbox>Tôi xác nhận dùng VND và danh mục tài khoản khởi tạo cho thương mại/dịch vụ; sẽ rà soát tài khoản và cấu hình chính sách trước khi ghi sổ.</Checkbox></Form.Item>}
      {!setup && <Form.Item name="remember" valuePropName="checked"><Checkbox>Giữ đăng nhập 30 ngày</Checkbox></Form.Item>}
      {mutation.error && <Alert type="error" showIcon title={mutation.error.message} style={{ marginBottom: 20 }} />}
      <Button type="primary" block htmlType="submit" loading={mutation.isPending}>{setup ? 'Khởi tạo và đăng nhập' : 'Đăng nhập'}</Button>
    </Form><small>Dữ liệu được lưu tại máy chủ của doanh nghiệp.</small>
  </section></main>;
}
