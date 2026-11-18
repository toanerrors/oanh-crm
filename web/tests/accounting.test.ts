import assert from 'node:assert/strict';
import test from 'node:test';
import { formatMoney, isMoney, MAX_MONEY, isCalendarDate, normalizeSearch, compactMoney, parseMoneyInput } from '../src/shared/accounting/money.ts';
import { accountBalance, allocationInvoiceOptions, filterDocuments, formatLineAccount, formatLinePartner, formatLineProject, hydrateVoucherDefaultDates, initialVoucherDateSync, invoiceAvailableAmount, nonZeroTrialRows, normalizeVoucherInputDates, partnerBalances, periodDocuments, periodMovement, projectSummaryRows, syncVoucherDependentDates, totals, trialBalance, validateDraft } from '../src/shared/accounting/ledger.ts';
import { accountingRangeSlug, activePeriodPreset, formatAccountingRange, monthDateRange, normalizeDateRange, periodPresetOptions, resolvePresetRange } from '../src/shared/accounting/period-range.ts';
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

test('hiển thị đối tượng và tài khoản trên dòng hạch toán dùng mã và tên, không lộ UUID', () => {
  const partners = [{ id: '67fe6f44-66e8-47eb-b0d4-f747be62cbd4', code: '0318088925', name: 'CÔNG TY CỔ PHẦN HARMONY WORLD', role: 'supplier' as const, email: '', phone: '', active: true }];
  const accounts = [{ id: 'a1b2c3d4-0000-4000-8000-000000000331', code: '3311', name: 'Phải trả nhà cung cấp nội địa', group: 'payable' as const, active: true }];
  assert.deepEqual(formatLinePartner({}, partners), { text: '—' });
  assert.deepEqual(
    formatLinePartner({ partnerId: '67fe6f44-66e8-47eb-b0d4-f747be62cbd4', partnerName: 'CÔNG TY CỔ PHẦN HARMONY WORLD' }, partners),
    { text: '0318088925 · CÔNG TY CỔ PHẦN HARMONY WORLD', title: '0318088925 · CÔNG TY CỔ PHẦN HARMONY WORLD' },
  );
  assert.deepEqual(
    formatLinePartner({ partnerId: '67fe6f44-66e8-47eb-b0d4-f747be62cbd4', partnerName: 'Tên tại thời điểm ghi sổ' }, partners),
    { text: '0318088925 · Tên tại thời điểm ghi sổ', title: '0318088925 · Tên tại thời điểm ghi sổ' },
  );
  assert.deepEqual(formatLinePartner({ partnerId: 'missing-uuid' }, partners), { text: 'Không tìm thấy đối tượng' });
  assert.deepEqual(formatLinePartner({ partnerId: 'loading-uuid' }, undefined), { text: 'Đang tải…' });
  assert.deepEqual(formatLinePartner({ partnerId: 'loading-uuid', partnerName: 'Tên từ server' }, undefined), { text: 'Tên từ server', title: 'Tên từ server' });
  assert.deepEqual(
    formatLineAccount({ accountId: 'a1b2c3d4-0000-4000-8000-000000000331' }, accounts),
    { code: '3311', name: 'Phải trả nhà cung cấp nội địa', title: '3311 · Phải trả nhà cung cấp nội địa' },
  );
  assert.deepEqual(formatLineAccount({ accountId: 'missing-account-uuid' }, accounts), { code: 'Không tìm thấy tài khoản', name: undefined, title: undefined });
  assert.deepEqual(formatLineAccount({ accountId: 'loading-account-uuid' }, undefined), { code: 'Đang tải…', name: undefined, title: undefined });
});

