import { useEffect, useState } from 'react';
import { App } from 'antd';
import { parseSidebarPins, toggleSidebarPin } from './sidebar-shortcuts';
import type { SidebarShortcut } from './sidebar-shortcuts';

function readPins(key: string, shortcuts: SidebarShortcut[]) {
  try { return parseSidebarPins(localStorage.getItem(key), shortcuts); } catch { return []; }
}

export function useSidebarPins(key: string, shortcuts: SidebarShortcut[]) {
  const { message } = App.useApp();
  const [state, setState] = useState(() => ({ key, ids: readPins(key, shortcuts) }));
  const ids = state.key === key ? state.ids.filter(id => shortcuts.some(item => item.id === id)) : [];
  useEffect(() => {
    setState({ key, ids: readPins(key, shortcuts) });
    const sync = (event: StorageEvent) => { if (event.key === key || event.key === null) setState({ key, ids: readPins(key, shortcuts) }); };
    window.addEventListener('storage', sync);
    return () => window.removeEventListener('storage', sync);
  }, [key, shortcuts]);
  function toggle(id: string) {
    if (!shortcuts.some(item => item.id === id)) return;
    const next = toggleSidebarPin(ids, id);
    setState({ key, ids: next });
    try { localStorage.setItem(key, JSON.stringify(next)); } catch { void message.warning('Trình duyệt không cho lưu tùy chọn. Ghim chỉ được giữ trong phiên này.'); }
  }
  return { ids, toggle };
}
