import { useId, useState } from 'react';
import { Alert, Button, Empty, Skeleton, Tag } from 'antd';
import type { ReactNode } from 'react';
import { useWorkspace } from '../../app/WorkspaceProvider';
import { Icon } from './Icon';
import styles from '../../app/App.module.css';

export function PageHeading({ title, description }: { title: string; description: string }) {
  return <div className={styles.visuallyHidden}><h1>{title}</h1><p>{description}</p></div>;
}

export function PageToolbar({ children, actions, label = 'Công cụ bảng', actionsLabel, activeFilters = 0, onResetFilters }: { children?: ReactNode; actions?: ReactNode; label?: string; actionsLabel?: string; activeFilters?: number; onResetFilters?: () => void }) {
  const [filtersOpen, setFiltersOpen] = useState(false);
  const filterId = useId();
  return <div className={styles.tableToolbar} role="group" aria-label={label}>
    {children && <div className={styles.mobileFilterBar}>
      <Button
        className={`${styles.mobileFilterToggle} ${filtersOpen || activeFilters > 0 ? styles.mobileFilterActive : ''}`}
        icon={<Icon name="filter" size={14} />}
        aria-expanded={filtersOpen}
        aria-controls={filterId}
        onClick={() => setFiltersOpen(value => !value)}
      >
        Bộ lọc{activeFilters > 0 ? <span className={styles.filterBadge}>{activeFilters}</span> : null}
      </Button>
    </div>}
    {children && <div id={filterId} className={`${styles.toolbarFilters} ${filtersOpen ? styles.toolbarFiltersOpen : ''}`}>
      {children}
      {activeFilters > 0 && onResetFilters && <Button type="link" size="small" className={styles.mobileFilterClear} onClick={onResetFilters}>Xóa bộ lọc</Button>}
    </div>}
    {actions && <div className={styles.toolbarActions} role={actionsLabel ? 'group' : undefined} aria-label={actionsLabel}>{actions}</div>}
  </div>;
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
