import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { App, ConfigProvider } from 'antd';
import viVN from 'antd/locale/vi_VN';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { RouterProvider } from 'react-router-dom';
import { router } from './app/router';
import { AuthGate } from './app/AuthGate';
import './app/global.css';

const queryClient = new QueryClient({ defaultOptions: { queries: { gcTime: 300_000, refetchOnWindowFocus: true, refetchOnReconnect: true }, mutations: { retry: false } } });

createRoot(document.getElementById('root')!).render(
  <StrictMode><ConfigProvider locale={viVN} theme={{ token: { colorPrimary: '#087f6d', colorInfo: '#087f6d', colorSuccess: '#087f6d', colorText: '#24332f', colorTextSecondary: '#7a8580', colorBorder: '#e5eae7', colorBgLayout: '#f7f9f8', fontFamily: 'Inter, "Segoe UI", Arial, sans-serif', fontSize: 13, borderRadius: 7, controlHeight: 36 }, components: { Button: { primaryShadow: '0 2px 3px #087f6d14', fontWeight: 500 }, Table: { headerBg: '#f8faf9', headerColor: '#78817e', headerSplitColor: 'transparent', cellPaddingBlock: 15 }, Tag: { defaultBg: '#f3f5f4' }, Select: { optionSelectedBg: '#eaf5ef' } } }}><App><QueryClientProvider client={queryClient}><AuthGate><RouterProvider router={router} /></AuthGate></QueryClientProvider></App></ConfigProvider></StrictMode>,
);
