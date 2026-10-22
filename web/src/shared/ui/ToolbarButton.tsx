import { Button, Tooltip } from 'antd';
import type { ButtonProps } from 'antd';
import { Icon } from './Icon';
import type { IconName } from './Icon';
import styles from '../../app/App.module.css';

type ToolbarButtonProps = Omit<ButtonProps, 'children' | 'icon' | 'aria-label'> & { label: string; icon: IconName };

export function ToolbarButton({ label, icon, ...props }: ToolbarButtonProps) {
  return <Tooltip title={label} trigger={['hover', 'focus']}><span className={styles.toolbarButton}><Button {...props} aria-label={label} icon={<Icon name={icon} size={17} />} /></span></Tooltip>;
}
