import test from 'node:test';
import assert from 'node:assert/strict';
import { filterCatalogPartners, filterCatalogProjects } from '../src/shared/data/partner-catalog.ts';
import { saveDemoPartners, saveDemoProjects } from '../src/shared/data/workspace.ts';
import type { Partner } from '../src/shared/data/types.ts';

const partners: Partner[] = [
  { id: 'c1', code: 'KH001', name: 'Công ty TNHH Ánh Dương', role: 'customer', taxCode: '0318088925', address: '12 Nguyễn Huệ, Quận 1, TP.HCM', email: 'anhduong@example.invalid', phone: '090111222', active: true },
  { id: 's1', code: 'NCC001', name: 'Nhà cung cấp Vật Tư', role: 'supplier', taxCode: '0101245486-001', address: '88 Láng Hạ, Đống Đa, Hà Nội', email: '', phone: '090333444', active: true },
  { id: 'b1', code: 'DT001', name: 'Đối tác Song Hành', role: 'both', email: 'songhanh@example.invalid', phone: '', active: true },
  { id: 'c2', code: 'KH-OLD', name: 'Khách hàng cũ', role: 'customer', email: '', phone: '', active: false },
];

test('lọc đối tượng theo vai trò khách hàng, nhà cung cấp và dùng chung cả hai bên', () => {
  assert.deepEqual(filterCatalogPartners(partners, 'customer', '', true).map(p => p.id), ['c1', 'b1']);
  assert.deepEqual(filterCatalogPartners(partners, 'supplier', '', true).map(p => p.id), ['s1', 'b1']);
  assert.deepEqual(filterCatalogPartners(partners, 'all', '', true).map(p => p.id), ['c1', 's1', 'b1']);
});

test('tìm không dấu theo mã, tên, mã số thuế, địa chỉ, email, điện thoại và giữ đối tượng ngừng dùng ở danh mục', () => {
  assert.deepEqual(filterCatalogPartners(partners, 'customer', 'anh duong', true).map(p => p.id), ['c1']);
  assert.deepEqual(filterCatalogPartners(partners, 'all', '0318088925').map(p => p.id), ['c1']);
  assert.deepEqual(filterCatalogPartners(partners, 'supplier', '0101245486-001', true).map(p => p.id), ['s1']);
  assert.deepEqual(filterCatalogPartners(partners, 'all', 'lang ha', true).map(p => p.id), ['s1']);
  assert.deepEqual(filterCatalogPartners(partners, 'all', 'nguyen hue', true).map(p => p.id), ['c1']);
  assert.deepEqual(filterCatalogPartners(partners, 'all', '090333').map(p => p.id), ['s1']);
  assert.deepEqual(filterCatalogPartners(partners, 'customer').map(p => p.id), ['c1', 'b1', 'c2']);
});

test('lưu đối tượng trong phiên mẫu chuẩn hóa mã số thuế và địa chỉ dạng chuỗi', async () => {
  const [saved] = await saveDemoPartners([{
    id: 'demo-harmony-tax',
    code: ' ncc-harmony ',
    name: ' Công ty Harmony World ',
    role: 'supplier',
    taxCode: ' 0318088925 ',
    address: ' 12 Nguyễn Huệ, Quận 1, TP.HCM ',
    email: ' ketoan@harmony.invalid ',
    phone: ' 0901234567 ',
    active: true,
  }]);
  assert.equal(saved.code, 'NCC-HARMONY');
  assert.equal(saved.taxCode, '0318088925');
  assert.equal(saved.address, '12 Nguyễn Huệ, Quận 1, TP.HCM');
});

test('lọc và lưu danh mục công trình không dấu, chặn trùng mã', async () => {
  const [saved] = await saveDemoProjects([{
    id: 'demo-ct-ld',
    code: ' ct-ld01 ',
    name: ' Cho thuê lao động Nhà máy A ',
    description: ' Hợp đồng quý 4 ',
    active: true,
  }]);
  assert.equal(saved.code, 'CT-LD01');
  assert.equal(saved.name, 'Cho thuê lao động Nhà máy A');
  assert.equal(saved.description, 'Hợp đồng quý 4');
  await assert.rejects(() => saveDemoProjects([{ id: 'dup-ct', code: 'ct-ld01', name: 'Trùng', description: '', active: true }]), /Mã công trình CT-LD01 bị trùng/);
  assert.deepEqual(filterCatalogProjects([saved, { id: 'old', code: 'CT-OLD', name: 'Đã xong', description: '', active: false }], 'nha may a', true).map(p => p.id), ['demo-ct-ld']);
});
