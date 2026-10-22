import assert from 'node:assert/strict';
import test from 'node:test';
import { parseSidebarPins, sidebarPinsKey, sidebarShortcuts, toggleSidebarPin } from '../src/app/sidebar-shortcuts.ts';
import type { SidebarItem } from '../src/app/sidebar-shortcuts.ts';

const navigation: SidebarItem[] = ['/cash', '/bank', '/sales', '/purchases', '/documents', '/inventory', '/receivables', '/reports', '/catalog', '/automation'].map(path => ({ path, label: path, icon: 'book' }));
const owner = sidebarShortcuts(navigation, 'owner', false);

test('lối tắt giữ báo cáo/danh mục/công nợ trong phân hệ nguồn và tài khoản tiền đúng', () => {
  assert.equal(new Set(owner.map(item => item.id)).size, owner.length);
  for (const path of ['/cash', '/bank', '/sales', '/purchases', '/documents', '/inventory']) {
    assert.equal(owner.find(item => item.id === `${path}:report-financial`)?.to, `${path}/reports?tab=financial`);
  }
  assert.equal(owner.find(item => item.id === '/sales:partners')?.to, '/sales/partners');
  assert.equal(owner.find(item => item.id === '/purchases:settlements')?.to, '/purchases/settlements');
  assert.equal(owner.find(item => item.id === '/cash:receipt')?.to, '/vouchers/new?kind=receipt&account=111');
  assert.equal(owner.find(item => item.id === '/bank:payment')?.to, '/vouchers/new?kind=payment&account=1121');
  assert.equal(owner.find(item => item.id === '/sales:service')?.to, '/vouchers/new?kind=sale&service=1');
  assert.equal(owner.find(item => item.id === '/documents:new')?.action, 'journal');
  assert.equal(owner.find(item => item.id === '/receivables:payable')?.to, '/receivables?debt=payable');
});

test('lối tắt theo quyền: viewer không có thao tác tạo/nghiệp vụ, chỉ owner lập kết chuyển', () => {
  for (const role of ['viewer', 'unknown']) {
    const items = sidebarShortcuts(navigation, role, false);
    assert.ok(items.some(item => item.id === '/reports:report-financial'));
    assert.ok(items.every(item => !item.action && !item.to?.startsWith('/vouchers/') && !item.to?.startsWith('/automation') && !item.to?.endsWith('/items')));
  }
  const accountant = sidebarShortcuts(navigation, 'accountant', false);
  assert.ok(accountant.some(item => item.id === '/bank:receipt'));
  assert.ok(!accountant.some(item => item.id === '/documents:new-closing'));
  assert.ok(owner.some(item => item.id === '/documents:new-closing'));
});

test('mẫu chỉ cung cấp chức năng đã chạy trong mẫu', () => {
  const demo = sidebarShortcuts(navigation.filter(item => item.path !== '/automation'), 'owner', true);
  assert.ok(demo.some(item => item.action === 'journal'));
  assert.ok(demo.every(item => !item.to?.startsWith('/vouchers/') && !item.to?.includes('financial') && !item.to?.includes('settlements') && !item.to?.endsWith('/items')));
});

test('ghim chỉ nhận ID cho phép, loại trùng và từ chối JSON sai hoặc lối tắt vượt quyền', () => {
  const raw = JSON.stringify(['/bank:receipt', '/reports:report-financial', '/bank:receipt', 'https://invalid.test', null, {}, 1]);
  assert.deepEqual(parseSidebarPins(raw, owner), ['/bank:receipt', '/reports:report-financial']);
  assert.deepEqual(parseSidebarPins(raw, sidebarShortcuts(navigation, 'viewer', false)), ['/reports:report-financial']);
  for (const value of [null, '{bad', '{}', 'false', 'null', '"/bank:receipt"']) assert.deepEqual(parseSidebarPins(value, owner), []);
  assert.deepEqual(toggleSidebarPin(['/bank:receipt'], '/bank:payment'), ['/bank:receipt', '/bank:payment']);
  assert.deepEqual(toggleSidebarPin(['/bank:receipt', '/bank:payment'], '/bank:receipt'), ['/bank:payment']);
});

test('tùy chọn ghim tách theo công ty/người dùng, không nhập nhằng dấu phân cách', () => {
  assert.notEqual(sidebarPinsKey('company-a', 'actor'), sidebarPinsKey('company-b', 'actor'));
  assert.notEqual(sidebarPinsKey('company', 'actor-a'), sidebarPinsKey('company', 'actor-b'));
  assert.notEqual(sidebarPinsKey('a:b', 'c'), sidebarPinsKey('a', 'b:c'));
});
