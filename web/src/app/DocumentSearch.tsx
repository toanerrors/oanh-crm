import { useRef, useState } from 'react';
import { AutoComplete, Button, ConfigProvider, Input, Modal } from 'antd';
import type { RefSelectProps } from 'antd';
import { useNavigate } from 'react-router-dom';
import { useWorkspace } from './WorkspaceProvider';
import { filterDocuments, periodDocuments } from '../shared/accounting/ledger';
import { formatDate } from '../shared/accounting/money';
import { Icon } from '../shared/ui/Icon';
import styles from './App.module.css';

export function DocumentSearch() {
  const { data, period, openDocument } = useWorkspace();
  const [search, setSearch] = useState('');
  const [open, setOpen] = useState(false);
  const inputRef = useRef<RefSelectProps>(null);
  const keyboardSelection = useRef(false);
  const navigate = useNavigate();
  const matches = data && search.trim() ? filterDocuments(periodDocuments(data.documents, period), search, data.partners).slice(0, 6) : [];
  const options = matches.map(item => ({
    value: item.reference,
    documentId: item.id,
    label: <div className={styles.searchResult}><span><strong>{item.reference}</strong><small>{formatDate(item.date)}</small></span><span>{item.description}</span></div>,
  }));

  function findAll() {
    keyboardSelection.current = false;
    navigate(`/documents?q=${encodeURIComponent(search.trim())}`);
    setOpen(false);
    setSearch('');
  }

  function searchInput(inModal = false) {
    return <AutoComplete ref={inModal ? inputRef : undefined} className={inModal ? styles.searchModalInput : styles.desktopSearch} value={search} onChange={value => { keyboardSelection.current = false; setSearch(value); }} options={options} showSearch={{ filterOption: false }} defaultActiveFirstOption={false} onInputKeyDown={event => { if (event.key === 'ArrowDown' || event.key === 'ArrowUp') keyboardSelection.current = true; }} onSelect={(_, option) => { keyboardSelection.current = false; setOpen(false); setSearch(''); openDocument(option.documentId); }} popupRender={menu => <><div className={styles.searchHint}>Chứng từ gần đây trong kỳ</div>{menu}<Button type="text" block onMouseDown={event => event.preventDefault()} onClick={findAll}>Tìm tất cả chứng từ trong kỳ <Icon name="arrow" size={15} /></Button></>}>
      <Input className={styles.globalSearch} prefix={<Icon name="search" size={17} />} placeholder="Tìm chứng từ, đối tượng…" aria-label="Tìm chứng từ trong kỳ" allowClear suffix={inModal ? undefined : <kbd>↵</kbd>} onPressEnter={event => { if (!event.nativeEvent.isComposing && (!keyboardSelection.current || options.length === 0)) { event.preventDefault(); findAll(); } }} />
    </AutoComplete>;
  }

  return <ConfigProvider theme={{ token: { motion: false } }}>
    {searchInput()}
    <Button type="text" className={styles.mobileSearch} icon={<Icon name="search" />} aria-label="Mở tìm kiếm chứng từ" onClick={() => setOpen(true)} />
    <Modal title="Tìm chứng từ" open={open} onCancel={() => setOpen(false)} footer={null} destroyOnHidden afterOpenChange={visible => { if (visible) inputRef.current?.focus(); }}>
      <p className={styles.muted}>Tìm theo số chứng từ, diễn giải hoặc tên đối tượng trong kỳ {period.split('-').reverse().join('/')}.</p>
      {searchInput(true)}
      <Button className={styles.searchAll} type="primary" block onClick={findAll}>Xem tất cả kết quả <Icon name="arrow" size={16} /></Button>
    </Modal>
  </ConfigProvider>;
}
