import { normalizeSearch } from '../accounting/money.ts';
import type { Item } from './operations.ts';

export type ItemKind = 'goods' | 'service';
export function filterCatalogItems(items: Item[], kind: ItemKind | 'all', search = '', activeOnly = false) {
  const query = normalizeSearch(search);
  return items.filter(item => (!activeOnly || item.active) && (kind === 'all' || item.tracked === (kind === 'goods')) && normalizeSearch(`${item.code} ${item.name} ${item.unit}`).includes(query));
}
