export const accountingModules = [
  { path: '/documents', title: 'Kế toán tổng hợp' },
  { path: '/sales', title: 'Bán hàng', tradeKind: 'sale' },
  { path: '/purchases', title: 'Mua hàng', tradeKind: 'purchase' },
  { path: '/cash', title: 'Tiền mặt' },
  { path: '/bank', title: 'Tiền gửi' },
  { path: '/inventory', title: 'Hàng tồn kho' },
] as const;

export function accountingModule(pathname: string) {
  return accountingModules.find(module => pathname === module.path || pathname.startsWith(`${module.path}/`));
}

export function moduleTabs(pathname: string) {
  const module = accountingModule(pathname);
  if (!module) return [];
  return [
    { key: 'documents', label: ['/cash', '/bank'].includes(module.path) ? 'Thu, chi tiền' : module.title, to: module.path },
    ...(module.path === '/documents' ? [{ key: 'closing', label: 'Kết chuyển lãi lỗ', to: `${module.path}?source=closing` }] : []),
    ...('tradeKind' in module ? [
      { key: 'settlements', label: 'Công nợ theo chứng từ', to: `${module.path}/settlements` },
      { key: 'partners', label: module.tradeKind === 'sale' ? 'Khách hàng' : 'Nhà cung cấp', to: `${module.path}/partners` },
      { key: 'items', label: 'Hàng hóa, dịch vụ', to: `${module.path}/items` },
    ] : []),
    { key: 'reports', label: 'Báo cáo', to: `${module.path}/reports` },
  ];
}

export function activeModuleTab(pathname: string, search: string) {
  const module = accountingModule(pathname);
  if (!module) return '';
  if (pathname === module.path || pathname === `${module.path}/`) return module.path === '/documents' && new URLSearchParams(search).get('source') === 'closing' ? 'closing' : 'documents';
  return pathname.slice(module.path.length + 1).split('/')[0];
}
