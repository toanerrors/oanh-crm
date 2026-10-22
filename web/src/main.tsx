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
  <StrictMode><ConfigProvider locale={viVN} theme={{ token: { colorPrimary: '#087f6d', colorInfo: '#087f6d', colorSuccess: '#087f6d', colorText: '#24332f', colorTextSecondary: '#63756c', colorTextPlaceholder: '#69766e', colorBorder: '#d8e3dc', colorBgLayout: '#f5f8f6', fontFamily: 'Inter, "Segoe UI", Arial, sans-serif', fontSize: 13, fontSizeSM: 12, fontSizeLG: 14, fontSizeHeading1: 20, fontSizeHeading2: 18, fontSizeHeading3: 16, fontSizeHeading4: 14, fontSizeHeading5: 13, lineHeight: 20 / 13, borderRadius: 6, paddingLG: 16, controlHeight: 32, controlHeightSM: 32, controlHeightLG: 32 }, components: { DatePicker: { cellWidth: 36, cellHeight: 24 }, Input: { paddingBlock: 5, paddingBlockSM: 5, paddingBlockLG: 5 }, InputNumber: { paddingBlock: 5, paddingBlockSM: 5, paddingBlockLG: 5 }, Button: { primaryShadow: '0 2px 4px #087f6d1f', fontWeight: 600 }, Table: { headerBg: '#f4f8f5', headerColor: '#586d60', headerSplitColor: 'transparent', cellPaddingBlock: 8, cellPaddingBlockMD: 8, cellPaddingBlockSM: 6, cellPaddingInline: 12, cellPaddingInlineMD: 12, cellPaddingInlineSM: 8, rowHoverBg: '#f3f9f5' }, Form: { itemMarginBottom: 12, verticalLabelPadding: '0 0 4px' }, Tabs: { horizontalItemPadding: '10px 0' }, Tag: { defaultBg: '#f3f5f4' }, Select: { optionSelectedBg: '#eaf5ef', optionHeight: 32 } } }}><App><QueryClientProvider client={queryClient}><AuthGate><RouterProvider router={router} /></AuthGate></QueryClientProvider></App></ConfigProvider></StrictMode>,
);
