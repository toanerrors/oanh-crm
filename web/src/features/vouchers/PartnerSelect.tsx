import { useState } from 'react';
import { Button, Select } from 'antd';
import { useWorkspace } from '../../app/WorkspaceProvider';
import { apiScope, isDemo } from '../../shared/api/client';
import { filterCatalogPartners } from '../../shared/data/partner-catalog';
import type { PartnerRoleFilter } from '../../shared/data/partner-catalog';
import type { Partner } from '../../shared/data/types';
import { searchOptions } from '../../shared/ui/search';
import { Icon } from '../../shared/ui/Icon';
import { PartnerEditor } from '../catalog/PartnerEditor';

export function PartnerSelect({ id, value, onChange, onSelected, role = 'all', defaultRole, label, placeholder, disabled }: { id?: string; value?: string; onChange?: (id?: string) => void; onSelected?: (partner?: Partner) => void; role?: PartnerRoleFilter; defaultRole?: Partner['role']; label: string; placeholder?: string; disabled?: boolean }) {
  const { data } = useWorkspace();
  const [editing, setEditing] = useState<Partner | null>(null);
  const canWrite = isDemo || apiScope().permissions !== 'viewer';
  const name = role === 'customer' ? 'khách hàng' : role === 'supplier' ? 'nhà cung cấp' : 'đối tượng';
  const partners = data?.partners ?? [];
  const options = filterCatalogPartners(partners, role, '', true).map(item => ({ value: item.id, label: `${item.code} · ${item.name}` }));
  const current = value && !options.some(item => item.value === value) ? partners.find(item => item.id === value) : undefined;
  if (current) options.unshift({ value: current.id, label: `${current.code} · ${current.name}` });
  const choose = (nextId?: string) => {
    onChange?.(nextId);
    onSelected?.(partners.find(row => row.id === nextId));
  };
  return <div style={{ display: 'flex', gap: 4, minWidth: 0 }}>
    <Select id={id} style={{ flex: 1, minWidth: 0 }} allowClear value={value} aria-label={label} disabled={disabled} loading={!data} showSearch={searchOptions} placeholder={placeholder ?? `Chọn ${name}`} options={options} notFoundContent={!data ? 'Đang tải danh mục…' : `Không tìm thấy ${name} phù hợp. Kiểm tra vai trò hoặc nhấn + để thêm.`} onChange={choose} />
    {!disabled && canWrite && <Button icon={<Icon name="plus" size={14} />} aria-label={`${label}: thêm ${name}`} title={`Thêm ${name}`} onClick={() => setEditing({ id: crypto.randomUUID(), code: '', name: '', role: defaultRole ?? (role === 'supplier' ? 'supplier' : 'customer'), email: '', phone: '', active: true })} />}
    {editing && <PartnerEditor key={editing.id} partner={editing} allowedRole={role} onClose={() => setEditing(null)} onSaved={partner => { if (partner.active) { onChange?.(partner.id); onSelected?.(partner); } }} />}
  </div>;
}
