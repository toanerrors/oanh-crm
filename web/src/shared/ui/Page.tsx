import { Alert, Button, Empty, Skeleton, Tag } from 'antd';
import type { ReactNode } from 'react';
import { useWorkspace } from '../../app/WorkspaceProvider';
import { Icon } from './Icon';
import styles from '../../app/App.module.css';
import { isDemo } from '../api/client';

export function PageHeading({ title, description, actions }: { title: string; description: string; actions?: ReactNode }) {
  return <div className={styles.pageHeading}><div><h1>{title}</h1><p>{description}</p></div><div className={styles.headingActions}>{actions}</div></div>;
}

export function DataBoundary({ children }: { children: ReactNode }) {
  const { isPending, error, refresh } = useWorkspace();
  if (isPending) return <div className={styles.panel}><Skeleton active paragraph={{ rows: 8 }} /></div>;
  if (error) return <Alert type="error" title="Không tải được dữ liệu" description={error.message} action={<Button onClick={refresh}>Thử lại</Button>} />;
  return children;
}

export function StatusTag({ status }: { status: 'posted' | 'draft' }) {
  return <Tag variant="filled" className={`${styles.statusTag} ${status === 'posted' ? styles.postedStatus : styles.draftStatus}`}><span className={status === 'posted' ? styles.greenDot : styles.amberDot} />{status === 'posted' ? 'Đã ghi sổ' : 'Bản nháp'}</Tag>;
}

export function EmptyData({ description = 'Chưa có chứng từ trong phạm vi đã chọn', action }: { description?: string; action?: ReactNode }) {
  return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={description}>{action}</Empty>;
}

export function ScopeNote() {
  const { data, period, isFetching } = useWorkspace();
  return <div className={styles.scopeNote}><span><Icon name="shield" size={14} />{isDemo ? 'Dữ liệu mẫu' : 'Dữ liệu doanh nghiệp'} · Kỳ {period.split('-').reverse().join('/')} · Phiên bản {data?.revision ?? '—'}</span><span>{isFetching ? 'Đang cập nhật…' : 'Đơn vị: VND'}</span></div>;
}
