import { normalizeSearch } from '../shared/accounting/money.ts';
import type { SidebarShortcut } from './sidebar-shortcuts.ts';

export function quickActions(shortcuts: SidebarShortcut[], pinnedIds: string[], search: string, limit = 8) {
  const term = normalizeSearch(search).trim();
  const words = term.split(/[^a-z0-9]+/).filter(Boolean);
  const seen = new Set<string>();
  return [...shortcuts].sort((a, b) => {
    const rank = (item: SidebarShortcut) => { const pin = pinnedIds.indexOf(item.id); return pin >= 0 ? pin : pinnedIds.length; };
    const relevance = (item: SidebarShortcut) => { const label = normalizeSearch(item.label); return !term || label === term ? 0 : label.includes(term) ? 1 : words.every(word => label.includes(word)) ? 2 : 3; };
    return relevance(a) - relevance(b) || rank(a) - rank(b);
  }).filter(item => {
    const target = item.to ?? item.action;
    const text = normalizeSearch(`${item.label} ${item.moduleLabel} ${item.group}`);
    const tokens = text.split(/[^a-z0-9]+/).filter(Boolean);
    if (!target || seen.has(target) || !words.every((word, index) => tokens.some(token => index === words.length - 1 ? token.startsWith(word) : token === word))) return false;
    seen.add(target); return true;
  }).slice(0, limit);
}
