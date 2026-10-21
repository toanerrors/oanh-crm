import { expect, test } from '@playwright/test';

test('tổng quan, điều hướng SPA và các màn hình chính hiển thị đúng', async ({ page }) => {
  const errors: string[] = [];
  page.on('pageerror', error => errors.push(error.message));
  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Tổng quan', exact: true })).toBeVisible();
  await expect(page.getByRole('link', { name: /Doanh thu trong kỳ/ })).toContainText('528');
  for (const [label, heading] of [['Bán hàng', 'Bán hàng'], ['Mua hàng', 'Mua hàng'], ['Tiền mặt & ngân hàng', 'Tiền mặt & ngân hàng'], ['Hàng tồn kho', 'Hàng tồn kho'], ['Công nợ', 'Công nợ'], ['Sổ sách & báo cáo', 'Sổ sách & báo cáo'], ['Danh mục', 'Danh mục']] as const) {
    await page.getByRole('navigation', { name: 'Điều hướng chính' }).getByRole('link', { name: label, exact: true }).click();
    await expect(page.getByRole('heading', { name: heading, exact: true })).toBeVisible();
  }
  expect(errors).toEqual([]);
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
  await page.getByRole('combobox', { name: 'Tài khoản dòng 1' }).click();
  await page.getByRole('combobox', { name: 'Tài khoản dòng 1' }).fill('111');
  await page.getByRole('combobox', { name: 'Tài khoản dòng 1' }).press('Enter');
  await page.getByRole('combobox', { name: 'Tài khoản dòng 2' }).click();
  await page.getByRole('combobox', { name: 'Tài khoản dòng 2' }).fill('411');
  await page.getByRole('combobox', { name: 'Tài khoản dòng 2' }).press('Enter');
  await page.getByRole('spinbutton', { name: 'Số tiền Nợ dòng 1', exact: true }).fill('9007199254740993');
  await page.getByRole('spinbutton', { name: 'Số tiền Có dòng 2', exact: true }).fill('9007199254740993');
  await page.getByRole('button', { name: 'Thêm dòng hạch toán' }).click();
  await page.getByRole('button', { name: 'Xóa dòng 3', exact: true }).click();
  await expect(page.getByText('Cân đối Nợ / Có', { exact: true })).toBeVisible();
  await page.keyboard.press('Escape');
  await page.getByRole('navigation', { name: 'Điều hướng chính' }).getByRole('link', { name: 'Tổng quan', exact: true }).click();
  await page.getByRole('button', { name: 'Tiếp tục nháp', exact: true }).click();
  await expect(page.getByLabel('Diễn giải', { exact: true })).toHaveValue('Kiểm tra số tiền lớn');
  await expect(page.getByRole('spinbutton', { name: 'Số tiền Nợ dòng 1', exact: true })).toHaveValue('9.007.199.254.740.993');
  await page.getByRole('spinbutton', { name: 'Số tiền Nợ dòng 1', exact: true }).press('ArrowUp');
  await page.getByRole('spinbutton', { name: 'Số tiền Nợ dòng 1', exact: true }).hover();
  await page.mouse.wheel(0, -100);
  await expect(page.getByRole('spinbutton', { name: 'Số tiền Nợ dòng 1', exact: true })).toHaveValue('9.007.199.254.740.993');
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
  const input = page.getByRole('spinbutton', { name: 'Số tiền Nợ dòng 1', exact: true });
  for (const value of ['1.5', '1,5', '9223372036854775808']) {
    await input.fill(value);
    await page.getByLabel('Diễn giải', { exact: true }).click();
    await expect(page.getByText('Nhập số nguyên không âm hợp lệ', { exact: true })).toBeVisible();
    await expect(input).toHaveValue(value);
  }
  await input.fill('1234567');
  await page.getByLabel('Diễn giải', { exact: true }).click();
  await expect(input).toHaveValue('1.234.567');
  await expect(page.getByText('Nhập số nguyên không âm hợp lệ', { exact: true })).toHaveCount(0);
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
});
