import { expect, test } from '@playwright/test';
import type { APIRequestContext, Page } from '@playwright/test';

test.describe.configure({ mode: 'serial' });
const credentials = { username: 'e2e-owner', password: 'Synthetic-test-only-2026' };
let epoch = '';
let backupPath = '';
const policy = { regime: 'Chế độ kiểm thử', effectiveFrom: '2026-01-01', confirmed: true, rounding: 'half_up', vatMethod: 'deduction', vatRates: ['0', '10'], inventoryMethod: 'fifo', quantityScale: 2, standardDays: '26', hoursPerDay: '8', personalDeduction: '100', dependentDeduction: '50', corporateTaxRate: '20', insurance: [{ name: 'Khoản BH thử', employeeRate: '10', employerRate: '20' }], pitBrackets: [{ upTo: '1000', rate: '5' }, { upTo: '', rate: '10' }], depreciationMethod: 'straight_line', residualMethod: 'last_period' };

async function command(request: APIRequestContext, path: string, data: unknown) {
  const response = await request.post(`/api/v1${path}`, { data, headers: { 'X-Dataset-Epoch': epoch, 'Idempotency-Key': crypto.randomUUID() } });
  expect(response.ok(), await response.text()).toBe(true);
  return response;
}
async function login(page: Page) {
  await page.clock.setFixedTime(new Date('2026-10-21T03:00:00Z'));
  await page.goto('/');
  await page.getByLabel('Tên đăng nhập', { exact: true }).fill(credentials.username);
  await page.getByLabel('Mật khẩu', { exact: true }).fill(credentials.password);
  await expect(page.getByRole('checkbox', { name: 'Giữ đăng nhập 30 ngày', exact: true })).toBeChecked();
  await page.getByRole('button', { name: 'Đăng nhập', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Tổng quan', exact: true })).toBeVisible();
}
async function choose(page: Page, label: string, search?: string) {
  const input = page.getByLabel(label, { exact: true });
  await input.click();
  if (search) { await input.fill(search); await page.locator('.ant-select-dropdown:visible .ant-select-item-option').filter({ hasText: search }).first().click(); }
  else await input.press('Enter');
}

test('khởi tạo, chính sách, ghi nhận bán hàng và tải lại vẫn giữ dữ liệu', async ({ page }) => {
  await page.clock.setFixedTime(new Date('2026-10-21T03:00:00Z'));
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message));
  await page.goto('/');
  await page.getByLabel('Tên doanh nghiệp', { exact: true }).fill('Doanh nghiệp E2E');
  await page.getByLabel('Tên chủ sở hữu', { exact: true }).fill('Người kiểm thử');
  await page.getByLabel('Chế độ kế toán / phiên bản áp dụng', { exact: true }).fill('Bộ chính sách kiểm thử, không phải tham số pháp luật');
  await page.getByLabel('Tên đăng nhập', { exact: true }).fill(credentials.username);
  await page.getByLabel('Mật khẩu', { exact: true }).fill(credentials.password);
  await page.getByRole('checkbox').check();
  await page.getByRole('button', { name: 'Khởi tạo và đăng nhập' }).click();
  await expect(page.getByRole('heading', { name: 'Tổng quan', exact: true })).toBeVisible();
  await expect(page.getByRole('banner').getByRole('button', { name: 'Thiết lập công ty', exact: true })).toContainText('Doanh nghiệp E2E');
  await expect(page.getByRole('complementary').getByRole('button', { name: 'Thiết lập công ty', exact: true })).toHaveCount(0);
  await page.getByRole('button', { name: 'Thiết lập công ty', exact: true }).click();
  await expect(page).toHaveURL(/\/settings$/);
  const session = await (await page.request.get('/api/v1/session')).json(); epoch = session.context.epoch;
  await command(page.request, '/periods', { period: '2026-10', closed: false });
  await command(page.request, '/policies', policy);
  await command(page.request, '/partners', { partners: [{ id: 'customer', code: 'KH1', name: 'Khách kiểm thử', role: 'customer', email: '', phone: '', active: true }, { id: 'supplier', code: 'NCC1', name: 'Nhà cung cấp kiểm thử', role: 'supplier', email: '', phone: '', active: true }] });
  await command(page.request, '/items', { id: 'service', code: 'DV1', name: 'Dịch vụ kiểm thử', unit: 'lần', tracked: false, active: true });
  await test.step('gõ tiền liên tục trong số dư không chèn dấu phân cách giữa chừng', async () => {
    await page.goto('/reports?tab=trial');
    await page.getByRole('button', { name: 'Nhập số dư đầu kỳ', exact: true }).click();
    const debit = page.getByRole('spinbutton', { name: 'Dư Nợ dòng 1', exact: true });
    const credit = page.getByRole('spinbutton', { name: 'Dư Có dòng 1', exact: true });
    for (const [raw, formatted] of [['5000000', '5.000.000'], ['9007199254740993', '9.007.199.254.740.993'], ['9223372036854775807', '9.223.372.036.854.775.807']]) {
      await debit.click();
      await debit.press('ControlOrMeta+A');
      await debit.press('Backspace');
      await credit.click();
      await expect(debit).toHaveValue('');
      await debit.pressSequentially(raw);
      await expect(debit).toHaveValue(raw);
      await credit.click();
      await expect(debit).toHaveValue(formatted);
      await expect(page.getByText('Nhập số tiền VND hợp lệ.', { exact: true })).toHaveCount(0);
      await debit.click();
      await expect(debit).toHaveValue(raw);
      await debit.press('End');
      await debit.press('Backspace');
      await expect(debit).toHaveValue(raw.slice(0, -1));
    }
    for (const invalid of ['1.5', '1e3', '-100', '9223372036854775808']) {
      await debit.fill(invalid);
      await credit.click();
      await expect(debit).toHaveValue(invalid);
      await expect(page.getByText('Nhập số tiền VND hợp lệ.', { exact: true })).toHaveCount(1);
    }
    await page.getByRole('dialog').getByRole('button', { name: 'Đóng', exact: true }).last().click();
    await page.getByRole('button', { name: 'Bỏ thay đổi', exact: true }).click();
    await expect(page.getByRole('dialog')).toHaveCount(0);
  });
  await page.goto('/automation?kind=sale');
  await page.getByLabel('Mã nguồn / ký hiệu-số chứng từ gốc', { exact: true }).fill('E2E-SALE-1');
  await page.getByLabel('Diễn giải', { exact: true }).fill('Dịch vụ kiểm thử từ giao diện');
  await choose(page, 'Khách hàng / nhà cung cấp', 'Khách kiểm thử');
  await choose(page, 'Đơn giá nhập');
  await choose(page, 'Hàng/dịch vụ 1', 'Dịch vụ kiểm thử');
  await page.getByLabel('Số lượng', { exact: true }).fill('2');
  await page.getByLabel('Đơn giá (VND)', { exact: true }).fill('1000');
  await choose(page, 'VAT (%)');
  await page.getByRole('button', { name: 'Tính và xem trước' }).click();
  await expect(page.getByRole('heading', { name: /Kết quả server tính/ })).toBeVisible();
  await page.getByRole('button', { name: 'Ghi nhận và ghi sổ', exact: true }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Ghi nhận và ghi sổ', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'BH-2026-000001', exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Xem cơ sở tính và chính sách' }).click();
  await expect(page.getByRole('dialog').filter({ hasText: 'Cơ sở tính đã lưu cùng chứng từ' }).getByText('Tổng thanh toán', { exact: true })).toBeVisible();
  await page.reload();
  await page.goto('/documents');
  await expect(page.getByRole('button', { name: 'BH-2026-000001', exact: true })).toBeVisible();
  expect(errors).toEqual([]);
});

