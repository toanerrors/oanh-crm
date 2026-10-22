import { expect, test } from '@playwright/test';

test('tổng quan, điều hướng SPA và các màn hình chính hiển thị đúng', async ({ page }) => {
  const errors: string[] = [];
  page.on('pageerror', error => errors.push(error.message));
  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Tổng quan', exact: true })).toBeVisible();
  await expect(page.locator('footer')).toHaveCount(0);
  await expect(page.getByRole('link', { name: /Doanh thu trong kỳ/ })).toContainText('528');
  for (const [label, heading] of [['Bán hàng', 'Bán hàng'], ['Mua hàng', 'Mua hàng'], ['Tiền mặt & ngân hàng', 'Tiền mặt & ngân hàng'], ['Hàng tồn kho', 'Hàng tồn kho'], ['Công nợ', 'Công nợ'], ['Sổ sách & báo cáo', 'Sổ sách & báo cáo'], ['Danh mục', 'Danh mục']] as const) {
    await page.getByRole('navigation', { name: 'Điều hướng chính' }).getByRole('link', { name: label, exact: true }).click();
    await expect(page.getByRole('heading', { name: heading, exact: true })).toBeVisible();
    await expect(page.getByText('Không gian làm việc', { exact: true })).toHaveCount(0);
    await expect(page.getByText(/^Cân đối Nợ\/Có chỉ là một bước kiểm tra\./)).toHaveCount(0);
    await expect(page.getByText(/^(Dữ liệu mẫu|Dữ liệu doanh nghiệp) · Kỳ /)).toHaveCount(0);
    if (label === 'Danh mục') {
      const tools = page.getByRole('group', { name: 'Công cụ danh mục', exact: true });
      await expect(tools.getByRole('textbox', { name: 'Tìm trong danh mục', exact: true })).toBeVisible();
      await expect(tools.getByRole('button', { name: 'Thêm đối tượng', exact: true })).toBeVisible();
      const add = tools.getByRole('button', { name: 'Thêm đối tượng', exact: true });
      await expect(add).toHaveText('');
      await add.focus();
      await expect(page.getByRole('tooltip', { name: 'Thêm đối tượng', exact: true })).toBeVisible();
    }
  }
  expect(errors).toEqual([]);
});

test('gợi ý không dấu, nhập tự do và chiều cao control thống nhất', async ({ page }) => {
  await page.goto('/documents');
  const create = page.getByRole('button', { name: 'Tạo chứng từ', exact: true });
  expect((await create.boundingBox())?.height).toBe(32);
  for (const control of [
    page.getByRole('combobox', { name: 'Tìm chứng từ trong kỳ' }).locator('xpath=ancestor::span[contains(@class,"ant-input-affix-wrapper")]'),
    page.getByRole('textbox', { name: 'Tìm trong danh sách chứng từ' }).locator('xpath=ancestor::span[contains(@class,"ant-input-affix-wrapper")]'),
    page.getByRole('combobox', { name: 'Chọn kỳ kế toán' }).locator('xpath=ancestor::*[contains(@class,"ant-select")][last()]'),
  ]) expect((await control.boundingBox())?.height).toBe(32);
  await create.click();
  const description = page.getByLabel('Diễn giải', { exact: true });
  await description.fill('thue van phong');
  const suggestion = page.locator('.ant-select-dropdown:visible .ant-select-item-option').filter({ hasText: 'Thanh toán chi phí thuê văn phòng tháng 10' });
  await expect(suggestion).toBeVisible();
  await suggestion.click();
  await expect(description).toHaveValue('Thanh toán chi phí thuê văn phòng tháng 10');
  await description.fill('Diễn giải mới chưa có trong lịch sử');
  await page.getByLabel('Ngày chứng từ', { exact: true }).click();
  await expect(description).toHaveValue('Diễn giải mới chưa có trong lịch sử');

  const partner = page.getByRole('combobox', { name: 'Đối tượng', exact: true });
  await partner.fill('minh anh');
  await expect(page.locator('.ant-select-dropdown:visible .ant-select-item-option').filter({ hasText: 'KH001 · Công ty TNHH Minh Anh' })).toBeVisible();
  await partner.press('Enter');
  const account = page.getByRole('combobox', { name: 'TK Nợ dòng 1' });
  await account.fill('tien mat');
  await expect(page.locator('.ant-select-dropdown:visible .ant-select-item-option').filter({ hasText: '111' })).toBeVisible();
  await account.press('Enter');

  for (const control of [
    page.getByLabel('Ngày chứng từ', { exact: true }),
    description,
    partner.locator('xpath=ancestor::*[contains(@class,"ant-select")][last()]'),
    account.locator('xpath=ancestor::*[contains(@class,"ant-select")][last()]'),
    page.getByRole('spinbutton', { name: 'Số tiền dòng 1', exact: true }).locator('xpath=ancestor::*[contains(@class,"ant-input-number")][last()]'),
    page.getByRole('button', { name: 'Lưu nháp mẫu', exact: true }),
  ]) expect((await control.boundingBox())?.height).toBe(32);

  await page.locator('.ant-drawer-close').click();
  await page.getByRole('button', { name: 'Tiếp tục nháp', exact: true }).click();
  await expect(description).toHaveValue('Diễn giải mới chưa có trong lịch sử');
});

