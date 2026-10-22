import { useState } from 'react';
import { Button, Dropdown, Space } from 'antd';
import type { MenuProps } from 'antd';
import { Icon } from './Icon';
import type { IconName } from './Icon';
import styles from '../../app/App.module.css';

export function ToolbarActionMenu({ label, icon, onClick, menu }: { label: string; icon: IconName; onClick: () => void; menu: MenuProps }) {
  const [open, setOpen] = useState(false);
  return <Space.Compact className={styles.toolbarActionMenu} role="group" aria-label={`${label} và chức năng liên quan`}>
    <Button type="primary" onClick={onClick} icon={<Icon name={icon} size={17} />}>{label}</Button>
    <Dropdown open={open} onOpenChange={setOpen} autoFocus menu={{ ...menu, onClick: info => { menu.onClick?.(info); setOpen(false); } }} trigger={['click']} placement="bottomRight" classNames={{ root: styles.toolbarActionDropdown }}>
      <Button type="primary" aria-label={`Tùy chọn ${label}`} aria-haspopup="menu" aria-expanded={open} icon={<Icon name="down" size={16} />} />
    </Dropdown>
  </Space.Compact>;
}
