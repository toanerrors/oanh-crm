import assert from 'node:assert/strict';
import test from 'node:test';
import { samePaginationScope } from '../src/shared/data/pagination-scope.ts';
import { documentListState, voucherReturnPath } from '../src/features/documents/list-state.ts';
import { routeScreen } from '../src/app/route-screen.ts';
import { quickActions } from '../src/app/quick-actions.ts';
import { sidebarShortcuts } from '../src/app/sidebar-shortcuts.ts';
import { editorShortcut } from '../src/features/vouchers/editor-shortcut.ts';

const query = (change: Record<string, string> = {}) => ['workspace', 'actor', 'owner', 'company', 'epoch', 'documents', new URLSearchParams({ period: '2026-10', page: '1', pageSize: '20', q: 'phiếu', account: '111', bank: 'b1', type: 'receipt', status: 'posted', source: '', sort: 'date', direction: 'desc', revision: '5', ...change }).toString()];

test('giữ trang trước chỉ khi đúng phạm vi; đổi kỳ, quyền, dữ liệu hoặc bộ lọc phải bỏ placeholder', () => {
  assert.equal(samePaginationScope(query({ page: '2' }), query()), true);
  const reordered = [...query()]; reordered[6] = new URLSearchParams([...new URLSearchParams(reordered[6])].reverse()).toString();
  assert.equal(samePaginationScope(reordered, query()), true);
  for (const key of ['period', 'pageSize', 'q', 'account', 'bank', 'type', 'status', 'source', 'sort', 'direction', 'revision']) assert.equal(samePaginationScope(query({ [key]: 'changed' }), query()), false, key);
  for (let index = 0; index < 6; index++) { const changed = [...query()]; changed[index] += '-new'; assert.equal(samePaginationScope(changed, query()), false); }
  assert.equal(samePaginationScope(query()), false);
  assert.equal(samePaginationScope(['workspace', {}], ['workspace', {}]), false);
});

test('danh sách khôi phục phân trang, sort, loại và trạng thái từ URL có kiểm tra', () => {
  assert.deepEqual(documentListState(new URLSearchParams('page=2&pageSize=50&type=receipt&status=draft&sort=amount&direction=asc'), false), { page: 2, pageSize: 50, type: 'receipt', status: 'draft', sort: 'amount', direction: 'asc' });
  for (const page of ['-1', '0', '1.5', 'NaN', '9007199254740992']) assert.equal(documentListState(new URLSearchParams({ page, pageSize: '999', type: 'bad', status: 'bad' }), false).page, 1);
  assert.equal(documentListState(new URLSearchParams(), true).pageSize, 8);
  assert.equal(documentListState(new URLSearchParams('pageSize=8'), false).pageSize, 20);
});

test('đóng phiếu quay lại đúng bộ lọc, từ chối địa chỉ ngoài ứng dụng và vòng lặp phiếu', () => {
  for (const path of ['/', '/cash?q=a&page=3&status=draft', '/bank?bank=b1', '/purchases/reports?tab=trial', '/sales/partners?q=a', '/catalog']) assert.equal(voucherReturnPath(path, '/cash'), path);
  for (const path of [null, {}, 'https://example.test', '//example.test', '/\\example.test', '/vouchers/new', '/vouchers/id', '/unknown', '/bank/partners']) assert.equal(voucherReturnPath(path, '/cash'), '/cash');
});

const navigation = ['/cash', '/bank', '/sales', '/purchases', '/documents', '/reports'].map(path => ({ path, label: path, icon: 'book' as const }));
const owner = sidebarShortcuts(navigation, 'owner', false);
test('tìm nhanh hỗ trợ tiếng Việt không dấu, nhiều từ và ưu tiên chức năng ghim', () => {
  assert.equal(quickActions(owner, ['/bank:payment', '/cash:receipt'], '')[0].id, '/bank:payment');
  assert.equal(quickActions(owner, ['/bank:payment', '/cash:receipt'], '')[1].id, '/cash:receipt');
  assert.equal(quickActions(owner, [], 'chi tien')[0].id, '/bank:payment');
  assert.ok(quickActions(owner, [], 'chi tien').every(item => !item.id.includes('report-financial')));
  assert.ok(quickActions(owner, [], 'bao cao tai chinh').length > 0);
  assert.equal(quickActions(owner, [], 'khongchucnang').length, 0);
  const duplicated = [...owner, ...owner]; const result = quickActions(duplicated, [], '', 100);
  assert.equal(result.length, new Set(result.map(item => item.to ?? item.action)).size);
});

test('tìm nhanh dùng đúng quyền; pin cũ không khôi phục thao tác tạo cho viewer', () => {
  const viewer = quickActions(sidebarShortcuts(navigation, 'viewer', false), ['/bank:payment'], '', 100);
  assert.ok(viewer.length > 0);
  assert.ok(viewer.every(item => !item.action && !item.to?.startsWith('/vouchers/')));
});

test('tải trước đúng chunk màn hình, không nhận route lạ', () => {
  for (const [path, screen] of [['/', 'dashboard'], ['/bank?bank=x', 'documents'], ['/cash/reports?tab=trial', 'reports'], ['/sales/items', 'items'], ['/purchases/partners', 'catalog'], ['/sales/settlements', 'settlements'], ['/vouchers/new?kind=receipt', 'voucher'], ['/vouchers/abc', 'voucher'], ['/imports', 'imports'], ['/automation', 'automation'], ['/settings', 'workspace'], ['/help', 'workspace']]) assert.equal(routeScreen(path), screen);
  for (const path of ['/unknown', '//example.test', 'https://example.test', '/bank/items', '/vouchers/abc/other']) assert.equal(routeScreen(path), undefined);
});

test('phím tắt chỉ lưu nháp/xem trước, không ghi sổ và không ngắt IME', () => {
  const key = { key: 's', ctrlKey: true, metaKey: false, altKey: false, shiftKey: false, isComposing: false, repeat: false };
  assert.equal(editorShortcut(key), 'save');
  assert.equal(editorShortcut({ ...key, key: 'S', ctrlKey: false, metaKey: true }), 'save');
  assert.equal(editorShortcut({ ...key, key: 'Enter' }), 'preview');
  for (const field of ['altKey', 'shiftKey', 'isComposing', 'repeat']) assert.equal(editorShortcut({ ...key, [field]: true }), undefined);
  assert.equal(editorShortcut({ ...key, ctrlKey: false }), undefined);
  assert.equal(editorShortcut({ ...key, key: 'p' }), undefined);
});