test('tìm nhanh không dấu bằng bàn phím và Enter tra cứu toàn bộ kỳ', async ({ page }) => {
  await page.goto('/');
  const search = page.getByRole('combobox', { name: 'Tìm chứng từ trong kỳ' });
  await search.fill('thue van phong');
  await expect(page.locator('.ant-select-dropdown:visible .ant-select-item-option').filter({ hasText: 'PC-10009' })).toBeVisible();
  await search.press('ArrowDown');
  await search.press('Enter');
  await expect(page.getByRole('heading', { name: 'PC-10009', exact: true })).toBeVisible();
  await page.locator('.ant-drawer-close').click();
  await search.fill('BH-10008');
  await search.press('Enter');
  await expect(page).toHaveURL(/\/documents\?q=BH-10008$/);
  await expect(page.getByRole('button', { name: 'BH-10008', exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'PC-10009', exact: true })).toHaveCount(0);
});

test('tìm kiếm mobile và thẻ chứng từ hiển thị đủ tiền, đối tượng, trạng thái', async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 844 });
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.goto('/documents');
  await expect(page.getByText('185.000.000', { exact: true })).toBeVisible();
  await expect(page.getByText('Công ty TNHH Minh Anh', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Mở tìm kiếm chứng từ', exact: true }).click();
  const search = page.getByRole('dialog').getByRole('combobox', { name: 'Tìm chứng từ trong kỳ' });
  await expect(search).toBeFocused();
  await search.fill('thue van phong');
  await page.locator('.ant-select-dropdown:visible .ant-select-item-option').filter({ hasText: 'PC-10009' }).click();
  await expect(page.getByRole('heading', { name: 'PC-10009', exact: true })).toBeVisible();
  await page.locator('.ant-drawer-close').click();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  expect(await page.locator('.ant-table-content').evaluate(element => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.getByRole('listitem', { name: '2', exact: true }).click();
  await expect(page.getByRole('button', { name: 'BH-10006', exact: true })).toBeVisible();
  await page.getByRole('combobox', { name: 'Sắp xếp chứng từ', exact: true }).click();
  await page.locator('.ant-select-dropdown:visible .ant-select-item-option').filter({ hasText: 'Số tiền giảm dần' }).click();
  await expect(page.locator('.ant-table-tbody .ant-table-row').first()).toContainText('BH-10007');
  await expect(page.getByRole('listitem', { name: '1', exact: true })).toHaveClass(/ant-pagination-item-active/);
});

test('nút lưu nháp luôn hiện khi nhập nhiều dòng', async ({ page }) => {
  await page.goto('/documents');
  await page.getByRole('button', { name: 'Tạo chứng từ', exact: true }).click();
  await page.getByLabel('Diễn giải', { exact: true }).fill('Nháp nhiều dòng');
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await expect(page.getByLabel('Diễn giải', { exact: true })).toHaveValue('Nháp nhiều dòng');
  for (let index = 0; index < 8; index++) await page.getByRole('button', { name: 'Thêm dòng hạch toán' }).click();
  await page.locator('.ant-drawer-body').evaluate(element => { element.scrollTop = 0; });
  const save = page.getByRole('button', { name: 'Lưu nháp mẫu', exact: true });
  await expect(save).toBeInViewport();
  expect((await save.boundingBox())?.height).toBe(32);
});

test('lọc không dấu, mở nguồn chứng từ và xuất toàn bộ CSV', async ({ page }) => {
  await page.goto('/documents');
  await page.getByRole('textbox', { name: 'Tìm trong danh sách chứng từ' }).fill('thue van phong');
  await expect(page.getByRole('button', { name: 'PC-10009', exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'BH-10008', exact: true })).toHaveCount(0);
  const download = page.waitForEvent('download');
  await page.getByRole('button', { name: 'Xuất CSV', exact: true }).click();
  expect((await download).suggestedFilename()).toBe('chung-tu-mau-2026-10.csv');
  await page.getByRole('button', { name: 'PC-10009', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'PC-10009', exact: true })).toBeVisible();
  await expect(page.getByText('28.500.000').first()).toBeVisible();
});

test('tiền lớn, thêm/xóa dòng và nháp giữ qua chuyển route, không tác động sổ', async ({ page }) => {
  await page.goto('/documents');
  await page.getByRole('button', { name: 'Tạo chứng từ', exact: true }).click();
  await page.getByLabel('Diễn giải', { exact: true }).fill('Kiểm tra số tiền lớn');
  await expect(page.getByRole('columnheader', { name: 'TK Nợ', exact: true })).toBeVisible();
  await expect(page.getByRole('columnheader', { name: 'TK Có', exact: true })).toBeVisible();
  await expect(page.getByRole('spinbutton', { name: 'Số tiền dòng 1', exact: true })).toHaveCount(1);
  await expect(page.getByRole('combobox', { name: 'Diễn giải dòng 1', exact: true })).toHaveCount(1);
  await expect(page.getByRole('combobox', { name: 'Đối tượng Nợ dòng 1', exact: true })).toBeVisible();
  await expect(page.getByRole('combobox', { name: 'Đối tượng Có dòng 1', exact: true })).toBeVisible();
  await page.getByRole('combobox', { name: 'TK Nợ dòng 1' }).click();
  await page.getByRole('combobox', { name: 'TK Nợ dòng 1' }).fill('111');
  await page.getByRole('combobox', { name: 'TK Nợ dòng 1' }).press('Enter');
  await page.getByRole('combobox', { name: 'TK Có dòng 1' }).click();
  await page.getByRole('combobox', { name: 'TK Có dòng 1' }).fill('411');
  await page.getByRole('combobox', { name: 'TK Có dòng 1' }).press('Enter');
  await page.getByRole('spinbutton', { name: 'Số tiền dòng 1', exact: true }).fill('9007199254740993');
  await page.getByRole('button', { name: 'Thêm dòng hạch toán' }).click();
  await page.getByRole('button', { name: 'Xóa dòng hạch toán 2', exact: true }).click();
  await expect(page.getByText('Cân đối Nợ / Có', { exact: true })).toBeVisible();
  await page.keyboard.press('Escape');
  await page.getByRole('navigation', { name: 'Điều hướng chính' }).getByRole('link', { name: 'Tổng quan', exact: true }).click();
  await page.getByRole('button', { name: 'Tiếp tục nháp', exact: true }).click();
  await expect(page.getByLabel('Diễn giải', { exact: true })).toHaveValue('Kiểm tra số tiền lớn');
  await expect(page.getByRole('spinbutton', { name: 'Số tiền dòng 1', exact: true })).toHaveValue('9.007.199.254.740.993');
  await page.getByRole('spinbutton', { name: 'Số tiền dòng 1', exact: true }).press('ArrowUp');
  await page.getByRole('spinbutton', { name: 'Số tiền dòng 1', exact: true }).hover();
  await page.mouse.wheel(0, -100);
  await expect(page.getByRole('spinbutton', { name: 'Số tiền dòng 1', exact: true })).toHaveValue('9.007.199.254.740.993');
  await page.getByRole('button', { name: 'Lưu nháp mẫu', exact: true }).click();
  await expect(page.getByText('Đã lưu nháp trong phiên mẫu. Nháp chưa tác động đến sổ.')).toBeVisible();
  await expect(page.getByRole('link', { name: /Doanh thu trong kỳ/ })).toContainText('528');
  await page.getByRole('navigation', { name: 'Điều hướng chính' }).getByRole('link', { name: /Kế toán tổng hợp/ }).click();
  await page.getByRole('tab', { name: /Bản nháp/ }).click();
  await expect(page.getByText('Kiểm tra số tiền lớn', { exact: true })).toBeVisible();
  await expect(page.getByText('9.007.199.254.740.993', { exact: true }).first()).toBeVisible();
});

test('danh mục thêm đối tượng và báo trùng mã', async ({ page }) => {
  await page.goto('/catalog');
  await page.getByRole('button', { name: 'Thêm đối tượng', exact: true }).click();
  await page.getByLabel('Mã đối tượng', { exact: true }).fill('KH005');
  await page.getByLabel('Tên đối tượng', { exact: true }).fill('Đối tượng kiểm tra');
  await page.getByRole('button', { name: 'Lưu trong phiên mẫu', exact: true }).click();
  await expect(page.getByText('Đã cập nhật danh mục trong phiên mẫu.')).toBeVisible();
  await page.getByRole('textbox', { name: 'Tìm trong danh mục' }).fill('doi tuong kiem tra');
  await expect(page.getByRole('button', { name: 'KH005', exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Thêm đối tượng', exact: true }).click();
  await page.getByLabel('Mã đối tượng', { exact: true }).fill('kh005');
  await page.getByLabel('Tên đối tượng', { exact: true }).fill('Đối tượng trùng');
  await page.getByRole('button', { name: 'Lưu trong phiên mẫu', exact: true }).click();
  await expect(page.getByText('Mã đối tượng KH005 bị trùng.', { exact: true })).toBeVisible();
});

test('đổi kỳ không giữ chứng từ kỳ cũ, sổ và công nợ truy được nguồn', async ({ page }) => {
  await page.goto('/documents');
  await page.getByRole('listitem', { name: '2', exact: true }).click();
  await page.getByRole('combobox', { name: 'Chọn kỳ kế toán' }).click();
  await page.getByRole('combobox', { name: 'Chọn kỳ kế toán' }).press('ArrowUp');
  await page.getByRole('combobox', { name: 'Chọn kỳ kế toán' }).press('Enter');
  await expect(page.getByRole('button', { name: 'BH-09001', exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'BH-10008', exact: true })).toHaveCount(0);
  await page.getByRole('navigation', { name: 'Điều hướng chính' }).getByRole('link', { name: 'Sổ sách & báo cáo', exact: true }).click();
  await page.getByRole('tab', { name: 'Cân đối phát sinh', exact: true }).click();
  await expect(page.getByText('Tổng toàn bộ tập lọc', { exact: true })).toBeVisible();
  await expect(page.getByText('1.814.000.000', { exact: true })).toHaveCount(2);
  await page.getByRole('tab', { name: 'Sổ cái', exact: true }).click();
  await page.getByRole('combobox', { name: 'Chọn tài khoản sổ cái' }).click();
  await page.getByRole('combobox', { name: 'Chọn tài khoản sổ cái' }).fill('131');
  await page.getByRole('combobox', { name: 'Chọn tài khoản sổ cái' }).press('Enter');
  await expect(page.getByText('Dư đầu Nợ', { exact: false }).first()).toBeVisible();
  await expect(page.getByText('490.000.000', { exact: false }).first()).toBeVisible();
});

test('ô tiền từ chối định dạng mơ hồ và số vượt giới hạn, không lưu giá trị cũ', async ({ page }) => {
  await page.goto('/documents');
  await page.getByRole('button', { name: 'Tạo chứng từ', exact: true }).click();
  const input = page.getByRole('spinbutton', { name: 'Số tiền dòng 1', exact: true });
  for (const value of ['1.5', '1,5', '9223372036854775808']) {
    await input.fill(value);
    await page.getByLabel('Diễn giải', { exact: true }).click();
    await expect(page.getByText('Nhập số tiền nguyên dương hợp lệ', { exact: true })).toBeVisible();
    await expect(input).toHaveValue(value);
  }
  await input.fill('1234567');
  await page.getByLabel('Diễn giải', { exact: true }).click();
  await expect(input).toHaveValue('1.234.567');
  await expect(page.getByText('Nhập số tiền nguyên dương hợp lệ', { exact: true })).toHaveCount(0);
});

test('mobile 390px không tràn ngang và mở được điều hướng, biểu mẫu', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Tổng quan', exact: true })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.getByRole('button', { name: 'Mở điều hướng', exact: true }).click();
  await page.getByRole('navigation', { name: 'Điều hướng chính' }).getByRole('link', { name: /Kế toán tổng hợp/ }).click();
  await expect(page.getByRole('heading', { name: 'Kế toán tổng hợp', exact: true })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.getByRole('button', { name: 'Tạo chứng từ', exact: true }).click();
  await expect(page.getByLabel('Diễn giải', { exact: true })).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.getByRole('button', { name: 'Tiếp tục nháp', exact: true })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});