test('gợi ý đơn vị, diễn giải và danh mục không dấu trên biểu mẫu mua hàng responsive', async ({ page }, testInfo) => {
  await login(page);
  await page.goto('/automation?kind=purchase');
  await page.getByRole('tab', { name: 'Hàng hóa & dịch vụ', exact: true }).click();
  await page.getByRole('button', { name: 'Thêm hàng hóa / dịch vụ', exact: true }).click();
  const unit = page.getByLabel('Đơn vị', { exact: true });
  await unit.fill('lan');
  await page.locator('.ant-select-dropdown:visible .ant-select-item-option').filter({ hasText: 'lần' }).click();
  await expect(unit).toHaveValue('lần');
  await page.locator('.ant-modal-close').click();
  await page.goto('/automation?kind=purchase');
  const description = page.getByLabel('Diễn giải', { exact: true });
  await description.fill('dich vu kiem thu');
  await page.locator('.ant-select-dropdown:visible .ant-select-item-option').filter({ hasText: 'Dịch vụ kiểm thử từ giao diện' }).click();
  await expect(description).toHaveValue('Dịch vụ kiểm thử từ giao diện');
  const partner = page.getByLabel('Khách hàng / nhà cung cấp', { exact: true });
  await partner.fill('nha cung cap');
  await page.locator('.ant-select-dropdown:visible .ant-select-item-option').filter({ hasText: 'NCC1' }).click();
  await expect(partner.locator('xpath=ancestor::*[contains(@class,"ant-select")][last()]')).toContainText('NCC1 · Nhà cung cấp kiểm thử');
  for (const width of [1440, 820, 390]) {
    await page.setViewportSize({ width, height: 1000 });
    await expect(page.getByRole('banner').getByRole('button', { name: 'Thiết lập công ty', exact: true })).toBeInViewport();
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    const row = page.getByLabel('Hàng/dịch vụ 1', { exact: true }).locator('xpath=ancestor::*[contains(@class,"tradeRow")]');
    expect(await row.evaluate(element => element.scrollWidth <= element.clientWidth)).toBe(true);
    await page.screenshot({ path: testInfo.outputPath(`purchase-${width}.png`), fullPage: true });
  }
});

test('mất phản hồi sau commit rồi retry không tạo hai nháp; ghi sổ tiền lớn', async ({ page }) => {
  await login(page);
  await page.goto('/documents');
  await page.getByRole('button', { name: 'Tạo chứng từ', exact: true }).click();
  await page.getByLabel('Diễn giải', { exact: true }).fill('Lưu tiền lớn qua API thật');
  await choose(page, 'TK Nợ dòng 1', '111');
  await choose(page, 'TK Có dòng 1', '411');
  await page.getByRole('spinbutton', { name: 'Số tiền dòng 1', exact: true }).fill('9007199254740993');
  await page.route('**/api/v1/documents', async route => {
    if (route.request().method() !== 'POST') { await route.continue(); return; }
    const response = await route.fetch(); expect(response.ok()).toBe(true);
    await route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ code: 'NETWORK_ERROR', message: 'Không nhận được phản hồi sau khi lưu. Thử lại cùng nội dung.' }) }); await page.unroute('**/api/v1/documents');
  });
  await page.getByRole('button', { name: 'Lưu nháp', exact: true }).click();
  await expect(page.getByText('Không nhận được phản hồi sau khi lưu. Thử lại cùng nội dung.')).toBeVisible();
  const retry = page.getByRole('button').filter({ hasText: /^Lưu nháp$/ });
  await expect(retry).not.toHaveClass(/ant-btn-loading/);
  await expect(retry).toBeEnabled();
  await retry.click();
  await expect(page.getByText('Đã lưu nháp vào cơ sở dữ liệu. Kiểm tra chi tiết trước khi ghi sổ.')).toBeVisible();
  await page.getByRole('button', { name: 'Ghi sổ', exact: true }).click();
  await page.getByRole('dialog').filter({ hasText: 'Ghi sổ chứng từ?' }).getByRole('button', { name: 'Ghi sổ', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'CT-2026-000001', exact: true })).toBeVisible();
  const list = await (await page.request.get('/api/v1/documents?period=2026-10')).json();
  expect(list.total).toBe(2);
  expect(list.documents.filter((item: { description: string }) => item.description === 'Lưu tiền lớn qua API thật')).toHaveLength(1);
  expect(list.documents.find((item: { reference: string }) => item.reference === 'CT-2026-000001').lines[0].debit).toBe('9007199254740993');
});

