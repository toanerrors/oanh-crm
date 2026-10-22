import assert from 'node:assert/strict';
import test from 'node:test';
import { journalEntryRows, journalEntrySides } from '../src/shared/accounting/journal-entry.ts';
import type { JournalLine } from '../src/shared/data/types.ts';

test('bảng Nợ/Có có tài khoản, đối tượng và tiền riêng, giữ nguyên thứ tự dòng nguồn', () => {
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
