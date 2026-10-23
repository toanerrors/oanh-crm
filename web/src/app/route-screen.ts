export function routeScreen(to: string) {
  const path = to.split(/[?#]/, 1)[0];
  if (path === '/') return 'dashboard';
  if (/^\/vouchers\/[^/]+$/.test(path)) return 'voucher';
  if (/^\/(cash|bank|sales|purchases|inventory|documents)(\/reports)?$/.test(path)) return path.endsWith('/reports') ? 'reports' : 'documents';
  if (/^\/(sales|purchases)\/settlements$/.test(path) || path === '/settlements') return 'settlements';
  if (/^\/(sales|purchases)\/partners$/.test(path) || path === '/catalog') return 'catalog';
  if (/^\/(sales|purchases)\/items$/.test(path)) return 'items';
  if (path === '/reports' || path === '/receivables') return 'reports';
  if (path === '/automation') return 'automation';
  if (path === '/imports') return 'imports';
  if (path === '/settings' || path === '/help') return 'workspace';
  return undefined;
}
