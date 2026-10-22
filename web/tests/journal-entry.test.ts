import assert from 'node:assert/strict';
import test from 'node:test';
import { canPairJournalLines, journalEntryRows, journalEntrySides } from '../src/shared/accounting/journal-entry.ts';
import type { JournalLine } from '../src/shared/data/types.ts';

test('bố trí cặp Nợ/Có giữ nguyên thứ tự và chi tiết dòng nguồn', () => {
  const lines: JournalLine[] = [
    { accountId: '331', partnerId: 'supplier', description: 'Bên Có riêng', debit: '0', credit: '9007199254740993' },
    { accountId: '642', partnerId: 'customer', description: 'Bên Nợ riêng', debit: '9007199254740000', credit: '0' },
    { accountId: '1331', partnerId: 'supplier', description: 'VAT', debit: '993', credit: '0' },
  ];
  const before = structuredClone(lines);
  const fields = lines.map((_, name) => ({ name }));
  const sides = journalEntrySides(lines);
  const rows = journalEntryRows(fields, sides);
  assert.deepEqual(sides, ['credit', 'debit', 'debit']);
  assert.deepEqual(rows, [{ key: 0, debit: fields[1], credit: fields[0] }, { key: 1, debit: fields[2], credit: undefined }]);
  assert.equal(lines[rows[0].debit!.name].partnerId, 'customer');
  assert.equal(lines[rows[0].credit!.name].partnerId, 'supplier');
  assert.equal(lines[rows[0].credit!.name].credit, '9007199254740993');
  assert.deepEqual(lines, before);
});

test('nháp mới có hai bên và thêm/xóa riêng không đổi bên của dòng còn lại', () => {
  const blank = { accountId: '', description: '', debit: '0', credit: '0' };
  assert.deepEqual(journalEntrySides([blank, blank]), ['debit', 'credit']);
  const sides = ['debit', 'credit', 'credit'] as const;
  const fields = [0, 1, 2].map(name => ({ name }));
  assert.equal(journalEntryRows(fields, [...sides])[1].credit?.name, 2);
  assert.deepEqual(journalEntryRows([{ name: 0 }, { name: 1 }], ['credit', 'credit']), [{ key: 0, debit: undefined, credit: { name: 0 } }, { key: 1, debit: undefined, credit: { name: 1 } }]);
});


test('cặp hạch toán dùng chung số tiền lớn và diễn giải, giữ đối tượng mỗi bên', () => {
  const lines: JournalLine[] = [
    { accountId: '111', partnerId: 'customer', description: 'Diễn giải chung', debit: '9007199254740993', credit: '0' },
    { accountId: '411', partnerId: 'supplier', description: 'Diễn giải chung', debit: '0', credit: '9007199254740993' },
  ];
  const original = structuredClone(lines);
  assert.equal(canPairJournalLines(lines), true);
  assert.deepEqual(lines, original);
  assert.equal(canPairJournalLines([{ accountId: '', description: '', debit: '0', credit: '0' }, { accountId: '', description: '', debit: '0', credit: '0' }]), true);
});

test('chứng từ nhiều đối ứng hoặc khác diễn giải giữ chế độ chi tiết', () => {
  const debit = { accountId: '111', description: 'Chung', debit: '100', credit: '0' };
  const credit = { accountId: '411', description: 'Chung', debit: '0', credit: '100' };
  assert.equal(canPairJournalLines([debit, { ...credit, credit: '40' }, { ...credit, accountId: '338', credit: '60' }]), false);
  assert.equal(canPairJournalLines([debit, { ...credit, description: 'Riêng' }]), false);
  assert.equal(canPairJournalLines([debit, { ...credit, credit: '99' }]), false);
  assert.equal(canPairJournalLines([{ ...debit, credit: '100' }, credit]), false);
  assert.equal(canPairJournalLines([{ ...debit, debit: '1.5' }, { ...credit, credit: '1.5' }]), false);
  assert.equal(canPairJournalLines([]), false);
});

test('chế độ cặp không sắp xếp lại các dòng nguồn Có trước Nợ', () => {
  const lines: JournalLine[] = [
    { accountId: '331', partnerId: 'supplier', description: 'Một', debit: '0', credit: '25' },
    { accountId: '642', description: 'Một', debit: '25', credit: '0' },
    { accountId: '111', description: 'Hai', debit: '75', credit: '0' },
    { accountId: '411', description: 'Hai', debit: '0', credit: '75' },
  ];
  const original = structuredClone(lines);
  assert.equal(canPairJournalLines(lines), true);
  assert.deepEqual(journalEntryRows([0, 1, 2, 3], journalEntrySides(lines)), [{ key: 0, debit: 1, credit: 0 }, { key: 1, debit: 2, credit: 3 }]);
  assert.deepEqual(lines, original);
});