test('cân đối phát sinh ẩn dòng có cả 6 cột bằng 0, giữ dòng có số dư hoặc phát sinh', () => {
  const septemberAll = trialBalance(initialWorkspace, '2026-09');
  const septemberVisible = nonZeroTrialRows(septemberAll);
  assert.ok(septemberAll.length > septemberVisible.length);
  for (const zeroCode of ['111', '156', '211', '331', '632']) {
    assert.ok(septemberAll.some(row => row.code === zeroCode));
    assert.equal(septemberVisible.some(row => row.code === zeroCode), false, zeroCode);
  }
  assert.equal(septemberVisible.every(row => row.openingDebit || row.openingCredit || row.debit || row.credit || row.closingDebit || row.closingCredit), true);
  for (const field of ['openingDebit', 'openingCredit', 'debit', 'credit', 'closingDebit', 'closingCredit'] as const) {
    assert.equal(
      septemberVisible.reduce((sum, row) => sum + row[field], 0n),
      septemberAll.reduce((sum, row) => sum + row[field], 0n),
    );
  }
  const settledOnly = nonZeroTrialRows([
    { code: '112', openingDebit: 0n, openingCredit: 0n, debit: 0n, credit: 0n, closingDebit: 0n, closingCredit: 0n },
    { code: '911', openingDebit: 0n, openingCredit: 0n, debit: 500n, credit: 500n, closingDebit: 0n, closingCredit: 0n },
  ]);
  assert.deepEqual(settledOnly.map(row => row.code), ['911']);
});

test('phân bổ công nợ hiển thị số chứng từ, tính đúng số tiền tự điền kể cả trên 2^53 và lọc dòng trùng', () => {
  const invoices = [
    { documentId: 'MWFVV4PZ5HVBEBSXJ63CAKMXT3', reference: 'MH-2026-000004', amount: '2160000', remaining: '2160000' },
    { documentId: 'INV-LARGE', reference: 'MH-2026-000005', invoiceSeries: '1C26TAA', invoiceNumber: '000088', amount: '9007199254740993', remaining: '9007199254740993' },
    { documentId: 'INV-PAID', reference: 'MH-2026-000006', amount: '500000', remaining: '0' },
  ];
  assert.equal(invoiceAvailableAmount(invoices[0]), '2160000');
  assert.equal(invoiceAvailableAmount(invoices[1]), '9007199254740993');
  assert.equal(
    invoiceAvailableAmount(invoices[2], [{ documentId: 'INV-PAID', amount: '500000' }], true),
    '500000',
  );
  const firstRowOptions = allocationInvoiceOptions(invoices, [{ documentId: 'MWFVV4PZ5HVBEBSXJ63CAKMXT3', amount: '2160000' }, { documentId: '', amount: '' }], 0);
  assert.deepEqual(firstRowOptions, [
    { value: 'MWFVV4PZ5HVBEBSXJ63CAKMXT3', label: 'MH-2026-000004 · Còn 2.160.000 VND' },
    { value: 'INV-LARGE', label: 'MH-2026-000005 · HĐ 1C26TAA 000088 · Còn 9.007.199.254.740.993 VND' },
  ]);
  const secondRowOptions = allocationInvoiceOptions(invoices, [{ documentId: 'MWFVV4PZ5HVBEBSXJ63CAKMXT3', amount: '2160000' }, { documentId: '', amount: '' }], 1);
  assert.deepEqual(secondRowOptions, [
    { value: 'INV-LARGE', label: 'MH-2026-000005 · HĐ 1C26TAA 000088 · Còn 9.007.199.254.740.993 VND' },
  ]);
  const loadingFallback = allocationInvoiceOptions(undefined, [{ documentId: 'MWFVV4PZ5HVBEBSXJ63CAKMXT3', amount: '2160000' }], 0, {
    documents: [{ id: 'MWFVV4PZ5HVBEBSXJ63CAKMXT3', reference: 'MH-2026-000004' }],
  });
  assert.deepEqual(loadingFallback, [{ value: 'MWFVV4PZ5HVBEBSXJ63CAKMXT3', label: 'MH-2026-000004' }]);
});

