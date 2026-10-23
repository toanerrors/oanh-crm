import { useEffect, useRef, useState } from 'react';
import type { KeyboardEvent, PointerEvent as ReactPointerEvent } from 'react';
import { Popover, Tooltip } from 'antd';
import { Link, NavLink, useLocation } from 'react-router-dom';
import { Icon } from '../shared/ui/Icon';
import { preloadRoute } from './route-preload';
import { SidebarHover } from './sidebar-hover';
import type { SidebarItem, SidebarShortcut } from './sidebar-shortcuts';
import appStyles from './App.module.css';
import styles from './SidebarNavigation.module.css';

interface Props {
  navigation: SidebarItem[];
  shortcuts: SidebarShortcut[];
  pinnedIds: string[];
  togglePin: (id: string) => void;
  onJournal: () => void;
  onNavigate: () => void;
  compact?: boolean;
  mobile?: boolean;
  draftCount: number;
}

export function SidebarNavigation(props: Props) {
  const { navigation, shortcuts, pinnedIds, compact, mobile, onNavigate } = props;
  const [openPath, setOpenPath] = useState('');
  const location = useLocation();
  const [hover] = useState(() => new SidebarHover(path => { setOpenPath(path); if (path) preloadRoute(path); }));
  useEffect(() => { hover.show(''); }, [location.key, compact, mobile, hover]);
  useEffect(() => () => hover.cancel(), [hover]);
  useEffect(() => {
    const breakpoint = window.matchMedia('(max-width: 760px)');
    const close = () => hover.show('');
    breakpoint.addEventListener('change', close);
    return () => breakpoint.removeEventListener('change', close);
  }, [hover]);
  function close() { hover.show(''); onNavigate(); }
  const pinned = pinnedIds.flatMap(id => shortcuts.find(item => item.id === id) ?? []);
  return <>
    {pinned.length > 0 && <nav aria-label="Chức năng đã ghim" className={styles.pinnedSection}>
      <div className={appStyles.navGroup}>ĐÃ GHIM</div>
      <div className={styles.pinnedList}>{pinned.map(item => <div key={item.id} className={styles.pinnedRow}>
        <Tooltip title={compact ? `${item.label} · ${item.moduleLabel}` : undefined} placement="right" trigger={['hover', 'focus']}>
          {item.to ? <Link to={item.to} onMouseEnter={() => preloadRoute(item.to!)} onFocus={() => preloadRoute(item.to!)} state={{ returnTo: location.pathname + location.search }} className={`${appStyles.navLink} ${styles.pinnedLink}`} aria-label={`${item.label} · ${item.moduleLabel}`} onClick={close}><Icon name={item.icon} size={18} /><span className={appStyles.navLabel}>{item.label}<small>{item.moduleLabel}</small></span></Link> : <button type="button" className={`${appStyles.navLink} ${styles.pinnedLink}`} aria-label={`${item.label} · ${item.moduleLabel}`} onClick={() => { close(); props.onJournal(); }}><Icon name={item.icon} size={18} /><span className={appStyles.navLabel}>{item.label}<small>{item.moduleLabel}</small></span></button>}
        </Tooltip>
        {!compact && <PinButton item={item} pinned toggle={props.togglePin} />}
        {compact && <span className={styles.pinDot}><Icon name="star" size={8} /></span>}
      </div>)}</div>
    </nav>}
    <nav className={appStyles.navigation} aria-label="Điều hướng chính">{navigation.map(item => <div key={item.path}>
      {item.group && <div className={appStyles.navGroup}>{item.group}</div>}
      <ModuleEntry {...props} item={item} entries={shortcuts.filter(entry => entry.modulePath === item.path)} hover={hover} open={openPath === item.path} setOpen={open => { if (open) hover.show(item.path); else hover.close(item.path); }} close={close} />
    </div>)}</nav>
  </>;
}

function PinButton({ item, pinned, toggle }: { item: SidebarShortcut; pinned: boolean; toggle: (id: string) => void }) {
  const label = `${pinned ? 'Bỏ ghim' : 'Ghim'} ${item.label} · ${item.moduleLabel}`;
  return <Tooltip title={pinned ? 'Bỏ ghim' : 'Ghim chức năng'} trigger={['hover', 'focus']}><button type="button" aria-label={label} aria-pressed={pinned} className={`${styles.pinButton} ${pinned ? styles.isPinned : ''}`} onClick={() => toggle(item.id)}><Icon name="star" size={17} /></button></Tooltip>;
}

