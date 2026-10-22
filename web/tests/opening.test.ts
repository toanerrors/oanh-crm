import assert from 'node:assert/strict';
import test from 'node:test';
import { openingTotals } from '../src/shared/accounting/opening.ts';

test('opening totals retain exact money and independent debit/credit partner balances', () => {
  const result = openingTotals({ lines: [{ debit: '9007199254740993', credit: '0' }, { debit: '100', credit: '0' }, { debit: '0', credit: '40' }, { debit: '0', credit: '9007199254741053' }], stocks: [], assets: [] });
  assert.deepEqual(result, { debit: 9007199254741093n, credit: 9007199254741093n, difference: 0n, balanced: true });
});
test('opening inventory and assets contribute the same lines as server', () => {
  assert.deepEqual(openingTotals({ lines: [{ debit: '500', credit: '0' }, { debit: '0', credit: '2501' }], stocks: [{ value: '1001' }], assets: [{ kind: 'fixed', cost: '1200', allocated: '300' }, { kind: 'prepaid', cost: '200', allocated: '100' }] }), { debit: 2801n, credit: 2801n, difference: 0n, balanced: true });
});
test('empty, invalid and unbalanced opening never show a successful balance', () => {
  assert.equal(openingTotals({ lines: [], stocks: [], assets: [] })?.balanced, false);
  assert.equal(openingTotals({ lines: [{ debit: '100', credit: '99' }], stocks: [], assets: [] })?.difference, 1n);
  assert.equal(openingTotals({ lines: [{ debit: '1.2' }], stocks: [], assets: [] }), null);
  assert.equal(openingTotals({ lines: [], stocks: [], assets: [{ kind: 'prepaid', cost: '100', allocated: '101' }] }), null);
});
