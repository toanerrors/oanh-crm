import { queryOptions } from '@tanstack/react-query';
import { validateDraft } from '../accounting/ledger.ts';
import { demoScope, initialWorkspace } from './demo.ts';
import type { AccountingDocument, Partner, Workspace } from './types.ts';
import { api, apiScope, command, isDemo } from '../api/client.ts';

let workspace = structuredClone(initialWorkspace);

export const workspaceKey = () => {
  const scope = isDemo ? demoScope : apiScope();
  return [isDemo ? 'demo-workspace' : 'workspace', scope.actor, scope.permissions, scope.companyId, scope.epoch] as const;
};

export function workspaceOptions(period: string) {
  return queryOptions({
    queryKey: [...workspaceKey(), period],
    queryFn: async ({ signal }): Promise<Workspace> => {
      signal.throwIfAborted();
      if (!isDemo) return api<Workspace>(`/workspace?period=${encodeURIComponent(period)}`, { signal });
      return structuredClone(workspace);
    },
    staleTime: 0,
    retry: false,
  });
}

export async function saveDemoDraft(document: AccountingDocument): Promise<AccountingDocument> {
  if (!isDemo) {
    const { datasetEpoch, ...payload } = document;
    return command<AccountingDocument>('/documents', payload, datasetEpoch ?? apiScope().epoch);
  }
  const existing = workspace.documents.find(item => item.id === document.id);
  if (existing?.status === 'posted') throw new Error('Chứng từ đã ghi sổ không thể sửa.');
  if (existing && existing.version !== document.version) throw new Error('Bản nháp đã thay đổi. Đóng biểu mẫu và tải lại trước khi sửa.');
  const errors = validateDraft(document, workspace);
  if (errors.length) throw new Error(errors.join('\n'));
  const saved = structuredClone({ ...document, status: 'draft' as const, version: (existing?.version ?? 0) + 1 });
  workspace = { ...workspace, revision: workspace.revision + 1, documents: existing ? workspace.documents.map(item => item.id === saved.id ? saved : item) : [...workspace.documents, saved] };
  return structuredClone(saved);
}

export async function saveDemoPartners(partners: Partner[]): Promise<void> {
  if (!isDemo) { await command('/partners', { partners }); return; }
  const ids = new Set<string>();
  const codes = new Set<string>();
  for (const partner of partners) {
    const code = partner.code.trim().toUpperCase();
    const existing = workspace.partners.find(item => item.id === partner.id);
    const referenced = workspace.documents.some(item => item.partnerId === partner.id || item.lines.some(line => line.partnerId === partner.id));
    if (!code || !partner.name.trim()) throw new Error('Mã và tên đối tượng không được để trống.');
    if (existing && existing.code !== code) throw new Error('Không được đổi mã đối tượng đã tạo.');
    if (existing && referenced && (existing.name !== partner.name.trim() || existing.role !== partner.role)) throw new Error('Giữ nguyên tên và vai trò đối tượng đã có chứng từ.');
    if (ids.has(partner.id) || codes.has(code) || workspace.partners.some(item => item.code.toUpperCase() === code && item.id !== partner.id)) throw new Error(`Mã đối tượng ${code} bị trùng.`);
    if (!['customer', 'supplier', 'both'].includes(partner.role)) throw new Error('Vai trò đối tượng không hợp lệ.');
    ids.add(partner.id);
    codes.add(code);
  }
  const normalized = partners.map(item => ({ ...item, code: item.code.trim().toUpperCase(), name: item.name.trim().normalize('NFC') }));
  workspace = { ...workspace, revision: workspace.revision + 1, partners: [...workspace.partners.filter(item => !ids.has(item.id)), ...structuredClone(normalized)] };
}
