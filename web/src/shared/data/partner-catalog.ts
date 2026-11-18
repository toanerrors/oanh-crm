import { normalizeSearch } from '../accounting/money.ts';
import type { Partner, Project } from './types.ts';

export type PartnerRoleFilter = 'customer' | 'supplier' | 'all';

export const partnerRoles: Record<Partner['role'], string> = {
  customer: 'Khách hàng',
  supplier: 'Nhà cung cấp',
  both: 'Khách hàng & NCC',
};

export function filterCatalogPartners(partners: Partner[], role: PartnerRoleFilter = 'all', search = '', activeOnly = false) {
  const query = normalizeSearch(search);
  return partners.filter(item => (!activeOnly || item.active) && (role === 'all' || item.role === 'both' || item.role === role) && normalizeSearch(`${item.code} ${item.name} ${item.taxCode ?? ''} ${item.address ?? ''} ${item.email} ${item.phone}`).includes(query));
}

export function filterCatalogProjects(projects: Project[] = [], search = '', activeOnly = false) {
  const query = normalizeSearch(search);
  return projects.filter(item => (!activeOnly || item.active) && normalizeSearch(`${item.code} ${item.name} ${item.description ?? ''}`).includes(query));
}
