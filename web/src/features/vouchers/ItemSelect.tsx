import { useState } from 'react';
import { Button, Select } from 'antd';
import { useQuery } from '@tanstack/react-query';
import { apiScope } from '../../shared/api/client';
import { automationOptions } from '../../shared/data/operations';
import type { Item } from '../../shared/data/operations';
import { filterCatalogItems } from '../../shared/data/item-catalog';
import type { ItemKind } from '../../shared/data/item-catalog';
import { searchOptions } from '../../shared/ui/search';
import { Icon } from '../../shared/ui/Icon';
import { ItemEditor } from '../catalog/ItemEditor';

export function ItemSelect({ value, onChange, onSelected, kind, label, disabled }: { value?: string; onChange?: (id: string) => void; onSelected?: (item: Item) => void; kind: ItemKind; label: string; disabled?: boolean }) {
  const query = useQuery({ ...automationOptions(), enabled: apiScope().permissions !== 'viewer' });
  const [editing, setEditing] = useState<Item | null>(null);
  const name = kind === 'service' ? 'dịch vụ' : 'hàng hóa';
  const choose = (id: string) => { onChange?.(id); const item = query.data?.items.find(row => row.id === id); if (item) onSelected?.(item); };
  return <div style={{ display: 'flex', gap: 4, minWidth: 0 }}>
    <Select style={{ flex: 1, minWidth: 0 }} value={value} aria-label={label} disabled={disabled} loading={query.isFetching} showSearch={searchOptions} placeholder={`Chọn ${name}`} options={filterCatalogItems(query.data?.items ?? [], kind, '', true).map(item => ({ value: item.id, label: `${item.code} · ${item.name}` }))} notFoundContent={query.isPending ? 'Đang tải danh mục…' : query.error ? 'Không tải được danh mục. Thử lại sau.' : `Không tìm thấy ${name} phù hợp. Kiểm tra loại danh mục hoặc nhấn + để thêm.`} onChange={choose} />
    {!disabled && <Button icon={<Icon name="plus" size={14} />} aria-label={`${label}: thêm ${name}`} title={`Thêm ${name}`} onClick={() => setEditing({ id: crypto.randomUUID(), code: '', name: '', unit: '', tracked: kind === 'goods', active: true })} />}
    {editing && <ItemEditor key={editing.id} item={editing} items={query.data?.items ?? []} onClose={() => setEditing(null)} onSaved={item => { onChange?.(item.id); onSelected?.(item); }} />}
  </div>;
}
