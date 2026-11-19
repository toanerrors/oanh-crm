import { normalizeSearch } from '../accounting/money.ts';
import type { Partner, Project } from './types.ts';

export type PartnerRoleFilter = 'customer' | 'supplier' | 'all';

const CATALOG_CODE_PATTERN = /^[A-Za-z0-9](?:[A-Za-z0-9 _.:/-]{0,98}[A-Za-z0-9_.:/-])?$/;

export const CATALOG_CODE_MESSAGE = 'Dùng chữ không dấu, chữ số, khoảng trắng đơn và ký tự _ . : / - (bắt đầu bằng chữ/số, tối đa 100 ký tự)';

export function isValidCatalogCode(value: string): boolean {
  const trimmed = value.trim();
  return CATALOG_CODE_PATTERN.test(trimmed) && !trimmed.includes('  ');
}

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
