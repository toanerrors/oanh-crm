import assert from 'node:assert/strict';
import test from 'node:test';
import { formatMoney, isMoney, MAX_MONEY, isCalendarDate, normalizeSearch, compactMoney, parseMoneyInput } from '../src/shared/accounting/money.ts';
import { accountBalance, filterDocuments, partnerBalances, periodDocuments, periodMovement, totals, trialBalance, validateDraft } from '../src/shared/accounting/ledger.ts';
import { serializeCsv } from '../src/shared/accounting/csv.ts';
import { initialWorkspace } from '../src/shared/data/demo.ts';
import { saveDemoDraft, saveDemoPartners, workspaceOptions } from '../src/shared/data/workspace.ts';
import type { AccountingDocument } from '../src/shared/data/types.ts';

test('định dạng tiền lớn giữ chính xác toàn bộ chữ số', () => {
  assert.equal(formatMoney('9007199254740993'), '9.007.199.254.740.993');
  assert.equal(formatMoney(MAX_MONEY), '9.223.372.036.854.775.807');
  assert.equal(formatMoney(-123456789n), '−123.456.789');
  assert.equal(formatMoney('0'), '0');
  assert.throws(() => formatMoney('1.5'));
  assert.deepEqual(compactMoney(528000000n), { amount: '528', unit: 'triệu' });
  assert.deepEqual(compactMoney(1279000000n), { amount: '1,279', unit: 'tỷ' });
});

test('tiền canonical không chấp nhận số gần đúng, định dạng locale hoặc tràn int64', () => {
  for (const value of [1, '', '-1', '01', '1.000', '1e3', '0.1', ' 100', (MAX_MONEY + 1n).toString()]) assert.equal(isMoney(value), false, String(value));
  assert.equal(isMoney('9007199254740993'), true);
  assert.equal(isMoney(MAX_MONEY.toString()), true);
});

test('ngày lịch kiểm tra đúng tháng và năm nhuận', () => {
  assert.equal(isCalendarDate('2026-02-29'), false);
  assert.equal(isCalendarDate('2024-02-29'), true);
  assert.equal(isCalendarDate('2026-13-01'), false);
  assert.equal(isCalendarDate('2026-1-01'), false);
});

test('parser ô tiền không đoán dấu phân cách hoặc tự làm tròn số lẻ', () => {
  assert.equal(parseMoneyInput('9.007.199.254.740.993'), '9007199254740993');
  assert.equal(parseMoneyInput('00015'), '15');
  assert.equal(parseMoneyInput(''), undefined);
  for (const text of ['1.5', '1,5', '1e3', '-100', '12.34', '1,000']) assert.equal(isMoney(parseMoneyInput(text)), false, text);
});

test('dữ liệu mẫu có bút toán cân đối và đối tượng hợp lệ', () => {
  for (const document of initialWorkspace.documents) {
    assert.deepEqual(validateDraft(document, initialWorkspace), [], document.reference);
    const total = totals(document.lines);
    assert.equal(total.debit, total.credit, document.reference);
  }
});

test('tổng quan tháng 10 lấy từ chứng từ đã ghi sổ, không tính nháp', () => {
  assert.equal(periodMovement(initialWorkspace.documents, initialWorkspace.accounts, '2026-10', 'revenue'), 528000000n);
  assert.equal(periodMovement(initialWorkspace.documents, initialWorkspace.accounts, '2026-10', 'expense'), 198000000n);
  assert.equal(accountBalance(initialWorkspace.documents, ['111', '1121'], '2026-10'), 1279000000n);
  assert.equal(periodDocuments(initialWorkspace.documents, '2026-10').length, 13);
  assert.equal(periodDocuments(initialWorkspace.documents, '2026-01').length, 1);
  assert.equal(periodDocuments(initialWorkspace.documents, '2025-10').length, 0);
});

test('công nợ giữ riêng số dư Nợ/Có từng đối tượng', () => {
  const balances = partnerBalances(initialWorkspace, '2026-10', 'receivable');
  assert.equal(balances.find(item => item.id === 'p1')?.balance, -305000000n);
  assert.equal(balances.find(item => item.id === 'p2')?.balance, 618000000n);
  assert.equal(balances.reduce((sum, item) => sum + (item.balance > 0n ? item.balance : 0n), 0n), 1106000000n);
});

test('cân đối phát sinh giữ hai bên công nợ và khớp số liệu liên kỳ', () => {
  const report = trialBalance(initialWorkspace, '2026-10');
  const receivable = report.find(row => row.code === '131')!;
  assert.equal(receivable.openingDebit, 1048000000n);
  assert.equal(receivable.openingCredit, 490000000n);
  assert.equal(receivable.closingDebit, 1106000000n);
  assert.equal(receivable.closingCredit, 305000000n);
  assert.equal(receivable.closingDebit - receivable.closingCredit, receivable.openingDebit - receivable.openingCredit + receivable.debit - receivable.credit);
  for (const [left, right] of [['openingDebit', 'openingCredit'], ['debit', 'credit'], ['closingDebit', 'closingCredit']] as const) assert.equal(report.reduce((sum, row) => sum + row[left], 0n), report.reduce((sum, row) => sum + row[right], 0n));
  const previous = trialBalance(initialWorkspace, '2026-09');
  for (const row of report) {
    const last = previous.find(item => item.code === row.code)!;
    assert.equal(row.openingDebit, last.closingDebit);
    assert.equal(row.openingCredit, last.closingCredit);
  }
});

