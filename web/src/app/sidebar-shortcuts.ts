import { moduleTabs } from './module-navigation.ts';
import type { IconName } from '../shared/ui/Icon';

export interface SidebarItem {
  path: string;
  label: string;
  icon: IconName;
  group?: string;
}

export interface SidebarShortcut {
  id: string;
  modulePath: string;
  moduleLabel: string;
  label: string;
  icon: IconName;
  group: 'Nghiệp vụ' | 'Danh mục' | 'Báo cáo';
  to?: string;
  action?: 'journal';
}

export function sidebarShortcuts(navigation: SidebarItem[], role: string, demo: boolean): SidebarShortcut[] {
  const result: SidebarShortcut[] = [];
  const canWrite = role === 'owner' || role === 'accountant';
  for (const module of navigation) {
    const add = (key: string, label: string, group: SidebarShortcut['group'], to?: string, icon = module.icon, action?: 'journal') => result.push({ id: `${module.path}:${key}`, modulePath: module.path, moduleLabel: module.label, label, group, to, icon, action });
    const tabs = moduleTabs(module.path);
    for (const tab of tabs.filter(tab => tab.key !== 'reports')) {
      if (demo && ['closing', 'settlements', 'items'].includes(tab.key)) continue;
      if (!demo && !canWrite && tab.key === 'items') continue;
      add(tab.key, tab.label, ['partners', 'items'].includes(tab.key) ? 'Danh mục' : 'Nghiệp vụ', tab.to);
    }
    if (module.path === '/documents' && canWrite) add('new', 'Chứng từ nghiệp vụ khác', 'Nghiệp vụ', undefined, 'plus', 'journal');
    if (!demo && canWrite) {
      if (['/cash', '/bank'].includes(module.path)) {
        const account = module.path === '/cash' ? '111' : '1121';
        add('receipt', module.path === '/cash' ? 'Lập phiếu thu' : 'Thu tiền', 'Nghiệp vụ', `/vouchers/new?kind=receipt&account=${account}`, 'plus');
        add('payment', module.path === '/cash' ? 'Lập phiếu chi' : 'Chi tiền', 'Nghiệp vụ', `/vouchers/new?kind=payment&account=${account}`, 'plus');
        add('transfer', 'Chuyển tiền nội bộ', 'Nghiệp vụ', `/vouchers/new?kind=transfer&account=${account}`, 'transfer');
      }
      if (['/sales', '/purchases'].includes(module.path)) {
        const kind = module.path === '/sales' ? 'sale' : 'purchase';
        add('goods', module.path === '/sales' ? 'Chứng từ bán hàng' : 'Chứng từ mua hàng', 'Nghiệp vụ', `/vouchers/new?kind=${kind}`, 'plus');
        add('service', module.path === '/sales' ? 'Bán dịch vụ' : 'Mua dịch vụ', 'Nghiệp vụ', `/vouchers/new?kind=${kind}&service=1`, 'file');
      }
      if (module.path === '/documents' && role === 'owner') add('new-closing', 'Lập kết chuyển lãi lỗ', 'Nghiệp vụ', '/vouchers/new?kind=closing', 'transfer');
      if (module.path === '/automation') {
        for (const [kind, label] of [['payroll', 'Tính lương, bảo hiểm & TNCN'], ['asset', 'Ghi tăng tài sản / chi phí trả trước'], ['depreciation', 'Khấu hao / phân bổ theo kỳ'], ['costing', 'Giá thành dịch vụ'], ['corporate_tax', 'Tạm tính thuế TNDN']] as const) add(kind, label, 'Nghiệp vụ', `/automation?kind=${kind}`);
        add('items', 'Hàng hóa, dịch vụ', 'Danh mục', '/automation?tab=items', 'box');
      }
    }
    if (module.path === '/receivables') {
      add('receivable', 'Phải thu khách hàng', 'Báo cáo', '/receivables?debt=receivable');
      add('payable', 'Phải trả nhà cung cấp', 'Báo cáo', '/receivables?debt=payable');
      if (!demo) {
        add('sale', 'Công nợ chứng từ bán hàng', 'Nghiệp vụ', '/settlements?kind=sale');
        add('purchase', 'Công nợ chứng từ mua hàng', 'Nghiệp vụ', '/settlements?kind=purchase');
      }
    }
    if (tabs.length || module.path === '/reports') {
      const path = tabs.find(tab => tab.key === 'reports')?.to ?? '/reports';
      for (const [tab, label] of [['journal', 'Nhật ký chung'], ['ledger', 'Sổ cái'], ['trial', 'Cân đối phát sinh'], ...(!demo ? [['financial', 'Báo cáo tài chính']] : [])]) add(`report-${tab}`, label, 'Báo cáo', `${path}?tab=${tab}`, 'chart');
    }
    if (module.path === '/catalog') add('partners', 'Danh mục đối tượng & tài khoản', 'Danh mục', '/catalog');
  }
  return result;
}

export function sidebarPinsKey(companyId: string, actor: string) {
  return `oanh.sidebar.pins:${JSON.stringify([companyId, actor])}`;
}

export function parseSidebarPins(raw: string | null, shortcuts: SidebarShortcut[]): string[] {
  try {
    const value: unknown = JSON.parse(raw ?? '[]');
    if (!Array.isArray(value)) return [];
    const allowed = new Set(shortcuts.map(item => item.id));
    return [...new Set(value.filter((id): id is string => typeof id === 'string' && allowed.has(id)))];
  } catch { return []; }
}

export function toggleSidebarPin(ids: string[], id: string) {
  return ids.includes(id) ? ids.filter(value => value !== id) : [...ids, id];
}
