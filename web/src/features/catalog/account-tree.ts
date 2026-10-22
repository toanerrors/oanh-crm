import type { Account } from '../../shared/data/types';
import { normalizeSearch } from '../../shared/accounting/money.ts';

export interface AccountNode extends Account { level: number; childCount: number; path: string; children?: AccountNode[] }
export function accountDescendants(accounts: Account[], id: string): Set<string> {
  const branch = new Set([id]);
  let changed = true;
  while (changed) { changed = false; for (const account of accounts) if (account.parentId && branch.has(account.parentId) && !branch.has(account.id)) { branch.add(account.id); changed = true; } }
  return branch;
}
export function accountTree(accounts: Account[], search = ''): AccountNode[] {
  const nodes = new Map(accounts.map(account => [account.id, { ...account, level: 1, childCount: 0, path: `${account.code} · ${account.name}` } as AccountNode]));
  const roots: AccountNode[] = [];
  for (const node of nodes.values()) {
    const chain: AccountNode[] = [];
    const seen = new Set([node.id]);
    let parent = nodes.get(node.parentId ?? '');
    while (parent && !seen.has(parent.id)) { seen.add(parent.id); chain.unshift(parent); parent = nodes.get(parent.parentId ?? ''); }
    const direct = nodes.get(node.parentId ?? '');
    if (!parent && direct) { (direct.children ??= []).push(node); node.level = chain.length + 1; node.path = [...chain, node].map(item => `${item.code} · ${item.name}`).join(' / '); }
    else roots.push(node);
  }
  for (const node of nodes.values()) node.childCount = node.children?.length ?? 0;
  const key = normalizeSearch(search);
  const select = (items: AccountNode[]): AccountNode[] => items.flatMap(node => {
    const children = node.children ? select(node.children) : [];
    if (key && !normalizeSearch(`${node.code} ${node.name}`).includes(key) && !children.length) return [];
    return [{ ...node, children: children.length ? children : undefined }];
  });
  return select(roots);
}
export function accountBranchKeys(nodes: AccountNode[]): string[] { return nodes.flatMap(node => node.children ? [node.id, ...accountBranchKeys(node.children)] : []); }
