import { useEffect, useRef, useState } from 'react';
import { AutoComplete, Button, ConfigProvider, Input, Modal } from 'antd';
import type { RefSelectProps } from 'antd';
import { useLocation, useNavigate } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { useWorkspace } from './WorkspaceProvider';
import { filterDocuments, periodDocuments } from '../shared/accounting/ledger';
import { formatDate } from '../shared/accounting/money';
import { api, apiScope, isDemo } from '../shared/api/client';
import { workspaceKey } from '../shared/data/workspace';
import type { DocumentList } from '../shared/data/reports';
import { useDebouncedValue } from '../shared/ui/useDebouncedValue';
import { Icon } from '../shared/ui/Icon';
import { quickActions } from './quick-actions';
import type { SidebarShortcut } from './sidebar-shortcuts';
import { preloadRoute } from './route-preload';
import styles from './App.module.css';

export function DocumentSearch({ shortcuts, pinnedIds }: { shortcuts: SidebarShortcut[]; pinnedIds: string[] }) {
  const { data, period, openDocument, newDraft, setDraftOpen } = useWorkspace();
  const [search, setSearch] = useState('');
  const [open, setOpen] = useState(false);
  const [focused, setFocused] = useState(false);
  const [composing, setComposing] = useState(false);
  const inputRef = useRef<RefSelectProps>(null);
  const keyboardSelection = useRef(false);
  const navigate = useNavigate();
  const location = useLocation();
  const delayed = useDebouncedValue(search.trim());
  const epoch = apiScope().epoch;
  useEffect(() => { setSearch(''); setOpen(false); }, [period, epoch]);
  useEffect(() => {
    const shortcut = (event: KeyboardEvent) => {
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'k' && !event.isComposing && !event.altKey && !event.shiftKey && !event.defaultPrevented && !event.repeat) { event.preventDefault(); setOpen(value => !value); }
    };
    document.addEventListener('keydown', shortcut);
    return () => document.removeEventListener('keydown', shortcut);
  }, []);
  const query = useQuery({ queryKey: [...workspaceKey(), 'quick-search', period, delayed, data?.revision], queryFn: ({ signal }) => api<DocumentList>(`/documents?${new URLSearchParams({ period, q: delayed, page: '1', pageSize: '6', type: 'all', status: 'all', account: 'all', sort: 'date', direction: 'desc' })}`, { signal }), enabled: !isDemo && Boolean(data) && (focused || open) && !composing && search.trim() === delayed && delayed.length >= 2, retry: false });
  const local = data && search.trim() ? filterDocuments(periodDocuments(data.documents, period), search, data.partners).slice(0, 6) : [];
  const matches = search.trim() === delayed && delayed.length >= 2 && query.data ? query.data.documents : local;
  const actions = quickActions(shortcuts, pinnedIds, search, search.trim() ? 5 : 8);
  const options = [
    ...actions.map(item => ({ value: `action:${item.id}`, shortcutId: item.id, documentId: '', label: <div className={styles.quickAction} onMouseEnter={() => { if (item.to) preloadRoute(item.to); }}><Icon name={item.icon} size={18} /><span><strong>{item.label}</strong><small>{item.moduleLabel}</small></span><Icon name="arrow" size={15} /></div> })),
    ...matches.map(item => ({ value: `document:${item.id}`, shortcutId: '', documentId: item.id, label: <div className={styles.searchResult}><span><strong>{item.reference}</strong><small>{formatDate(item.date)}</small></span><span>{item.description}</span></div> })),
  ];
  function close() { setOpen(false); setSearch(''); keyboardSelection.current = false; }
  function findAll() { navigate(`/documents?q=${encodeURIComponent(search.trim())}`); close(); }
  function searchInput(inModal = false) {
    return <AutoComplete ref={inModal ? inputRef : undefined} className={inModal ? styles.searchModalInput : styles.desktopSearch} value={search} onChange={value => { keyboardSelection.current = false; setSearch(value); }} options={options} showSearch={{ filterOption: false }} defaultOpen={inModal} defaultActiveFirstOption={false} onInputKeyDown={event => { if (event.key === 'ArrowDown' || event.key === 'ArrowUp') keyboardSelection.current = true; }} onSelect={(_, option) => {
      close();
      const action = shortcuts.find(item => item.id === option.shortcutId);
      if (action) { openDocument(null); setDraftOpen(false); if (action.action === 'journal') newDraft(); else if (action.to) navigate(action.to, { state: { returnTo: location.pathname + location.search } }); }
      else if (option.documentId) openDocument(option.documentId);
    }} popupRender={menu => <><div className={styles.searchHint}>{query.isFetching ? 'Đang tìm toàn bộ chứng từ trong kỳ…' : search.trim() ? 'Chức năng và chứng từ phù hợp' : pinnedIds.length ? 'Chức năng đã ghim và truy cập nhanh' : 'Truy cập nhanh · gõ tên chức năng hoặc chứng từ'}</div>{menu}{query.error && <div role="status" className={styles.searchHint}>Chưa tải được kết quả đầy đủ. Có thể thử tìm trong danh sách.</div>}<Button type="text" block onMouseDown={event => event.preventDefault()} onClick={findAll}>Tìm tất cả chứng từ trong kỳ <Icon name="arrow" size={15} /></Button></>}>
      <Input className={styles.globalSearch} prefix={<Icon name="search" size={17} />} placeholder="Tìm chức năng, chứng từ…" aria-label="Tìm chứng từ trong kỳ" allowClear onFocus={() => setFocused(true)} onBlur={() => setFocused(false)} onCompositionStart={() => setComposing(true)} onCompositionEnd={() => setComposing(false)} suffix={inModal ? undefined : <Button type="text" size="small" aria-label="Mở tìm nhanh" onClick={() => setOpen(true)}><kbd>Ctrl/⌘ K</kbd></Button>} onPressEnter={event => { if (!event.nativeEvent.isComposing && !composing && (!keyboardSelection.current || options.length === 0)) { event.preventDefault(); findAll(); } }} />
    </AutoComplete>;
  }
  return <ConfigProvider theme={{ token: { motion: false } }}>
    {searchInput()}
    <Button type="text" className={styles.mobileSearch} icon={<Icon name="search" />} aria-label="Mở tìm kiếm chứng từ" onClick={() => setOpen(true)} />
    <Modal title="Tìm nhanh" open={open} onCancel={close} footer={null} width={640} destroyOnHidden afterOpenChange={visible => { if (visible) inputRef.current?.focus(); }}>
      <p className={styles.muted}>Gõ tên chức năng hoặc chứng từ · Kỳ {period.split('-').reverse().join('/')} · ↑↓ chọn, Enter mở, Esc đóng.</p>
      {searchInput(true)}
      <Button className={styles.searchAll} type="primary" block onClick={findAll}>Xem tất cả kết quả chứng từ <Icon name="arrow" size={16} /></Button>
    </Modal>
  </ConfigProvider>;
}
