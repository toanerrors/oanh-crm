import { useState } from 'react';
import { App, Button, Dropdown, Tooltip } from 'antd';
import { Icon } from './Icon';
import styles from '../../app/App.module.css';

export type ExportFormat = 'xlsx' | 'csv' | 'print';
const choices = [
  { key: 'xlsx' as const, title: 'Excel (.xlsx)', detail: 'Bảng có định dạng, bộ lọc cột và dòng tổng', icon: 'book' as const },
  { key: 'csv' as const, title: 'CSV (.csv)', detail: 'UTF-8 · trao đổi dữ liệu với phần mềm khác', icon: 'file' as const },
  { key: 'print' as const, title: 'Bản in / Lưu PDF', detail: 'A4 · lưu PDF qua hộp thoại in của trình duyệt', icon: 'print' as const },
];

export function ExportMenu({ onExport, disabled, formats, onReload, text = false }: { onExport: (format: ExportFormat) => void | Promise<void>; disabled?: boolean; formats?: ExportFormat[]; onReload?: () => void; text?: boolean }) {
  const [busy, setBusy] = useState(false);
  const [open, setOpen] = useState(false);
  const { message } = App.useApp();
  const items = choices.filter(choice => !formats || formats.includes(choice.key)).map(choice => ({ key: choice.key, icon: <Icon name={choice.icon} size={18} />, label: <div className={styles.exportOption}><strong>{choice.title}</strong><small>{choice.detail}</small></div>, onClick: async () => {
    setOpen(false); setBusy(true);
    try { await onExport(choice.key); } catch (error) { onReload?.(); void message.error(error instanceof Error ? error.message : 'Không xuất được tệp. Vui lòng thử lại.'); }
    finally { setBusy(false); }
  } }));
  return <Dropdown autoFocus trigger={['click']} disabled={disabled || busy} open={open && !disabled && !busy} onOpenChange={setOpen} menu={{ items }}><span className={styles.toolbarButton}><Tooltip title="Xuất tệp · toàn bộ tập lọc" trigger={['hover', 'focus']}><Button aria-label="Xuất tệp" aria-haspopup="menu" aria-expanded={open} loading={busy} disabled={disabled || busy} icon={<Icon name="download" size={17} />} onKeyDown={event => { if (event.key === 'ArrowDown') { event.preventDefault(); setOpen(true); } }}>{text ? 'Xuất tệp' : undefined}</Button></Tooltip></span></Dropdown>;
}
