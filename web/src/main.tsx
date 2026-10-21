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
  <StrictMode><ConfigProvider locale={viVN} theme={{ token: { colorPrimary: '#087f6d', colorInfo: '#087f6d', colorSuccess: '#087f6d', colorText: '#24332f', colorTextSecondary: '#63756c', colorTextPlaceholder: '#69766e', colorBorder: '#d8e3dc', colorBgLayout: '#f5f8f6', fontFamily: 'Inter, "Segoe UI", Arial, sans-serif', fontSize: 14, borderRadius: 8, controlHeight: 40, controlHeightSM: 40, controlHeightLG: 40 }, components: { Button: { primaryShadow: '0 2px 4px #087f6d1f', fontWeight: 600 }, Table: { headerBg: '#f4f8f5', headerColor: '#586d60', headerSplitColor: 'transparent', cellPaddingBlock: 14, rowHoverBg: '#f3f9f5' }, Tag: { defaultBg: '#f3f5f4' }, Select: { optionSelectedBg: '#eaf5ef', optionHeight: 40 } } }}><App><QueryClientProvider client={queryClient}><AuthGate><RouterProvider router={router} /></AuthGate></QueryClientProvider></App></ConfigProvider></StrictMode>,
);
