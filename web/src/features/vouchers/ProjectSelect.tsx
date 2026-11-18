import { useState } from 'react';
import { Button, Select } from 'antd';
import { useWorkspace } from '../../app/WorkspaceProvider';
import { apiScope, isDemo } from '../../shared/api/client';
import { filterCatalogProjects } from '../../shared/data/partner-catalog';
import type { Project } from '../../shared/data/types';
import { searchOptions } from '../../shared/ui/search';
import { Icon } from '../../shared/ui/Icon';
import { ProjectEditor } from '../catalog/ProjectEditor';

export function ProjectSelect({ id, value, onChange, onSelected, label, placeholder = 'Chọn công trình', disabled }: { id?: string; value?: string; onChange?: (id?: string) => void; onSelected?: (project?: Project) => void; label: string; placeholder?: string; disabled?: boolean }) {
  const { data } = useWorkspace();
  const [editing, setEditing] = useState<Project | null>(null);
  const canWrite = isDemo || apiScope().permissions !== 'viewer';
  const projects = data?.projects ?? [];
  const options = filterCatalogProjects(projects, '', true).map(item => ({ value: item.id, label: `${item.code} · ${item.name}` }));
  const current = value && !options.some(item => item.value === value) ? projects.find(item => item.id === value) : undefined;
  if (current) options.unshift({ value: current.id, label: `${current.code} · ${current.name}` });
  const choose = (nextId?: string) => {
    onChange?.(nextId || undefined);
    onSelected?.(projects.find(row => row.id === nextId));
  };
  return <div style={{ display: 'flex', gap: 4, minWidth: 0 }}>
    <Select id={id} style={{ flex: 1, minWidth: 0 }} allowClear value={value || undefined} popupMatchSelectWidth={340} aria-label={label} disabled={disabled} loading={!data} showSearch={searchOptions} placeholder={placeholder} options={options} notFoundContent={!data ? 'Đang tải danh mục…' : 'Chưa có công trình phù hợp. Nhấn + để thêm.'} onChange={choose} />
    {!disabled && canWrite && <Button icon={<Icon name="plus" size={14} />} aria-label={`${label}: thêm công trình`} title="Thêm công trình" onClick={() => setEditing({ id: crypto.randomUUID(), code: '', name: '', description: '', active: true })} />}
    {editing && <ProjectEditor key={editing.id} project={editing} onClose={() => setEditing(null)} onSaved={project => { if (project.active) { onChange?.(project.id); onSelected?.(project); } }} />}
  </div>;
}
