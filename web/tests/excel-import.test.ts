import assert from 'node:assert/strict';
import test from 'node:test';
import { excelKindForPath, matchExcelHeaders, unusedExcelColumns } from '../src/features/imports/model.ts';
import type { ExcelField, ExcelSource } from '../src/features/imports/model.ts';
const field = (key: string, label: string): ExcelField => ({ key, label, type: 'text', basic: true, required: true, aliases: [key, label] });
test('ghép tiêu đề tiếng Việt có/không dấu và giữ vị trí cột thực tế', () => {
  assert.deepEqual(matchExcelHeaders(['SỐ TIỀN', 'Ngay hach toan', 'Ma nguon'], [field('date', 'Ngày hạch toán'), field('amount', 'Số tiền'), field('sourceId', 'Mã nguồn')]), { date: 1, amount: 0, sourceId: 2 });
});
test('không tự đoán khi tiêu đề trùng hoặc nhiều trường cùng nhận một cột', () => {
  assert.deepEqual(matchExcelHeaders(['Mã', 'Mã'], [field('code', 'Mã')]), {});
  assert.deepEqual(matchExcelHeaders(['Mã'], [field('code', 'Mã'), field('source', 'Mã')]), {});
});
test('cảnh báo cột có dữ liệu ngoài 10 dòng mẫu đầu tiên', () => {
  const source: ExcelSource = { batch: { id: 'batch', filename: 'test.xlsx', hash: '', state: 'staged', createdAt: '', sheets: [] }, sheet: 'Data', headerRow: 1, headers: ['Mã', 'Thông tin khác'], sample: [[{ value: '001' }, { value: '' }]], sampleRows: [2], populatedColumns: [0, 1] };
  assert.deepEqual(unusedExcelColumns(source, { code: 0 }), ['Thông tin khác']);
  assert.deepEqual(unusedExcelColumns(source, { code: 0, name: 1 }), []);
});
test('lối vào phân hệ chọn đúng loại nhập mà không thay đổi đường dẫn nguồn', () => {
  for (const [path, kind] of [['/sales', 'sale'], ['/sales/partners', 'partners'], ['/purchases', 'purchase'], ['/purchases/items', 'items'], ['/cash', 'receipt'], ['/bank', 'receipt'], ['/documents', 'journal-pairs']]) assert.equal(excelKindForPath(path), kind);
});
