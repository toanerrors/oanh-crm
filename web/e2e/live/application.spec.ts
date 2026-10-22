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
  await page.getByRole('tab', { name: 'Backup / khôi phục', exact: true }).click();
  const download = page.waitForEvent('download');
  await page.getByRole('button', { name: 'Tạo và tải backup', exact: true }).click();
  const backup = await download;
  expect(backup.suggestedFilename()).toBe('backup-2026-10.zip');
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
  await page.getByRole('tab', { name: 'Backup / khôi phục', exact: true }).click();
  await page.locator('input[type="file"]').setInputFiles(backupPath);
  await expect(page.getByText('Kết quả kiểm tra gói', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Khôi phục từ gói đã kiểm tra', exact: true }).click();
  await page.getByRole('button', { name: 'Khôi phục dữ liệu', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Chào mừng trở lại', exact: true })).toBeVisible();
  await page.getByLabel('Tên đăng nhập', { exact: true }).fill(credentials.username);
  await page.getByLabel('Mật khẩu', { exact: true }).fill(credentials.password);
  await page.getByRole('button', { name: 'Đăng nhập', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Cài đặt doanh nghiệp', exact: true })).toBeVisible();
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
  await expect(page.getByRole('button', { name: 'Nhập chứng từ CSV', exact: true })).toHaveCount(0);
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
    await expect(reportTools.getByRole('button', { name: 'In báo cáo', exact: true })).toBeVisible();
    await expect(reportTools.getByRole('button', { name: 'Xuất CSV', exact: true })).toBeVisible();
  }
});
