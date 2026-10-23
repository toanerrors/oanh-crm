import { Button, Result } from 'antd';
import { createBrowserRouter, Link, useRouteError } from 'react-router-dom';
import { AppShell } from './App';
import { ModuleLayout } from './ModuleLayout';
import { accountingModules } from './module-navigation';
import { screens } from './route-preload';

function RouteError() {
  const error = useRouteError();
  const chunkError = error instanceof Error && /dynamically imported|module script|chunk/i.test(error.message);
  return <Result status="warning" title={chunkError ? 'Không tải được màn hình' : 'Không thể hiển thị nội dung'} subTitle={chunkError ? 'Kiểm tra kết nối rồi thử lại. Nháp được giữ trong phiên hiện tại; hãy xuất nháp trước khi tải lại ứng dụng.' : 'Dữ liệu trong phiên được giữ nguyên. Hãy quay về tổng quan.'} extra={<Link to="/"><Button type="primary">Về tổng quan</Button></Link>} />;
}

export const router = createBrowserRouter([{
  Component: AppShell,
  HydrateFallback: () => <div role="status" style={{ padding: 32 }}>Đang tải ứng dụng…</div>,
  ErrorBoundary: RouteError,
  children: [
    { index: true, lazy: screens.dashboard },
    ...accountingModules.map(module => ({ path: module.path.slice(1), Component: ModuleLayout, children: [
      { index: true, lazy: screens.documents },
      { path: 'reports', lazy: screens.reports },
      ...('tradeKind' in module ? [
        { path: 'settlements', lazy: screens.settlements },
        { path: 'partners', lazy: screens.catalog },
        { path: 'items', lazy: screens.items },
      ] : []),
    ] })),
    { path: 'settlements', lazy: screens.settlements },
    { path: 'vouchers/new', lazy: screens.voucher },
    { path: 'vouchers/:id', lazy: screens.voucher },
    { path: 'automation', lazy: screens.automation },
    { path: 'imports', lazy: screens.imports },
    { path: 'catalog', lazy: screens.catalog },
    ...['reports', 'receivables'].map(path => ({ path, lazy: screens.reports })),
    ...['settings', 'help'].map(path => ({ path, lazy: screens.workspace })),
    { path: '*', element: <Result status="404" title="Không tìm thấy trang" subTitle="Đường dẫn không tồn tại trong ứng dụng." extra={<Link to="/"><Button type="primary">Về tổng quan</Button></Link>} /> },
  ],
}]);