function ModuleEntry({ item, entries, open, setOpen, close, compact, mobile, draftCount, pinnedIds, togglePin, onJournal, hover }: Props & { hover: SidebarHover; item: SidebarItem; entries: SidebarShortcut[]; open: boolean; setOpen: (value: boolean) => void; close: () => void }) {
  const trigger = useRef<HTMLAnchorElement>(null);
  const panel = useRef<HTMLDivElement>(null);
  const focusOnOpen = useRef(false);
  const id = `sidebar-${mobile ? 'mobile' : 'desktop'}-${item.path.slice(1) || 'home'}`;
  useEffect(() => {
    if (!open || mobile) return;
    const dismiss = (event: PointerEvent) => { if (!panel.current?.contains(event.target as Node) && !trigger.current?.contains(event.target as Node)) setOpen(false); };
    document.addEventListener('pointerdown', dismiss, true);
    return () => document.removeEventListener('pointerdown', dismiss, true);
  }, [open, mobile, setOpen]);
  function enter(event: ReactPointerEvent) {
    if (mobile || event.pointerType === 'touch' || !entries.length) return;
    const active = document.getElementById(`sidebar-desktop-${hover.active.slice(1) || 'home'}`);
    hover.enter(item.path, { x: event.clientX, y: event.clientY }, active?.getBoundingClientRect());
  }
  function leave(event: ReactPointerEvent) {
    if (mobile || event.pointerType === 'touch') return;
    if (panel.current?.contains(document.activeElement) && document.activeElement?.matches(':focus-visible')) return;
    if (trigger.current === document.activeElement && trigger.current?.matches(':focus-visible')) return;
    hover.leave(item.path, { x: event.clientX, y: event.clientY });
  }
  function focusFirst() { panel.current?.querySelector<HTMLElement>('a, button')?.focus(); }
  function handleKey(event: KeyboardEvent) {
    if (event.key === 'Escape' && open) { event.preventDefault(); event.stopPropagation(); focusOnOpen.current = false; trigger.current?.focus(); setOpen(false); }
    if (!mobile && entries.length && ['ArrowRight', 'ArrowDown', ...(open ? ['Tab'] : [])].includes(event.key) && !event.shiftKey) {
      event.preventDefault();
      if (open) focusFirst(); else { focusOnOpen.current = true; setOpen(true); }
    }
  }
  const link = <NavLink ref={trigger} to={item.path} end={item.path === '/'} onClick={close} onKeyDown={handleKey} onPointerEnter={enter} onPointerMove={event => hover.track(item.path, { x: event.clientX, y: event.clientY })} onPointerLeave={leave} onMouseEnter={() => { if (!entries.length) preloadRoute(item.path); }} onFocus={event => { preloadRoute(item.path); if (!mobile && entries.length && (compact || event.currentTarget.matches(':focus-visible'))) setOpen(true); }} onBlur={event => { if (!mobile && !panel.current?.contains(event.relatedTarget)) setOpen(false); }} aria-expanded={entries.length && !mobile ? open : undefined} aria-controls={entries.length && !mobile && open ? id : undefined} aria-haspopup={entries.length && !mobile ? 'dialog' : undefined} className={({ isActive }) => `${appStyles.navLink} ${isActive ? appStyles.activeNav : ''}`}><Icon name={item.icon} size={19} /><span className={appStyles.navLabel}>{item.label}</span>{item.path === '/documents' && draftCount > 0 && <span className={appStyles.navCount}>{draftCount}</span>}</NavLink>;
  const content = <div ref={panel} id={id} role={mobile ? 'region' : 'dialog'} aria-label={`Chức năng ${item.label}`} className={`${styles.functionPanel} ${mobile ? styles.inlinePanel : ''}`} onPointerEnter={() => { if (!mobile) hover.keep(item.path); }} onPointerLeave={leave} onKeyDown={event => { if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); focusOnOpen.current = false; trigger.current?.focus(); setOpen(false); } }} onBlurCapture={event => { if (!event.currentTarget.contains(event.relatedTarget) && !trigger.current?.contains(event.relatedTarget)) setOpen(false); }}>
    {!mobile && <div className={styles.panelTitle}><Icon name={item.icon} size={19} /><strong>{item.label}</strong><span>Ghim bằng <Icon name="star" size={12} /></span></div>}
    {(['Nghiệp vụ', 'Danh mục', 'Báo cáo'] as const).map(group => {
      const rows = entries.filter(entry => entry.group === group);
      return rows.length > 0 && <section key={group} className={styles.functionGroup} aria-label={group}><h3>{group}</h3>{rows.map(entry => <div key={entry.id} className={styles.functionRow}>
        {entry.to ? <Link to={entry.to} onMouseEnter={() => preloadRoute(entry.to!)} onFocus={() => preloadRoute(entry.to!)} state={{ returnTo: item.path }} onClick={close}>{entry.label}</Link> : <button type="button" className={styles.actionLink} onClick={() => { close(); onJournal(); }}>{entry.label}</button>}
        <PinButton item={entry} pinned={pinnedIds.includes(entry.id)} toggle={togglePin} />
      </div>)}</section>;
    })}
  </div>;
  if (mobile) return <><div className={styles.moduleRow}>{link}{entries.length > 0 && <button type="button" className={styles.expandButton} aria-label={`Chức năng ${item.label}`} aria-expanded={open} aria-controls={open ? id : undefined} onClick={() => setOpen(!open)}><Icon name={open ? 'down' : 'chevron'} size={16} /></button>}</div>{open && content}</>;
  if (!entries.length) return <Tooltip title={compact ? item.label : undefined} placement="right" trigger={['hover', 'focus']}>{link}</Tooltip>;
  return <Popover content={content} open={open} afterOpenChange={value => { if (value && focusOnOpen.current) { focusOnOpen.current = false; focusFirst(); } }} trigger={[]} placement="rightTop" arrow={false} classNames={{ root: styles.popupRoot, container: styles.popupContainer }} styles={{ container: { padding: 0, background: '#202329' } }}>{link}</Popover>;
}
