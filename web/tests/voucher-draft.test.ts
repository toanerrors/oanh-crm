import assert from 'node:assert/strict';
import test from 'node:test';
import { updateVoucherDraft, voucherDraftKey, voucherDraftLabel, voucherDraftModule, voucherKind } from '../src/features/vouchers/draft-state.ts';
import type { VoucherDraft } from '../src/features/vouchers/draft-state.ts';
const params = (value: string) => new URLSearchParams(value);
const key = (value: string, id?: string) => voucherDraftKey(id, params(value));
const payment: VoucherDraft = { input: { kind: 'payment', sourceId: 'payment-only', description: 'Chi thuê nhà', date: '2026-10-23', moneyAccount: '111', treasuryLines: [{ counterAccount: '642', amount: '9007199254740993', description: 'Chi thuê nhà' }] }, allocate: false, route: '/vouchers/new?kind=payment&account=111', returnTo: '/cash?page=2' };
const sale: VoucherDraft = { input: { kind: 'sale', sourceId: 'sale-only', description: 'Bán hàng riêng', date: '2026-10-23', tradeMode: 'goods' }, allocate: false, route: '/vouchers/new?kind=sale', returnTo: '/sales' };

test('chi tiền, bán hàng và các quy trình độc lập không lấy nháp của nhau', () => {
  const flows = ['kind=payment&account=111', 'kind=receipt&account=111', 'kind=payment&account=1121', 'kind=receipt&account=1121', 'kind=transfer&account=111', 'kind=transfer&account=1121', 'kind=sale', 'kind=sale&service=1', 'kind=purchase', 'kind=purchase&service=1', 'kind=closing', 'kind=payment&settlement=1', 'kind=receipt&settlement=1'];
  assert.equal(new Set(flows.map(value => key(value))).size, flows.length);
  assert.equal(key('kind=sale'), key('service=0&kind=sale&unused=1'));
  assert.equal(key('kind=payment'), key('account=111&kind=payment'));
  assert.equal(voucherKind(params('kind=unknown')), 'receipt');
  const drafts = updateVoucherDraft({}, key('kind=payment'), payment);
  assert.equal(drafts[key('kind=sale')], undefined);
  const next = updateVoucherDraft(drafts, key('kind=sale'), sale);
  assert.equal(next[key('kind=payment')].input.sourceId, 'payment-only');
  assert.equal(next[key('kind=sale')].input.sourceId, 'sale-only');
  assert.equal(next[key('kind=payment')].input.treasuryLines?.[0].amount, '9007199254740993');
});

test('sửa từng chứng từ và lập bản thay thế giữ đúng ID, không kế thừa nháp mới', () => {
  const newKey = key('kind=payment');
  const firstKey = key('', 'draft-1');
  const secondKey = key('', 'draft-2');
  const replacementKey = key('kind=payment&replacement=original-1');
  assert.equal(new Set([newKey, firstKey, secondKey, replacementKey, key('kind=payment&replacement=original-2')]).size, 5);
  assert.equal(key('kind=sale', 'draft-1'), firstKey);
  let drafts = updateVoucherDraft({}, newKey, payment);
  drafts = updateVoucherDraft(drafts, firstKey, { ...payment, input: { ...payment.input, draftId: 'draft-1', draftVersion: 3 } });
  const after = updateVoucherDraft(drafts, firstKey, null);
  assert.equal(after[firstKey], undefined);
  assert.equal(after[newKey].input.description, payment.input.description);
  assert.equal(drafts[firstKey].input.draftVersion, 3);
  const original = structuredClone(payment);
  const copy = updateVoucherDraft({}, newKey, original);
  original.input.description = 'Không được ghi đè bản đang giữ';
  assert.equal(copy[newKey].input.description, payment.input.description);
});

test('nháp nhanh chỉ hiển thị trong đúng phân hệ, có nhãn nghiệp vụ rõ ràng', () => {
  assert.equal(voucherDraftModule(payment), '/cash');
  assert.equal(voucherDraftModule(sale), '/sales');
  const bank = { ...payment, input: { ...payment.input, moneyAccount: 'custom-bank-id', draftId: 'bank-draft' }, route: '/vouchers/bank-draft', returnTo: '/bank' };
  assert.equal(voucherDraftModule(bank), '/bank');
  assert.match(voucherDraftLabel(bank), /^Chi tiền gửi/);
  assert.match(voucherDraftLabel({ ...sale, input: { ...sale.input, tradeMode: 'service' } }), /^Bán dịch vụ/);
  assert.match(voucherDraftLabel({ ...payment, allocate: true }), /theo chứng từ/);
});
