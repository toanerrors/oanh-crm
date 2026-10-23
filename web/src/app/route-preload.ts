import { routeScreen } from './route-screen';

export const screens = {
  dashboard: () => import('../features/dashboard/Dashboard'),
  documents: () => import('../features/documents/Documents'),
  reports: () => import('../features/reports/Reports'),
  settlements: () => import('../features/documents/Settlements'),
  catalog: () => import('../features/catalog/Catalog'),
  items: () => import('../features/workspace/Automation').then(({ ItemsPage }) => ({ Component: ItemsPage })),
  voucher: () => import('../features/vouchers/VoucherForm'),
  automation: () => import('../features/workspace/Automation'),
  imports: () => import('../features/imports/ExcelImport'),
  workspace: () => import('../features/workspace/Workspace'),
};
const loaded = new Set<keyof typeof screens>();
export function preloadRoute(to: string) {
  const screen = routeScreen(to);
  const connection = (navigator as Navigator & { connection?: { saveData?: boolean; effectiveType?: string } }).connection;
  if (!screen || loaded.has(screen) || connection?.saveData || connection?.effectiveType?.includes('2g')) return;
  loaded.add(screen);
  void screens[screen]().catch(() => loaded.delete(screen));
}
