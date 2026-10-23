import { expect, test } from '@playwright/test';
import type { Page } from '@playwright/test';

async function choose(page: Page, label: string, text: string) {
  const input = page.getByLabel(label, { exact: true });
  await input.click();
  if (await input.isEditable()) await input.fill(text);
  await page.locator('.ant-select-dropdown:visible .ant-select-item-option').filter({ hasText: text }).first().click();
}

test('chọn tài khoản mua hàng, lưu nháp và ghi sổ giữ đúng tài khoản', async ({ page }) => {
  await page.clock.setFixedTime(new Date('2026-12-03T03:00:00Z'));
  const credentials = { username: 'e2e-owner', password: 'Synthetic-test-only-2026' };
  const session = await (await page.request.get('/api/v1/session')).json();
  if (session.needsSetup) {
    const response = await page.request.post('/api/v1/setup', { data: { ...credentials, name: 'Người kiểm thử', companyName: 'Doanh nghiệp E2E', regime: 'Chế độ kiểm thử', confirmAccounts: true } });
    expect(response.ok(), await response.text()).toBe(true);
  }
  const loggedIn = await page.request.post('/api/v1/auth/login', { data: credentials });
  expect(loggedIn.ok(), await loggedIn.text()).toBe(true);
  const { context } = await (await page.request.get('/api/v1/session')).json();
  async function command(path: string, data: unknown) {
    const response = await page.request.post(`/api/v1${path}`, { data, headers: { 'X-Dataset-Epoch': context.epoch, 'Idempotency-Key': crypto.randomUUID() } });
    expect(response.ok(), await response.text()).toBe(true);
  }
  await command('/periods', { period: '2026-12', closed: false });
  await command('/policies', { regime: 'Chế độ kiểm thử', effectiveFrom: '2026-12-01', confirmed: true, rounding: 'half_up', vatMethod: 'deduction', vatRates: ['0', '10'], inventoryMethod: 'fifo', quantityScale: 2, standardDays: '26', hoursPerDay: '8', personalDeduction: '100', dependentDeduction: '50', corporateTaxRate: '20', insurance: [{ name: 'Khoản BH thử', employeeRate: '10', employerRate: '20' }], pitBrackets: [{ upTo: '1000', rate: '5' }, { upTo: '', rate: '10' }], depreciationMethod: 'straight_line', residualMethod: 'last_period' });
  await command('/partners', { partners: [{ id: 'account-supplier', code: 'NCC-TK', name: 'Nhà cung cấp thử tài khoản', role: 'supplier', active: true }] });
  await command('/items', { id: 'account-goods', code: 'HH-TK', name: 'Hàng mua dùng ngay', unit: 'cái', tracked: true, active: true });
  await command('/items', { id: 'account-service', code: 'DV-TK', name: 'Thuê nhà', unit: 'tháng', tracked: false, active: true });
  await page.goto('/vouchers/new?kind=purchase');
  await choose(page, 'Mã nhà cung cấp', 'NCC-TK');
  await page.getByLabel('Diễn giải', { exact: true }).fill('Mua dùng ngay vào chi phí');
  await choose(page, 'Hàng dịch vụ dòng 1', 'HH-TK');
  await page.getByLabel('Đơn giá dòng 1', { exact: true }).fill('1000');
  await choose(page, 'VAT dòng 1', '0');
  await choose(page, 'Khấu trừ VAT dòng 1', 'Có');
  await choose(page, 'Tài khoản dòng 1', '6427');
  await expect(page.getByLabel('Kho dòng 1', { exact: true })).toHaveCount(0);
  await expect(page.getByText('Không nhập kho', { exact: true })).toBeVisible();
  await choose(page, 'Tài khoản dòng 1', '156');
  await expect(page.getByLabel('Kho dòng 1', { exact: true })).toBeVisible();
  await choose(page, 'Tài khoản dòng 1', '6427');
  const savedResponse = page.waitForResponse(response => response.url().endsWith('/api/v1/voucher-drafts') && response.request().method() === 'POST');
  await page.getByRole('button', { name: /Cất nháp/ }).click();
  const saved = await (await savedResponse).json();
  expect(saved.input.lines[0].accountId).toBe('6427');
  await expect(page).toHaveURL(new RegExp(`/vouchers/${saved.document.id}$`));
  await page.reload();
  await expect(page.getByLabel('Tài khoản dòng 1', { exact: true })).toBeVisible();
  await expect(page.getByText('Không nhập kho', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: /Tính và xem trước/ }).click();
  await page.getByRole('button', { name: 'Ghi sổ', exact: true }).click();
  await page.getByRole('dialog').last().getByRole('button', { name: 'Ghi sổ', exact: true }).click();
  await expect(page.getByText('Đã ghi sổ', { exact: true })).toBeVisible();
  const posted = await (await page.request.get(`/api/v1/documents/${saved.document.id}/voucher`)).json();
  expect(posted.document.lines.find((line: { accountId: string }) => line.accountId === '6427').debit).toBe('1000');
  expect(posted.document.lines.some((line: { accountId: string }) => line.accountId === '156')).toBe(false);
  await page.goto('/vouchers/new?kind=purchase&service=1');
  await choose(page, 'Hàng dịch vụ dòng 1', 'DV-TK');
  await choose(page, 'Tài khoản dòng 1', '154');
  await expect(page.getByLabel('Kho dòng 1', { exact: true })).toHaveCount(0);
});
