import { normalizeSearch } from '../accounting/money.ts';
import type { Partner } from './types.ts';

export type PartnerRoleFilter = 'customer' | 'supplier' | 'all';

export const partnerRoles: Record<Partner['role'], string> = {
  customer: 'Khách hàng',
  supplier: 'Nhà cung cấp',
  both: 'Khách hàng & NCC',
};

export function filterCatalogPartners(partners: Partner[], role: PartnerRoleFilter = 'all', search = '', activeOnly = false) {
  const query = normalizeSearch(search);
  return partners.filter(item => (!activeOnly || item.active) && (role === 'all' || item.role === 'both' || item.role === role) && normalizeSearch(`${item.code} ${item.name} ${item.email} ${item.phone}`).includes(query));
}
