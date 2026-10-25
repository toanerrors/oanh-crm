import test from 'node:test';
import assert from 'node:assert/strict';
import { filterCatalogPartners } from '../src/shared/data/partner-catalog.ts';
import type { Partner } from '../src/shared/data/types.ts';

const partners: Partner[] = [
  { id: 'c1', code: 'KH001', name: 'Công ty TNHH Ánh Dương', role: 'customer', email: 'anhduong@example.invalid', phone: '090111222', active: true },
  { id: 's1', code: 'NCC001', name: 'Nhà cung cấp Vật Tư', role: 'supplier', email: '', phone: '090333444', active: true },
  { id: 'b1', code: 'DT001', name: 'Đối tác Song Hành', role: 'both', email: 'songhanh@example.invalid', phone: '', active: true },
  { id: 'c2', code: 'KH-OLD', name: 'Khách hàng cũ', role: 'customer', email: '', phone: '', active: false },
];

test('lọc đối tượng theo vai trò khách hàng, nhà cung cấp và dùng chung cả hai bên', () => {
  assert.deepEqual(filterCatalogPartners(partners, 'customer', '', true).map(p => p.id), ['c1', 'b1']);
  assert.deepEqual(filterCatalogPartners(partners, 'supplier', '', true).map(p => p.id), ['s1', 'b1']);
  assert.deepEqual(filterCatalogPartners(partners, 'all', '', true).map(p => p.id), ['c1', 's1', 'b1']);
});

test('tìm không dấu theo mã, tên, email, điện thoại và giữ đối tượng ngừng dùng ở danh mục', () => {
  assert.deepEqual(filterCatalogPartners(partners, 'customer', 'anh duong', true).map(p => p.id), ['c1']);
  assert.deepEqual(filterCatalogPartners(partners, 'all', '090333').map(p => p.id), ['s1']);
  assert.deepEqual(filterCatalogPartners(partners, 'customer').map(p => p.id), ['c1', 'b1', 'c2']);
});