test('tìm kiếm không dấu hỗ trợ đ và tên đối tượng', () => {
  assert.equal(normalizeSearch('Đối TƯỢNG'), 'doi tuong');
  const found = filterDocuments(initialWorkspace.documents, 'minh anh', initialWorkspace.partners);
  assert.ok(found.some(item => item.id === 'd1'));
  assert.equal(filterDocuments(initialWorkspace.documents, 'khong ton tai', initialWorkspace.partners).length, 0);
});

test('nháp cho phép lưu chưa cân đối, từ chối dòng hai bên và tổng tràn', () => {
  const draft = structuredClone(initialWorkspace.documents.find(item => item.id === 'd11')!);
  draft.lines[1].credit = '1';
  assert.deepEqual(validateDraft(draft, initialWorkspace), []);
  draft.lines[0].credit = '1';
  assert.match(validateDraft(draft, initialWorkspace).join(), /chỉ nhập một bên/);
  draft.lines[0].credit = '0';
  draft.lines[0].partnerId = undefined;
  assert.match(validateDraft(draft, initialWorkspace).join(), /đối tượng/);
  draft.lines = [0, 1].map(() => ({ accountId: '111', description: '', debit: MAX_MONEY.toString(), credit: '0' }));
  assert.match(validateDraft(draft, initialWorkspace).join(), /Tổng chứng từ vượt/);
});

test('CSV chống công thức với khoảng trắng, giữ Unicode và escape dấu nháy', () => {
  const csv = serializeCsv([['=SUM(A1)', ' +cmd', '@formula', 'Tên "A"', '9007199254740993 VND']]);
  assert.ok(csv.startsWith('\uFEFF'));
  assert.ok(csv.includes('"\'=SUM(A1)"'));
  assert.ok(csv.includes('"\' +cmd"'));
  assert.ok(csv.includes('"Tên ""A"""'));
  assert.ok(csv.includes('9007199254740993 VND'));
});

test('lưu nháp giữ phiên bản, không cho sửa đã ghi sổ hoặc ghi đè bản cũ', async () => {
  const document: AccountingDocument = { ...structuredClone(initialWorkspace.documents.find(item => item.id === 'd11')!), id: 'test-draft', version: 0 };
  document.lines[0].debit = '9007199254740993';
  document.lines[1].credit = '9007199254740993';
  const saved = await saveDemoDraft(document);
  assert.equal(saved.status, 'draft');
  assert.equal(saved.version, 1);
  assert.equal(saved.lines[0].debit, '9007199254740993');
  await assert.rejects(saveDemoDraft(document), /đã thay đổi/);
  await assert.rejects(saveDemoDraft(initialWorkspace.documents.find(item => item.id === 'd1')!), /đã ghi sổ/);
});

test('danh mục từ chối trùng mã không phân biệt hoa thường trước khi cập nhật', async () => {
  const partner = { ...initialWorkspace.partners[0], id: 'new-partner', code: 'kh001' };
  await assert.rejects(saveDemoPartners([partner]), /bị trùng/);
});

test('danh mục giữ nguyên mã, tên và vai trò đã có lịch sử', async () => {
  await assert.rejects(saveDemoPartners([{ ...initialWorkspace.partners[0], name: 'Tên khác' }]), /Giữ nguyên tên/);
  await assert.rejects(saveDemoPartners([{ ...initialWorkspace.partners[0], code: 'KH999' }]), /Không được đổi mã/);
});

test('query key tách kỳ, danh tính, quyền và dataset', () => {
  const october = workspaceOptions('2026-10').queryKey;
  const september = workspaceOptions('2026-09').queryKey;
  assert.notDeepEqual(october, september);
  assert.deepEqual(october, ['demo-workspace', 'demo-user', 'demo-editor', 'demo-company', 'demo-2026', '2026-10']);
});


test('danh mục chưa dùng cho phép sửa mã và vẫn chặn mã trùng', async () => {
  const partner = { id: 'unused-edit', code: 'EDIT001', name: 'Đối tác mới', email: '', phone: '', role: 'customer' as const, active: true };
  await saveDemoPartners([partner]);
  await saveDemoPartners([{ ...partner, code: 'EDIT002', name: 'Đối tác đã sửa', role: 'both' }]);
  await assert.rejects(saveDemoPartners([{ ...partner, id: 'duplicate-edit', code: 'EDIT002' }]), /bị trùng/);
});