test('cài đặt chính sách, danh sách sổ phân trang và tải backup qua backend thật', async ({ page }, testInfo) => {
  await login(page);
  await page.goto('/settings');
  await expect(page.getByRole('heading', { name: 'Cài đặt doanh nghiệp', exact: true })).toBeVisible();
  await expect(page.getByRole('cell', { name: '2026-01-01', exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Thêm phiên bản chính sách', exact: true }).click();
  await page.getByLabel('Ngày hiệu lực', { exact: true }).fill('01/11/2026');
  await page.getByLabel('Ngày hiệu lực', { exact: true }).press('Enter');
  await page.getByRole('checkbox', { name: /Tôi xác nhận các thông số/ }).check();
  await page.getByRole('button', { name: 'Lưu phiên bản chính sách', exact: true }).click();
  await expect(page.getByRole('cell', { name: '2026-11-01', exact: true })).toBeVisible();
  await page.getByRole('tab', { name: 'Kỳ kế toán', exact: true }).click();
  await expect(page.getByRole('cell', { name: '2026-10', exact: true })).toBeVisible();
  await page.getByRole('tab', { name: 'Sao lưu & khôi phục', exact: true }).click();
  await page.getByRole('button', { name: 'Tạo bản sao', exact: true }).click();
  await page.getByRole('button', { name: 'Tạo và kiểm tra bản sao', exact: true }).click();
  await expect(page.getByRole('dialog', { name: 'Tạo bản sao lưu', exact: true })).toHaveCount(0);
  const download = page.waitForEvent('download');
  await page.getByRole('button', { name: 'Tải bản sao', exact: true }).first().click();
  const backup = await download;
  expect(backup.suggestedFilename()).toMatch(/^oanh-backup-/);
  backupPath = testInfo.outputPath('verified-backup.zip');
  await backup.saveAs(backupPath);
  await page.goto('/reports?tab=trial');
  await expect(page.getByText('Tổng toàn bộ tập lọc', { exact: true })).toBeVisible();
  const report = await (await page.request.get('/api/v1/reports?period=2026-10&kind=trial&pageSize=100')).json();
  expect(report.debit).toBe(report.credit);
  expect(report.closingDebit).toBe(report.closingCredit);
  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Tổng quan', exact: true })).toBeVisible();
  await page.screenshot({ path: '/home/errors/.codex/visualizations/2026/10/21/01a15ccc-685b-7d21-9e71-064f808bf9e2/live-desktop.png', fullPage: true });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Tổng quan', exact: true })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: '/home/errors/.codex/visualizations/2026/10/21/01a15ccc-685b-7d21-9e71-064f808bf9e2/live-mobile.png', fullPage: true });
});

test('khôi phục qua giao diện đổi epoch, thu hồi phiên và bỏ dữ liệu sau snapshot', async ({ page }) => {
  await login(page);
  await command(page.request, '/partners', { partners: [{ id: 'after-backup', code: 'AFTER', name: 'Đối tượng sau snapshot', role: 'customer', email: '', phone: '', active: true }] });
  await page.goto('/settings');
  await page.getByRole('tab', { name: 'Sao lưu & khôi phục', exact: true }).click();
  await page.getByRole('button', { name: 'Khôi phục dữ liệu', exact: true }).click();
  await page.locator('input[type="file"]').setInputFiles(backupPath);
  await expect(page.getByText('Gói đã đạt kiểm tra toàn vẹn và đối soát', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Tiếp tục xác nhận', exact: true }).click();
  const restore = page.getByRole('dialog', { name: 'Khôi phục dữ liệu', exact: true });
  await expect(restore.getByRole('button', { name: 'Khôi phục dữ liệu', exact: true })).toBeDisabled();
  await page.getByRole('textbox', { name: 'Lý do khôi phục', exact: true }).fill('Đối chiếu bản sao kiểm thử');
  await page.getByRole('textbox', { name: 'Tên doanh nghiệp xác nhận khôi phục', exact: true }).fill('Doanh nghiệp E2E');
  await page.getByRole('checkbox', { name: /Tôi đã đối chiếu bản sao/ }).check();
  await restore.getByRole('button', { name: 'Khôi phục dữ liệu', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Chào mừng trở lại', exact: true })).toBeVisible();
  await page.getByLabel('Tên đăng nhập', { exact: true }).fill(credentials.username);
  await page.getByLabel('Mật khẩu', { exact: true }).fill(credentials.password);
  await page.getByRole('button', { name: 'Đăng nhập', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Tổng quan', exact: true })).toBeVisible();
  const session = await (await page.request.get('/api/v1/session')).json();
  expect(session.context.epoch).not.toBe(epoch);
  epoch = session.context.epoch;
  const workspace = await (await page.request.get('/api/v1/workspace?period=2026-10')).json();
  expect(workspace.partners.some((item: { id: string }) => item.id === 'after-backup')).toBe(false);
  await page.goto('/catalog');
  await expect(page.getByText('Đối tượng sau snapshot', { exact: true })).toHaveCount(0);
});

test('thu công nợ theo chứng từ, đảo phân bổ và chuyển tiền qua giao diện', async ({ page }, testInfo) => {
  await login(page);
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message));
  await page.goto('/sales');
  await page.getByRole('button', { name: 'Công nợ theo chứng từ', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Công nợ theo chứng từ', exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Thu tiền', exact: true }).click();
  await page.getByLabel('Mã nguồn / ký hiệu-số chứng từ gốc', { exact: true }).fill('E2E-RECEIPT');
  await page.getByLabel('Diễn giải', { exact: true }).fill('Thu một phần công nợ từ giao diện');
  await choose(page, 'Tài khoản thu/chi', '1121');
  await page.getByRole('spinbutton', { name: 'Số tiền thu/chi', exact: true }).fill('500');
  await page.getByRole('spinbutton', { name: 'Số phân bổ 1', exact: true }).fill('500');
  await page.getByRole('button', { name: 'Tính và xem trước', exact: true }).click();
  await expect(page.getByRole('heading', { name: /Kết quả server tính/ })).toBeVisible();
  const receiptResponse = page.waitForResponse(response => response.url().endsWith('/api/v1/operations') && response.request().method() === 'POST');
  await page.getByRole('button', { name: 'Ghi nhận và ghi sổ', exact: true }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Ghi nhận và ghi sổ', exact: true }).click();
  const receipt = await (await receiptResponse).json();
  await expect(page.getByRole('heading', { name: receipt.document.reference, exact: true })).toBeVisible();
  const invoices = await (await page.request.get('/api/v1/trade-invoices?kind=sale&asOf=2026-10-21')).json();
  expect(invoices.paid).toBe('500'); expect(invoices.remaining).toBe('1500');
  await page.getByRole('button', { name: 'Đảo phiếu tiền', exact: true }).click();
  await page.getByLabel('Mã nguồn / ký hiệu-số chứng từ gốc', { exact: true }).fill('E2E-REVERSE-RECEIPT');
  await page.getByLabel('Diễn giải', { exact: true }).fill('Đảo phiếu thu lập sai');
  await page.getByRole('button', { name: 'Tính và xem trước', exact: true }).click();
  await expect(page.getByRole('heading', { name: /Kết quả server tính/ })).toBeVisible();
  const reverseResponse = page.waitForResponse(response => response.url().endsWith('/api/v1/operations') && response.request().method() === 'POST');
  await page.getByRole('button', { name: 'Ghi nhận và ghi sổ', exact: true }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Ghi nhận và ghi sổ', exact: true }).click();
  const reversal = await (await reverseResponse).json();
  expect(reversal.document.reversalOf).toBe(receipt.document.id);
  await expect(page.getByRole('heading', { name: reversal.document.reference, exact: true })).toBeVisible();
  const restored = await (await page.request.get('/api/v1/trade-invoices?kind=sale&asOf=2026-10-21')).json();
  expect(restored.paid).toBe('0'); expect(restored.remaining).toBe('2000');
  await page.keyboard.press('Escape');
  await expect(page.locator('.ant-drawer:visible')).toHaveCount(0);
  await page.goto('/cash');
  await page.getByRole('button', { name: 'Chuyển tiền', exact: true }).click();
  await page.getByLabel('Mã nguồn / ký hiệu-số chứng từ gốc', { exact: true }).fill('E2E-TRANSFER');
  await page.getByLabel('Diễn giải', { exact: true }).fill('Nộp tiền mặt vào ngân hàng');
  await choose(page, 'Tài khoản nhận', '1121');
  await page.getByRole('spinbutton', { name: 'Số tiền thu/chi', exact: true }).fill('123');
  await page.getByRole('button', { name: 'Tính và xem trước', exact: true }).click();
  await expect(page.getByRole('heading', { name: /Kết quả server tính/ })).toBeVisible();
  const transferResponse = page.waitForResponse(response => response.url().endsWith('/api/v1/operations') && response.request().method() === 'POST');
  await page.getByRole('button', { name: 'Ghi nhận và ghi sổ', exact: true }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Ghi nhận và ghi sổ', exact: true }).click();
  const transfer = await (await transferResponse).json();
  await expect(page.getByRole('heading', { name: transfer.document.reference, exact: true })).toBeVisible();
  const bank = await (await page.request.get('/api/v1/documents?period=2026-10&account=1121')).json();
  expect(bank.money).toEqual({ opening: '0', receipts: '623', payments: '500', closing: '123' });
  await page.keyboard.press('Escape');
  await expect(page.locator('.ant-drawer:visible')).toHaveCount(0);
  for (const width of [1440, 390]) {
    await page.setViewportSize({ width, height: 900 });
    for (const route of ['/cash', '/bank', '/settlements', '/automation?kind=payment']) {
      await page.goto(route);
      await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
      await expect(page.getByText('Đang cập nhật…', { exact: true })).toHaveCount(0);
      if (route.startsWith('/automation')) await expect(page.getByText('Chính sách v1 · Từ 2026-01-01', { exact: true })).toBeVisible();
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    }
    await page.screenshot({ path: testInfo.outputPath(`treasury-${width}.png`), fullPage: true });
  }
  expect(errors).toEqual([]);
});

test('tài khoản chỉ xem không có thao tác ghi hoặc cơ sở tính lương', async ({ page }) => {
  await login(page);
  await command(page.request, '/users', { id: 'e2e-viewer', username: 'e2e-viewer', name: 'Người chỉ xem', role: 'viewer', active: true, password: 'Synthetic-viewer-2026' });
  await page.getByRole('button', { name: /Người kiểm thử Chủ sở hữu/ }).click();
  await page.getByRole('menuitem', { name: 'Đăng xuất', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Chào mừng trở lại', exact: true })).toBeVisible();
  await page.getByLabel('Tên đăng nhập', { exact: true }).fill('e2e-viewer');
  await page.getByLabel('Mật khẩu', { exact: true }).fill('Synthetic-viewer-2026');
  await page.getByRole('button', { name: 'Đăng nhập', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Tổng quan', exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Tạo chứng từ', exact: true })).toHaveCount(0);
  await page.goto('/documents');
  await expect(page.getByRole('button', { name: 'Tạo chứng từ', exact: true })).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Nhập từ Excel', exact: true })).toHaveCount(0);
  const response = await page.request.post('/api/v1/periods', { data: { period: '2026-12', closed: false }, headers: { 'X-Dataset-Epoch': epoch, 'Idempotency-Key': 'viewer-blocked' } });
  expect(response.status()).toBe(403);
  expect((await page.request.get('/api/v1/automation')).status()).toBe(403);
  await page.goto('/settings');
  await expect(page.getByText('Tài khoản chỉ xem không có quyền truy cập cấu hình và dữ liệu nhân sự.', { exact: true })).toBeVisible();
});


test('tab phân hệ giữ nguyên khi mở danh mục, công nợ và các báo cáo', async ({ page }) => {
  await login(page);
  for (const [module, title, partner] of [['purchases', 'Mua hàng', 'Nhà cung cấp'], ['sales', 'Bán hàng', 'Khách hàng']]) {
    await page.goto(`/${module}`);
    const tabs = page.getByRole('navigation', { name: 'Điều hướng phân hệ', exact: true });
    await page.getByLabel('Tìm trong danh sách chứng từ', { exact: true }).fill('giữ bộ lọc');
    for (const [label, path] of [['Công nợ theo chứng từ', 'settlements'], [partner, 'partners'], ['Hàng hóa, dịch vụ', 'items'], ['Báo cáo', 'reports']]) {
      await tabs.getByRole('button', { name: label, exact: true }).click();
      await expect(page).toHaveURL(new RegExp(`/${module}/${path}$`));
      await expect(tabs.getByRole('button', { name: title, exact: true })).toBeVisible();
      await expect(tabs.getByRole('button', { name: label, exact: true })).toHaveAttribute('aria-current', 'page');
      await expect(page.getByRole('navigation', { name: 'Điều hướng chính', exact: true }).getByRole('link', { name: title, exact: true })).toHaveAttribute('aria-current', 'page');
      if (path === 'settlements') {
        const controls = page.getByRole('group', { name: 'Công cụ công nợ', exact: true }).locator('.ant-select, .ant-picker');
        await expect(controls).toHaveCount(3);
        const boxes = await controls.evaluateAll(elements => elements.map(element => { const rect = element.getBoundingClientRect(); return { top: rect.top, height: rect.height }; }));
        expect(boxes.map(box => box.height)).toEqual([32, 32, 32]);
        expect(boxes.map(box => box.top)).toEqual([boxes[0].top, boxes[0].top, boxes[0].top]);
      }
    }
    await page.getByRole('tab', { name: 'Báo cáo tài chính', exact: true }).click();
    await expect(page).toHaveURL(new RegExp(`/${module}/reports\\?tab=financial$`));
    await expect(tabs).toBeVisible();
    await page.getByRole('tab', { name: 'Cân đối phát sinh', exact: true }).click();
    await expect(page).toHaveURL(new RegExp(`/${module}/reports\\?tab=trial$`));
    await tabs.getByRole('button', { name: title, exact: true }).click();
    await expect(page.getByLabel('Tìm trong danh sách chứng từ', { exact: true })).toHaveValue('giữ bộ lọc');
    await tabs.getByRole('button', { name: 'Báo cáo', exact: true }).click();
    await expect(page.getByRole('tab', { name: 'Cân đối phát sinh', exact: true })).toHaveAttribute('aria-selected', 'true');
    await page.reload();
    await expect(tabs.getByRole('button', { name: 'Báo cáo', exact: true })).toHaveAttribute('aria-current', 'page');
    await expect(page.getByText('Không gian làm việc', { exact: true })).toHaveCount(0);
    await expect(page.getByText(/^Cân đối Nợ\/Có chỉ là một bước kiểm tra\./)).toHaveCount(0);
    await expect(page.getByText(/^Dữ liệu doanh nghiệp · Kỳ /)).toHaveCount(0);
    await expect(page.locator('.ant-pagination')).toBeVisible();
    const reportTools = page.getByRole('group', { name: 'Công cụ báo cáo', exact: true });
    await expect(reportTools.getByRole('textbox', { name: 'Tìm trong báo cáo', exact: true })).toBeVisible();
    await expect(reportTools.getByRole('button', { name: 'Xuất tệp', exact: true })).toBeVisible();
  }
  await page.goto('/catalog');
  await page.getByRole('tab', { name: 'Hệ thống tài khoản', exact: true }).click();
  await expect(page.getByRole('tab', { name: 'Tài khoản từ Google Sheet', exact: true })).toHaveCount(0);
  await expect(page.getByRole('cell', { name: '1281', exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Thu gọn tất cả tài khoản', exact: true }).click();
  await expect(page.getByRole('cell', { name: '1281', exact: true })).toHaveCount(0);
  await page.getByLabel('Tìm trong danh mục', { exact: true }).fill('1331');
  await expect(page.getByText('133', { exact: true })).toBeVisible();
  await expect(page.getByRole('cell', { name: '1331', exact: true })).toBeVisible();

});


test('nhập Excel ghép cột, chặn lỗi và lưu nháp đúng một lần', async ({ page }) => {
  await login(page);
  await page.goto('/imports');
  const file = (account: string) => Buffer.from(`Mã chứng từ nguồn,Ngày hạch toán,Diễn giải,TK Nợ,TK Có,Số tiền\nE2E-EXCEL-1,21/10/2026,Nhập tiền lớn,${account},411,9007199254740993\n`);
  await page.locator('input[type="file"]').setInputFiles({ name: 'invalid.csv', mimeType: 'text/csv', buffer: file('SAI') });
  await expect(page.getByText('invalid.csv', { exact: true }).first()).toBeVisible();
  await page.getByRole('button', { name: 'Tiếp tục', exact: true }).first().click();
  await expect(page.getByLabel('Ghép cột TK Có', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Kiểm tra dữ liệu', exact: true }).click();
  await expect(page.getByText('Có 1 lỗi cần sửa · chưa nhập dữ liệu', { exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Nhập dữ liệu', exact: true })).toBeDisabled();
  await page.getByRole('button', { name: 'Quay lại', exact: true }).click();
  await page.getByRole('button', { name: 'Quay lại', exact: true }).click();
  await page.locator('input[type="file"]').setInputFiles({ name: 'valid.csv', mimeType: 'text/csv', buffer: file('111') });
  await expect(page.getByText('valid.csv', { exact: true }).first()).toBeVisible();
  await page.getByRole('button', { name: 'Tiếp tục', exact: true }).first().click();
  await page.getByRole('button', { name: 'Kiểm tra dữ liệu', exact: true }).click();
  await expect(page.getByText('Dữ liệu hợp lệ · sẵn sàng nhập', { exact: true })).toBeVisible();
  await expect(page.getByText('9.007.199.254.740.993', { exact: true }).first()).toBeVisible();
  await page.getByRole('button', { name: 'Nhập dữ liệu', exact: true }).click();
  await expect(page.getByText('Đã nhập 1 bản ghi', { exact: true })).toBeVisible();
  await page.reload();
  await expect(page.getByText('Đã nhập', { exact: true })).toBeVisible();
  const documents = await (await page.request.get(`/api/v1/documents?${new URLSearchParams({ period: '2026-10', query: 'Nhập tiền lớn' })}`)).json();
  expect(documents.documents.filter((doc: { sourceId: string }) => doc.sourceId === 'E2E-EXCEL-1')).toHaveLength(1);
});

test('menu chức năng có nhãn, mở đúng thu chi hóa đơn, Excel và dịch vụ', async ({ page }) => {
  await login(page);
  await page.goto('/bank');
  for (const name of ['Thu tiền', 'Chi tiền']) await expect(page.getByRole('button', { name, exact: true })).toHaveText(name);
  await page.getByRole('button', { name: 'Tùy chọn Chi tiền', exact: true }).click();
  await page.getByRole('menuitem', { name: 'Trả tiền theo hóa đơn', exact: true }).click();
  await expect(page).toHaveURL(/kind=payment&account=1121&settlement=1$/);
  await expect(page.getByText('Trả công nợ theo chứng từ', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Đóng chứng từ', exact: true }).click();
  await expect(page).toHaveURL(/\/bank$/);
  for (const [action, kind] of [['Chi tiền', 'Phiếu chi, chi tiền gửi'], ['Thu tiền', 'Phiếu thu, thu tiền gửi']]) {
    await page.getByRole('button', { name: `Tùy chọn ${action}`, exact: true }).click();
    await page.getByRole('menuitem', { name: 'Nhập từ Excel', exact: true }).click();
    await expect(page).toHaveURL(/\/bank$/);
    await expect(page.getByRole('dialog').getByText(kind, { exact: true })).toBeVisible();
    await page.getByRole('dialog').getByRole('button', { name: 'Đóng', exact: true }).last().click();
  }
  for (const [path, action, service] of [['/purchases', 'Mua hàng', 'Mua dịch vụ'], ['/sales', 'Bán hàng', 'Bán dịch vụ']]) {
    await page.goto(path);
    await page.getByRole('button', { name: `Tùy chọn ${action}`, exact: true }).click();
    await page.getByRole('menuitem', { name: service, exact: true }).click();
    await expect(page).toHaveURL(/service=1$/);
    await expect(page.getByRole('heading', { name: `Chứng từ ${service.toLowerCase()}`, exact: true })).toBeVisible();
    await page.getByRole('button', { name: 'Đóng chứng từ', exact: true }).click();
  }
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('/bank');
  const mobileSearch = page.getByRole('textbox', { name: 'Tìm trong danh sách chứng từ', exact: true });
  await expect(mobileSearch).toBeHidden();
  await page.getByRole('button', { name: /^Bộ lọc/ }).click();
  await expect(mobileSearch).toBeVisible();
  await page.getByRole('button', { name: /^Bộ lọc/ }).click();
  await expect(mobileSearch).toBeHidden();
  const receipt = await page.getByRole('button', { name: 'Thu tiền', exact: true }).boundingBox();
  const payment = await page.getByRole('button', { name: 'Chi tiền', exact: true }).boundingBox();
  expect(receipt).not.toBeNull(); expect(payment).not.toBeNull();
  expect(Math.abs(receipt!.y - payment!.y)).toBeLessThan(1);
  await page.getByRole('button', { name: 'Tùy chọn Chi tiền', exact: true }).click();
  await expect(page.getByRole('menuitem', { name: 'Chuyển tiền nội bộ', exact: true })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});


test('tìm nhanh, debounce, Back giữ nháp và phím tắt không tự ghi sổ', async ({ page }) => {
  await login(page);
  await page.goto('/cash?status=draft&sort=amount&direction=asc');
  const requests: string[] = [];
  page.on('request', request => { const url = new URL(request.url()); if (url.pathname === '/api/v1/documents' && url.searchParams.get('q')?.startsWith('uxprobe')) requests.push(url.searchParams.get('q')!); });
  const search = page.getByRole('textbox', { name: 'Tìm trong danh sách chứng từ', exact: true });
  await search.pressSequentially('uxprobe');
  await expect(page).toHaveURL(/q=uxprobe/);
  expect(requests).toEqual(['uxprobe']);
  await search.fill(''); await search.press('Enter');
  await expect(page).not.toHaveURL(/q=/);
  const returnTo = new URL(page.url()).pathname + new URL(page.url()).search;
  await page.getByRole('button', { name: 'Thu tiền', exact: true }).click();
  await page.getByRole('combobox', { name: 'Lý do nộp', exact: false }).fill('Thu kiểm thử thao tác nhanh');
  const amount = page.getByRole('spinbutton', { name: 'Số tiền dòng 1', exact: true });
  await amount.click(); await amount.press('ControlOrMeta+A'); await amount.press('Backspace'); await amount.pressSequentially('9007199254740993');
  await page.goBack();
  await expect(page).toHaveURL(url => url.pathname + url.search === returnTo);
  const leaving = page.waitForEvent('dialog');
  const reload = page.reload().catch(() => undefined);
  const warning = await leaving;
  expect(warning.type()).toBe('beforeunload');
  await warning.dismiss(); await reload;
  await page.getByRole('button', { name: 'Nháp đang nhập (1)', exact: true }).click();
  await page.getByRole('menuitem', { name: /^Phiếu thu/ }).click();
  await expect(amount).toHaveValue('9.007.199.254.740.993');
  await expect(page.getByRole('combobox', { name: 'Lý do nộp', exact: false })).toHaveValue('Thu kiểm thử thao tác nhanh');
  await choose(page, 'Tài khoản đối ứng dòng 1', '4111');
  await amount.click(); await amount.press('ControlOrMeta+A'); await amount.press('Backspace'); await amount.pressSequentially('5000000');
  await amount.press('ControlOrMeta+s');
  await expect(page).toHaveURL(/\/vouchers\/[A-Z2-7]+$/);
  const id = new URL(page.url()).pathname.split('/').at(-1);
  let saved = await (await page.request.get(`/api/v1/documents/${id}/voucher`)).json();
  expect(saved.document.status).toBe('draft');
  expect(saved.input.amount).toBe('5000000');
  await amount.press('ControlOrMeta+Enter');
  await expect(page.getByRole('tab', { name: 'Bút toán server tính', exact: true })).toBeVisible();
  saved = await (await page.request.get(`/api/v1/documents/${id}/voucher`)).json();
  expect(saved.document.status).toBe('draft');
  await page.getByRole('button', { name: 'Đóng chứng từ', exact: true }).click();
  await expect(page).toHaveURL(url => url.pathname + url.search === returnTo);
  await search.press('ControlOrMeta+k');
  const palette = page.getByRole('dialog', { name: 'Tìm nhanh', exact: true });
  await expect(palette).toBeVisible();
  const quick = palette.getByRole('combobox', { name: 'Tìm chứng từ trong kỳ', exact: true });
  await quick.fill('chi tien'); await quick.press('ArrowDown'); await quick.press('Enter');
  await expect(page).toHaveURL(/kind=payment&account=1121/);
  await page.getByRole('button', { name: 'Đóng chứng từ', exact: true }).click();
});


test('menu xuất Excel, CSV và bản in giữ toàn bộ bộ lọc trong phân hệ', async ({ page }) => {
  await login(page);
  await page.goto('/sales');
  const exportButton = page.getByRole('button', { name: 'Xuất tệp', exact: true });
  await exportButton.press('ArrowDown');
  await expect(page.getByRole('menuitem', { name: /Excel/ })).toBeFocused();
  const xlsx = page.waitForEvent('download');
  await page.getByRole('menuitem', { name: /Excel/ }).click();
  expect((await xlsx).suggestedFilename()).toBe('chung-tu-2026-10.xlsx');
  await exportButton.click();
  const csv = page.waitForEvent('download');
  await page.getByRole('menuitem', { name: /CSV/ }).click();
  expect((await csv).suggestedFilename()).toBe('chung-tu-2026-10.csv');
  await exportButton.click();
  const popup = page.context().waitForEvent('page');
  await page.getByRole('menuitem', { name: /Bản in/ }).click();
  const print = await popup;
  await expect(print.getByRole('button', { name: 'In / Lưu PDF', exact: true })).toBeVisible();
  await expect(print.getByRole('heading', { name: 'Danh sách chứng từ', exact: true })).toBeVisible();
  await expect(page).toHaveURL(/\/sales$/);
  await print.close();
});

test('nhật ký dễ đọc, xem trước và khôi phục có liên kết', async ({ page }) => {
  await login(page);
  const session = await (await page.request.get('/api/v1/session')).json(); epoch = session.context.epoch;
  const customer = { id: 'audit-e2e-customer', code: 'AUDITE2E', name: 'Khách hàng kiểm thử lịch sử', role: 'customer', email: 'before@example.invalid', phone: '', active: true };
  await command(page.request, '/partners', { partners: [customer] });
  await command(page.request, '/partners', { partners: [{ ...customer, email: 'after@example.invalid' }] });
  const events = await (await page.request.get('/api/v1/audit/events?q=AUDITE2E')).json();
  const event = events.items[0];
  await page.goto('/settings?tab=audit');
  await page.getByRole('textbox', { name: 'Tìm nhật ký thao tác', exact: true }).fill('AUDITE2E');
  await expect(page.getByText('2 thao tác', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: `Xem chi tiết thao tác #${event.id}`, exact: true }).click();
  await expect(page.getByText('before@example.invalid', { exact: true })).toBeVisible();
  await expect(page.getByText('after@example.invalid', { exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Xem trước khôi phục', exact: true })).toBeDisabled();
  await page.getByRole('textbox', { name: 'Lý do khôi phục', exact: true }).fill('Sửa nhầm email trong kiểm thử');
  await page.getByRole('button', { name: 'Xem trước khôi phục', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Bản xem trước · Chưa áp dụng', exact: true })).toBeVisible();
  const unchanged = await (await page.request.get(`/api/v1/audit/detail?id=${event.id}`)).json();
  expect(unchanged.restoredBy).toBeUndefined();
  await page.getByRole('button', { name: 'Xác nhận khôi phục', exact: true }).click();
  await page.getByRole('dialog').filter({ hasText: 'Xác nhận khôi phục thao tác?' }).getByRole('button', { name: 'Xác nhận khôi phục', exact: true }).click();
  await expect(page.getByRole('button', { name: `Thao tác gốc · #${event.id}`, exact: true })).toBeVisible();
  const restored = await (await page.request.get(`/api/v1/audit/detail?id=${event.id}`)).json();
  expect(restored.restoredBy).toBeTruthy();
  const recovery = await (await page.request.get(`/api/v1/audit/detail?id=${restored.restoredBy}`)).json();
  expect(recovery.restoredFrom).toBe(event.id);
  expect(recovery.changes[0].after.email).toBe('before@example.invalid');
});

test('ngân hàng lọc theo tài khoản hạch toán và đổi tài khoản xóa lựa chọn cũ', async ({ page }) => {
  await login(page);
  epoch = (await (await page.request.get('/api/v1/session')).json()).context.epoch;
  for (const [id, number, accountId] of [['filter-bank-a', 'E2E-1121-A', '1121'], ['filter-bank-b', 'E2E-1121-B', '1121'], ['filter-bank-other', 'E2E-112-OTHER', '112']]) {
    await command(page.request, '/bank-accounts', { id, number, accountId, bankName: 'Ngân hàng kiểm thử bộ lọc', active: true });
  }
  await page.goto('/vouchers/new?kind=transfer');
  const dropdown = page.locator('.ant-select-dropdown:visible');
  const source = page.getByRole('combobox', { name: 'Tài khoản hạch toán chuyển đi', exact: true });
  await source.click();
  await dropdown.locator('.ant-select-item-option').filter({ hasText: '1121 ·' }).click();
  const sourceBank = page.getByRole('combobox', { name: 'Tài khoản ngân hàng chuyển đi', exact: true });
  await sourceBank.click();
  await expect(dropdown).toContainText('E2E-1121-A');
  await expect(dropdown).toContainText('E2E-1121-B');
  await expect(dropdown).not.toContainText('E2E-112-OTHER');
  await dropdown.locator('.ant-select-item-option').filter({ hasText: 'E2E-1121-A' }).click();
  await source.click();
  await dropdown.locator('.ant-select-item-option').filter({ hasText: '112 ·' }).click();
  await expect(page.locator('.ant-select-selection-item').filter({ hasText: 'E2E-1121-A' })).toHaveCount(0);
  await sourceBank.click();
  await expect(dropdown).toContainText('E2E-112-OTHER');
  await expect(dropdown).not.toContainText('E2E-1121-A');
  await expect(dropdown).not.toContainText('E2E-1121-B');
  await sourceBank.press('Escape');
  const destination = page.getByRole('combobox', { name: 'Tài khoản ngân hàng chuyển đến', exact: true });
  await destination.click();
  await expect(dropdown).toContainText('E2E-1121-A');
  await expect(dropdown).not.toContainText('E2E-112-OTHER');
  await destination.press('Escape');
});


test('nháp riêng theo chức năng: chi tiền, bán hàng, dịch vụ và tiền gửi', async ({ page }) => {
  await login(page);
  await page.goto('/cash');
  const navigation = page.getByRole('navigation', { name: 'Điều hướng chính' });
  const close = () => page.getByRole('button', { name: 'Đóng chứng từ', exact: true }).click();
  await page.getByRole('button', { name: 'Chi tiền', exact: true }).click();
  await page.getByRole('combobox', { name: /Lý do chi/ }).fill('Nháp chi riêng');
  await page.getByRole('spinbutton', { name: 'Số tiền dòng 1', exact: true }).fill('5000000');
  await close();
  await navigation.getByRole('link', { name: 'Bán hàng', exact: true }).click();
  await expect(page.getByRole('button', { name: /Nháp đang nhập/ })).toHaveCount(0);
  await page.getByRole('group', { name: 'Bán hàng và chức năng liên quan' }).getByRole('button', { name: 'Bán hàng', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Chứng từ bán hàng', exact: true })).toBeVisible();
  const description = page.getByRole('combobox', { name: /Diễn giải/ });
  await expect(description).toHaveValue('');
  await description.fill('Nháp bán hàng riêng');
  await close();
  await navigation.getByRole('link', { name: 'Tiền mặt', exact: true }).click();
  await page.getByRole('button', { name: 'Chi tiền', exact: true }).click();
  await expect(page.getByRole('combobox', { name: /Lý do chi/ })).toHaveValue('Nháp chi riêng');
  await expect(page.getByRole('spinbutton', { name: 'Số tiền dòng 1', exact: true })).toHaveValue('5.000.000');
  await page.getByRole('combobox', { name: 'Loại phiếu', exact: true }).click();
  await page.locator('.ant-select-dropdown:visible .ant-select-item-option').filter({ hasText: 'Phiếu thu' }).click();
  await expect(page.getByRole('heading', { name: 'Phiếu thu', exact: true })).toBeVisible();
  await expect(page.getByRole('combobox', { name: /Lý do nộp/ })).toHaveValue('');
  await close();
  await navigation.getByRole('link', { name: 'Tiền gửi', exact: true }).click();
  await page.getByRole('button', { name: 'Chi tiền', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Chi tiền gửi', exact: true })).toBeVisible();
  await expect(page.getByRole('combobox', { name: /Lý do chi/ })).toHaveValue('');
  await close();
  await navigation.getByRole('link', { name: 'Bán hàng', exact: true }).click();
  await page.getByRole('button', { name: 'Nháp đang nhập (1)', exact: true }).click();
  await page.getByRole('menuitem', { name: 'Bán hàng · Nháp bán hàng riêng', exact: true }).click();
  await expect(description).toHaveValue('Nháp bán hàng riêng');
  await page.getByRole('combobox', { name: 'Loại hàng hóa dịch vụ', exact: true }).click();
  await page.locator('.ant-select-dropdown:visible .ant-select-item-option').filter({ hasText: 'Bán dịch vụ' }).click();
  await expect(page.getByRole('heading', { name: 'Chứng từ bán dịch vụ', exact: true })).toBeVisible();
  await expect(description).toHaveValue('');
  await description.fill('Nháp bán dịch vụ riêng');
  await close();
  await page.getByRole('button', { name: 'Nháp đang nhập (2)', exact: true }).click();
  await expect(page.getByRole('menuitem', { name: 'Bán hàng · Nháp bán hàng riêng', exact: true })).toBeVisible();
  await expect(page.getByRole('menuitem', { name: 'Bán dịch vụ · Nháp bán dịch vụ riêng', exact: true })).toBeVisible();
});