test('khoảng ngày kế toán tự lọc, đầu năm đến nay, năm nhuận và cắt sổ theo ngày', () => {
  assert.deepEqual(monthDateRange('2024-02'), { from: '2024-02-01', to: '2024-02-29' });
  assert.deepEqual(monthDateRange('2026-02'), { from: '2026-02-01', to: '2026-02-28' });
  assert.equal(formatAccountingRange('2026-11-01', '2026-11-30'), 'Kỳ 11/2026');
  assert.equal(formatAccountingRange('2026-01-01', '2026-11-18'), 'Từ 01/01/2026 đến 18/11/2026');
  assert.equal(accountingRangeSlug('2026-10-01', '2026-10-31'), '2026-10');
  assert.equal(accountingRangeSlug('2026-01-01', '2026-11-18'), '2026-01-01_2026-11-18');
  assert.deepEqual(normalizeDateRange('2026-11-20', '2026-11-10', 'from'), { from: '2026-11-20', to: '2026-11-20' });
  assert.deepEqual(normalizeDateRange('2026-11-20', '2026-11-10', 'to'), { from: '2026-11-10', to: '2026-11-10' });

  const options = periodPresetOptions(2026, true);
  const octIdx = options.findIndex(item => item.value === '2026-10');
  assert.equal(options[octIdx - 1]?.value, '2026-09');

  assert.deepEqual(
    resolvePresetRange('ytd', { today: '2026-11-18', conversionDate: '2026-03-01' }),
    { from: '2026-03-01', to: '2026-11-18' },
  );
  assert.equal(
    activePeriodPreset('2026-03-01', '2026-11-18', 'ytd', { today: '2026-11-18', conversionDate: '2026-03-01' }),
    'ytd',
  );
  assert.equal(
    activePeriodPreset('2026-08-03', '2026-11-18', null, { today: '2026-11-18' }),
    'custom',
  );

  const twoMonthDocs = periodDocuments(initialWorkspace.documents, '2026-10', '2026-09-01', '2026-10-31');
  const ytdDocs = periodDocuments(initialWorkspace.documents, '2026-10', '2026-01-01', '2026-10-31');
  const sepDocs = periodDocuments(initialWorkspace.documents, '2026-09');
  const octDocs = periodDocuments(initialWorkspace.documents, '2026-10');
  assert.equal(twoMonthDocs.length, sepDocs.length + octDocs.length);
  assert.ok(ytdDocs.length > twoMonthDocs.length);

  const ytdTrial = trialBalance(initialWorkspace, '2026-10', '2026-01-01', '2026-10-31');
  const octTrial = trialBalance(initialWorkspace, '2026-10');
  for (const row of ytdTrial) {
    assert.equal(row.openingDebit, 0n);
    assert.equal(row.openingCredit, 0n);
    const octRow = octTrial.find(item => item.key === row.key)!;
    assert.equal(row.closingDebit, octRow.closingDebit);
    assert.equal(row.closingCredit, octRow.closingCredit);
  }
});

test('ngày chứng từ và ngày hóa đơn mặc định theo ngày hạch toán nếu người dùng không tự đổi', () => {
  const raw = { kind: 'purchase', date: '2026-11-18' };
  const hydrated = hydrateVoucherDefaultDates(raw);
  assert.equal(hydrated.voucher?.documentDate, '2026-11-18');
  assert.equal(hydrated.invoiceDate, '2026-11-18');
  assert.equal(hydrateVoucherDefaultDates({ kind: 'payment', date: '2026-11-18' }).invoiceDate, undefined);

  let state = initialVoucherDateSync(raw);
  assert.deepEqual(state, { documentDateCustom: false, invoiceDateCustom: false });

  const step1 = syncVoucherDependentDates({ date: '2026-08-03' }, { kind: 'purchase', date: '2026-08-03' }, state);
  state = step1.nextState;
  assert.deepEqual(step1.patch, { documentDate: '2026-08-03', invoiceDate: '2026-08-03' });

  const step2 = syncVoucherDependentDates({ invoiceDate: '2026-08-01' }, { kind: 'purchase', date: '2026-08-03', invoiceDate: '2026-08-01' }, state);
  state = step2.nextState;
  assert.equal(state.invoiceDateCustom, true);

  const step3 = syncVoucherDependentDates({ date: '2026-08-10' }, { kind: 'purchase', date: '2026-08-10', invoiceDate: '2026-08-01' }, state);
  state = step3.nextState;
  assert.deepEqual(step3.patch, { documentDate: '2026-08-10' });

  const step4 = syncVoucherDependentDates({ voucher: { documentDate: '2026-08-09' } }, { kind: 'purchase', date: '2026-08-10', voucher: { documentDate: '2026-08-09' } }, state);
  state = step4.nextState;
  assert.deepEqual(state, { documentDateCustom: true, invoiceDateCustom: true });

  const step5 = syncVoucherDependentDates({ date: '2026-08-15' }, { kind: 'purchase', date: '2026-08-15' }, state);
  assert.equal(step5.patch, undefined);

  const withoutInvoice = normalizeVoucherInputDates({
    kind: 'purchase',
    date: '2026-08-03',
    invoiceDate: '2026-08-03',
    voucher: { documentDate: '2026-08-03' },
  });
  assert.equal(withoutInvoice.invoiceDate, undefined);
  assert.equal(withoutInvoice.voucher?.documentDate, '2026-08-03');

  const withInvoice = normalizeVoucherInputDates({
    kind: 'purchase',
    date: '2026-08-03',
    invoiceNumber: '0000123',
    invoiceSeries: '1C26TAA',
    invoiceDate: '2026-08-03',
    voucher: { documentDate: '2026-08-03' },
  });
  assert.equal(withInvoice.invoiceDate, '2026-08-03');
  assert.equal(withInvoice.invoiceNumber, '0000123');

  const fallbackInvoiceDate = normalizeVoucherInputDates({
    kind: 'sale',
    date: '2026-08-03',
    invoiceNumber: '0000124',
  });
  assert.equal(fallbackInvoiceDate.invoiceDate, '2026-08-03');
  assert.equal(fallbackInvoiceDate.voucher?.documentDate, '2026-08-03');
});

