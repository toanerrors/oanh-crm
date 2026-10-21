import { Button, Result } from 'antd';
import { createBrowserRouter, Link, useRouteError } from 'react-router-dom';
import { AppShell } from './App';

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
    { index: true, lazy: () => import('../features/dashboard/Dashboard') },
    ...['documents', 'sales', 'purchases', 'cash', 'bank', 'inventory'].map(path => ({ path, lazy: () => import('../features/documents/Documents') })),
    { path: 'settlements', lazy: () => import('../features/documents/Settlements') },
    { path: 'automation', lazy: () => import('../features/workspace/Automation') },
    { path: 'catalog', lazy: () => import('../features/catalog/Catalog') },
    ...['reports', 'receivables'].map(path => ({ path, lazy: () => import('../features/reports/Reports') })),
    ...['settings', 'help'].map(path => ({ path, lazy: () => import('../features/workspace/Workspace') })),
    { path: '*', element: <Result status="404" title="Không tìm thấy trang" subTitle="Đường dẫn không tồn tại trong ứng dụng." extra={<Link to="/"><Button type="primary">Về tổng quan</Button></Link>} /> },
  ],
}]);
