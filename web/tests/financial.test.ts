import assert from 'node:assert/strict';
import test from 'node:test';
import { financialPreview, formatFinancialAmount } from '../src/features/reports/financial.ts';
import type { FinancialSheet } from '../src/features/reports/financial.ts';
test('BCTC giữ ô trống, tiền lớn, dấu âm và không đổi giá trị đầu vào', () => {
  const sheet: FinancialSheet = { id: 'B02-DN', name: 'Kết quả', rows: [{ code: '01', name: 'Doanh thu', level: 2, formula: [] }, { code: '02', name: 'Giảm trừ', level: 2, formula: [] }, { code: '10', name: 'Thuần', level: 0, formula: ['01', '-02'] }] };
  const input = { values: { 'B02-DN': { '01': { current: '9007199254740993', previous: '100', note: '' }, '02': { current: '100', previous: '', note: '' } } } };
  const result = financialPreview(input, sheet);
  assert.equal(result['10'].current, '9007199254740893');
  assert.equal(result['10'].previous, '');
  assert.equal(input.values['B02-DN']['01'].current, '9007199254740993');
  assert.equal(formatFinancialAmount('-9007199254740993'), '−9.007.199.254.740.993');
  assert.equal(formatFinancialAmount('-0.25'), '−0,25');
  assert.equal(formatFinancialAmount('1250.25'), '1.250,25');
});