test('tổng hợp doanh thu, chi phí và dở dang theo từng công trình giữ chính xác BigInt trên 2^53', () => {
  const projects = [
    { id: 'ct-1', code: 'CT001', name: 'Công trình A', description: '', active: true },
    { id: 'ct-old', code: 'CT000', name: 'Công trình cũ không phát sinh', description: '', active: false },
  ];
  assert.equal(formatLineProject({ projectId: 'ct-1' }, projects).text, 'CT001 · Công trình A');
  assert.equal(formatLineProject({ projectId: 'ct-1', projectName: 'Tên snapshot cũ' }, projects).text, 'CT001 · Tên snapshot cũ');
  assert.equal(formatLineProject({}, projects).text, '—');

  const ws = {
    ...initialWorkspace,
    accounts: [...initialWorkspace.accounts, { id: '154', code: '154', name: 'Chi phí dở dang', group: 'wip' as const }],
    projects,
    documents: [
      {
        id: 'sale-ct1', reference: 'BH-1', type: 'sale' as const, date: '2026-11-10', description: 'Doanh thu CT1', status: 'posted' as const, version: 1,
        lines: [
          { accountId: '131', partnerId: 'p1', description: 'Phải thu', debit: '9007199264741000', credit: '0' },
          { accountId: '511', projectId: 'ct-1', description: 'Doanh thu', debit: '0', credit: '9007199264741000' },
        ],
      },
      {
        id: 'wip-ct1', reference: 'MH-1', type: 'purchase' as const, date: '2026-11-11', description: 'Tập hợp 154', status: 'posted' as const, version: 1,
        lines: [
          { accountId: '154', projectId: 'ct-1', description: 'Dở dang', debit: '5000000', credit: '0' },
          { accountId: '331', partnerId: 'p3', description: 'Phải trả', debit: '0', credit: '5000000' },
        ],
      },
      {
        id: 'costing-ct1', reference: 'GT-1', type: 'journal' as const, sourceType: 'costing', date: '2026-11-15', description: 'Kết chuyển 154 -> 632', status: 'posted' as const, version: 1,
        lines: [
          { accountId: '632', projectId: 'ct-1', description: 'Giá vốn', debit: '3000000', credit: '0' },
          { accountId: '154', projectId: 'ct-1', description: 'Kết chuyển', debit: '0', credit: '3000000' },
        ],
      },
    ],
  };
  const summary = projectSummaryRows(ws, '2026-11', '2026-11-01', '2026-11-30');
  assert.equal(summary.length, 1);
  assert.equal(summary[0].revenue, 9007199264741000n);
  assert.equal(summary[0].expense, 3000000n);
  assert.equal(summary[0].wip, 2000000n);
  assert.equal(summary[0].totalCost, 5000000n);
  assert.equal(summary[0].profit, 9007199259741000n);
});
