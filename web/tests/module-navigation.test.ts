import assert from 'node:assert/strict';
import test from 'node:test';
import { accountingModule, accountingModules, activeModuleTab, moduleTabs } from '../src/app/module-navigation.ts';

test('mọi tab giữ đường dẫn và ngữ cảnh của phân hệ nguồn', () => {
  for (const module of accountingModules) {
    const tabs = moduleTabs(module.path);
    assert.equal(tabs[0].to, module.path);
    assert.equal(tabs.at(-1)?.to, `${module.path}/reports`);
    for (const tab of tabs) {
      const url = new URL(tab.to, 'https://example.test');
      assert.equal(accountingModule(url.pathname), module);
      assert.equal(activeModuleTab(url.pathname, url.search), tab.key);
      assert.deepEqual(moduleTabs(url.pathname), tabs);
    }
  }
});

test('mua/bán giữ riêng loại công nợ và danh mục đối tượng', () => {
  const purchase = accountingModule('/purchases/settlements');
  const sale = accountingModule('/sales/partners');
  assert.equal(purchase && 'tradeKind' in purchase && purchase.tradeKind, 'purchase');
  assert.equal(sale && 'tradeKind' in sale && sale.tradeKind, 'sale');
  assert.equal(moduleTabs('/purchases/reports')[2].label, 'Nhà cung cấp');
  assert.equal(moduleTabs('/sales/items')[2].label, 'Khách hàng');
  assert.deepEqual(moduleTabs('/inventory').map(tab => tab.key), ['documents', 'reports']);
});

test('báo cáo con và kết chuyển chọn đúng tab, không nhận nhầm đường dẫn ngoài phân hệ', () => {
  assert.equal(activeModuleTab('/purchases/reports', '?tab=financial'), 'reports');
  assert.equal(activeModuleTab('/documents', '?source=closing&q=test'), 'closing');
  assert.equal(activeModuleTab('/documents/reports', '?source=closing'), 'reports');
  assert.equal(activeModuleTab('/sales/', ''), 'documents');
  for (const path of ['/reports', '/catalog', '/sales-other', '/purchases-old/reports']) {
    assert.equal(accountingModule(path), undefined);
    assert.deepEqual(moduleTabs(path), []);
    assert.equal(activeModuleTab(path, ''), '');
  }
});
