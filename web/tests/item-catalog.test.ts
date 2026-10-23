import test from 'node:test';
import assert from 'node:assert/strict';
import { filterCatalogItems } from '../src/shared/data/item-catalog.ts';

const items = [
  { id: 'rent', code: 'PDV', name: 'THUÊ NHÀ', unit: 'THÁNG', tracked: false, active: true },
  { id: 'goods', code: 'DV-actually-goods', name: 'Hàng hóa', unit: 'cái', tracked: true, active: true },
  { id: 'inactive', code: 'DV-old', name: 'Dịch vụ cũ', unit: 'lần', tracked: false, active: false },
];
test('phân loại theo danh mục, không theo tiền tố mã; cùng mã cho mua và bán', () => {
  assert.deepEqual(filterCatalogItems(items, 'service', '', true).map(i => i.id), ['rent']);
  assert.deepEqual(filterCatalogItems(items, 'goods', '', true).map(i => i.id), ['goods']);
  assert.equal(filterCatalogItems(items, 'service', '', true)[0], items[0]);
});
test('tìm không dấu và hiển thị mã ngừng dùng tại danh mục để sửa', () => {
  assert.deepEqual(filterCatalogItems(items, 'service', 'thue nha').map(i => i.id), ['rent']);
  assert.deepEqual(filterCatalogItems(items, 'service').map(i => i.id), ['rent', 'inactive']);
  assert.deepEqual(filterCatalogItems(items, 'all', 'thang').map(i => i.id), ['rent']);
});
test('đổi loại mã chưa dùng cập nhật cả hai nhóm, không thêm bản sao', () => {
  const changed = items.map(i => i.id === 'rent' ? { ...i, tracked: true } : i);
  assert.equal(filterCatalogItems(changed, 'service', '', true).length, 0);
  assert.deepEqual(filterCatalogItems(changed, 'goods', '', true).map(i => i.id), ['rent', 'goods']);
  assert.equal(changed.length, items.length);
});
