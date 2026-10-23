import { useEffect, useState } from 'react';
import { Alert, App, Button, Checkbox, Drawer, Form, Input, InputNumber, Modal, Select, Space, Table, Tabs, Tag } from 'antd';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useSearchParams } from 'react-router-dom';
import { useWorkspace } from '../../app/WorkspaceProvider';
import { api, apiScope, command, downloadApi } from '../../shared/api/client';
import { workspaceKey } from '../../shared/data/workspace';
import { businessToday, formatDate, formatMoney, normalizeSearch, parseMoneyInput } from '../../shared/accounting/money';
import { downloadCsv } from '../../shared/accounting/csv';
import { PageHeading, PageToolbar, DataBoundary } from '../../shared/ui/Page';
import type { ServerReportRow } from '../../shared/data/reports';
import { financialPreview, formatFinancialAmount } from './financial';
import type { FinancialInput, FinancialReport, FinancialRow, FinancialTemplate } from './financial';
import { ToolbarButton } from '../../shared/ui/ToolbarButton';
import { ExportMenu } from '../../shared/ui/ExportMenu';
import { DateInput } from '../../shared/ui/DateInput';
import styles from './FinancialReports.module.css';
import appStyles from '../../app/App.module.css';

function ReportAmount({ value, onChange, label, decimal = false }: { value?: string; onChange?: (value: string) => void; label: string; decimal?: boolean }) {
  const parse = (text: string) => {
    const signed = text.replace(/^−/, '-');
    const unsigned = signed.replace(/^-/, '');
    const result = decimal && unsigned.includes(',') ? unsigned.replace(/\./g, '').replace(',', '.') : parseMoneyInput(unsigned) ?? '';
    return signed.startsWith('-') && result ? `-${result}` : result;
  };
  return <InputNumber<string> aria-label={label} stringMode value={value || undefined} controls={false} keyboard={false} changeOnWheel={false} changeOnBlur={false} inputMode="text" placeholder="Chưa nhập" style={{ width: '100%' }}
    formatter={(text, info) => info.userTyping ? info.input : value && /^-?(0|[1-9]\d*)(\.\d{1,2})?$/.test(value) ? formatFinancialAmount(value) : String(text ?? '')}
    parser={text => parse(text ?? '')} onInput={text => onChange?.(parse(text))} />;
}

export function FinancialReports() {
  const [, setParams] = useSearchParams();
  const { data, period } = useWorkspace();
  const [year, setYear] = useState(period.slice(0, 4));
  const [search, setSearch] = useState('');
  const [setup, setSetup] = useState(false);
  const [report, setReport] = useState<FinancialReport | null>(null);
  const [form] = Form.useForm<{ year: string; b02: boolean; cashflow: string }>();
  const template = useQuery({ queryKey: [...workspaceKey(), 'financial-template'], queryFn: ({ signal }) => api<FinancialTemplate>('/financial-reports/template', { signal }), retry: false });
  const list = useQuery({ queryKey: [...workspaceKey(), 'financial-list', year], queryFn: ({ signal }) => api<FinancialReport[]>(`/financial-reports?year=${year}`, { signal }), enabled: /^\d{4}$/.test(year), retry: false });
  if (!data) return <DataBoundary><></></DataBoundary>;
  const create = () => { const chosen = form.getFieldsValue(); const selectedYear = chosen.year;
    const input: FinancialInput = { id: crypto.randomUUID(), version: 0, template: template.data!.version, from: `${selectedYear}-01-01`, to: `${selectedYear}-12-31`, previousFrom: `${BigInt(selectedYear) - 1n}-01-01`, previousTo: `${BigInt(selectedYear) - 1n}-12-31`, preparedBy: apiScope().name, chiefAccountant: '', director: '', preparedDate: businessToday(), address: '', sheets: ['B01-DN', ...(chosen.b02 ? ['B02-DN'] : []), ...(chosen.cashflow !== 'none' ? [chosen.cashflow] : [])], values: {}, notes: '', sourceRevision: data.revision };
    setSetup(false); setReport({ input, computed: {}, issues: [], updatedAt: '', companyName: apiScope().companyName });
  };
  const pageActions = <ToolbarButton type="primary" disabled={!template.data || apiScope().permissions === 'viewer'} onClick={() => { form.setFieldsValue({ year: year < '2026' ? '2026' : year, b02: true, cashflow: 'B03-DN' }); setSetup(true); }} label="Lập báo cáo tài chính" icon="plus" />;

  return <DataBoundary>
    <PageHeading title="Sổ sách & báo cáo" description="Lập và đối chiếu báo cáo tài chính từ số liệu kế toán." />
    <section className={appStyles.panel}>
      <div className={styles.tabs}><Tabs activeKey="financial" onChange={value => setParams({ tab: value })} items={[{ key: 'journal', label: 'Nhật ký chung' }, { key: 'ledger', label: 'Sổ cái' }, { key: 'trial', label: 'Cân đối phát sinh' }, { key: 'financial', label: 'Báo cáo tài chính' }]} /></div>
      <PageToolbar label="Công cụ BCTC" actions={<>{pageActions}<ToolbarButton onClick={() => void list.refetch()} label="Làm mới" icon="refresh" /></>}><Input className={appStyles.filterSearch} aria-label="Tìm BCTC" placeholder="Tìm người lập, kỳ báo cáo…" value={search} onChange={event => setSearch(event.target.value)} allowClear /><Input aria-label="Năm tra cứu BCTC" type="number" min="2026" max="9999" value={year} onChange={event => { setYear(event.target.value); }} style={{ width: 100 }} /></PageToolbar>
      {(list.error || template.error) && <Alert type="error" title={(list.error || template.error)?.message} />}
      <Table<FinancialReport> rowKey={item => item.input.id} loading={list.isPending} dataSource={(list.data ?? []).filter(item => normalizeSearch(`${item.input.from} ${item.input.preparedBy}`).includes(normalizeSearch(search)))} scroll={{ x: 850 }} pagination={{ pageSize: 10, hideOnSinglePage: true, showSizeChanger: false }} locale={{ emptyText: 'Chưa có BCTC trong năm. Chọn “Lập báo cáo tài chính” để bắt đầu.' }} columns={[
        { title: 'KỲ BÁO CÁO', render: (_, item) => `Năm ${item.input.from.slice(0, 4)}`, width: 130 },
        { title: 'TÊN BÁO CÁO', render: () => 'Báo cáo tài chính' },
        { title: 'NGƯỜI LẬP', render: (_, item) => item.input.preparedBy || 'Chưa nhập' },
        { title: 'NGÀY LẬP', render: (_, item) => formatDate(item.input.preparedDate), width: 120 },
        { title: 'TRẠNG THÁI', render: (_, item) => <Tag color={item.issues.length ? 'gold' : 'blue'}>{item.issues.length ? 'Nháp · cần kiểm tra' : 'Nháp · đã kiểm tra số học'}</Tag>, width: 190 },
        { title: 'CHỨC NĂNG', render: (_, item) => <Button type="link" onClick={() => setReport(item)}>Xem</Button>, width: 100 },
      ]} />
    </section>
    <Alert className={styles.notice} type="info" title="Lập bản nháp BCTC" description="Nhập chỉ tiêu chi tiết từ sổ đã đối chiếu; tổng cộng được tính tự động. Số 0 cần xác nhận rõ. Chưa tự phân loại tài khoản, lập XML hoặc nộp cơ quan thuế." />
    <Modal title="Chọn tham số báo cáo: Báo cáo tài chính" open={setup} onCancel={() => setSetup(false)} onOk={() => void form.validateFields().then(create)} okText="Đồng ý" cancelText="Hủy" width={680}>
      <Form form={form} layout="vertical"><div className={styles.setup}><Form.Item label="Kỳ báo cáo"><Input value="Năm" readOnly /></Form.Item><Form.Item name="year" label="Năm" rules={[{ required: true, pattern: /^(202[6-9]|20[3-9]\d|2[1-9]\d{2}|[3-9]\d{3})$/, message: 'Nhập năm từ 2026 đến 9999.' }]}><Input aria-label="Năm lập BCTC" /></Form.Item></div>
        <p>Doanh nghiệp đáp ứng giả định hoạt động liên tục · TT99/2025/TT-BTC</p>
        <Table pagination={false} size="small" rowKey="code" dataSource={[{ code: 'B01-DN', name: 'Báo cáo tình hình tài chính' }, { code: 'B02-DN', name: 'Báo cáo kết quả hoạt động kinh doanh' }]} columns={[{ title: 'CHỌN', width: 70, render: (_, row) => row.code === 'B01-DN' ? <Checkbox checked disabled aria-label="Chọn B01-DN" /> : <Form.Item name="b02" valuePropName="checked" noStyle><Checkbox aria-label="Chọn B02-DN" /></Form.Item> }, { title: 'MÃ BÁO CÁO', dataIndex: 'code', width: 130 }, { title: 'TÊN BÁO CÁO', dataIndex: 'name' }]} />
        <Form.Item name="cashflow" label="Báo cáo lưu chuyển tiền tệ" style={{ marginTop: 16 }}><Select options={[{ value: 'B03-DN', label: 'B03-DN · Phương pháp trực tiếp' }, { value: 'B03-DN-GT', label: 'B03-DN · Phương pháp gián tiếp' }, { value: 'none', label: 'Chưa lập lưu chuyển tiền tệ' }]} /></Form.Item>
      </Form>
    </Modal>
    {report && template.data && <FinancialEditor key={report.input.id} initial={report} template={template.data} onClose={() => setReport(null)} onSaved={saved => { setYear(saved.input.from.slice(0, 4)); }} />}
  </DataBoundary>;
}

function FinancialEditor({ initial, template, onClose, onSaved }: { initial: FinancialReport; template: FinancialTemplate; onClose: () => void; onSaved: (report: FinancialReport) => void }) {
  const [form] = Form.useForm<FinancialInput>();
  const input = Form.useWatch([], form) as FinancialInput | undefined;
  const [saved, setSaved] = useState(initial);
  const [dirty, setDirty] = useState(false);
  const [active, setActive] = useState(initial.input.sheets[0]);
  const [busy, setBusy] = useState(false);
  const [checked, setChecked] = useState<FinancialReport | null>(null);
  const [sourceOpen, setSourceOpen] = useState(false);
  const [sourcePeriod, setSourcePeriod] = useState('current');
  const [showFormulas, setShowFormulas] = useState(false);
  const { message, modal } = App.useApp();
  const client = useQueryClient();
  const { data, refresh } = useWorkspace();
  const editable = apiScope().permissions !== 'viewer';
  const current = { ...saved.input, ...input };
  const from = sourcePeriod === 'current' ? current.from : current.previousFrom;
  const to = sourcePeriod === 'current' ? current.to : current.previousTo;
  const source = useQuery({ queryKey: [...workspaceKey(), 'financial-source', from, to, data?.revision], queryFn: ({ signal }) => api<{ rows: ServerReportRow[]; revision: number }>(`/financial-reports/source?from=${from}&to=${to}`, { signal }), enabled: sourceOpen, retry: false });
  useEffect(() => { const warn = (event: BeforeUnloadEvent) => { if (dirty) event.preventDefault(); }; window.addEventListener('beforeunload', warn); return () => window.removeEventListener('beforeunload', warn); }, [dirty]);
  const close = () => { if (!dirty) { onClose(); return; } modal.confirm({ title: 'BCTC chưa lưu', content: 'Đóng biểu mẫu sẽ bỏ các thay đổi đang nhập.', okText: 'Bỏ thay đổi', cancelText: 'Tiếp tục nhập', onOk: onClose }); };
  const run = async (save: boolean) => {
    setBusy(true);
    try {
      await form.validateFields();
      const payload = { ...saved.input, ...form.getFieldsValue(true), version: saved.input.version };
      const result = save ? await command<FinancialReport>('/financial-reports', payload) : await api<FinancialReport>('/financial-reports/preview', { method: 'POST', body: JSON.stringify(payload) });
      setChecked(result);
      if (save) { setSaved(result); form.setFieldsValue(result.input); setDirty(false); onSaved(result); void client.invalidateQueries({ queryKey: [...workspaceKey(), 'financial-list'] }); void refresh(); void message.success('Đã lưu bản nháp BCTC.'); }
      else void message.info(result.issues.length ? `Có ${result.issues.length} mục cần kiểm tra.` : 'Các phép kiểm tra số học đã khớp.');
    } catch (error) { if (error instanceof Error) void message.error(error.message); }
    finally { setBusy(false); }
  };
  const sheet = template.sheets.find(item => item.id === active);
  const computed = sheet ? financialPreview(current, sheet) : {};
  const amount = (row: FinancialRow, field: 'current' | 'previous') => row.formula.length ? (computed[row.code]?.[field] ? formatMoney(computed[row.code][field]) : '—') : <Form.Item name={['values', active, row.code, field]} rules={[{ validator: (_, value: unknown) => value === undefined || value === '' || typeof value === 'string' && (active === 'B02-DN' && ['70', '71'].includes(row.code) ? /^-?(0|[1-9]\d{0,29})(\.\d{1,2})?$/.test(value) : /^-?(0|[1-9]\d{0,29})$/.test(value)) && value !== '-0' ? Promise.resolve() : Promise.reject(new Error('Nhập số chuẩn tối đa 30 chữ số nguyên; lãi/cổ phiếu tối đa 2 chữ số lẻ.')) }]}><ReportAmount decimal={active === 'B02-DN' && ['70', '71'].includes(row.code)} label={`${active} ${row.code} ${field === 'current' ? 'kỳ này' : 'kỳ trước'}`} /></Form.Item>;
  const exportCsv = () => {
    if (dirty || !saved.input.version) { void message.info('Lưu nháp trước khi xuất.'); return; }
    const rows: string[][] = [['BẢN NHÁP BÁO CÁO TÀI CHÍNH', saved.companyName, template.regime], ['Kỳ', saved.input.from, saved.input.to, 'Kỳ so sánh', saved.input.previousFrom, saved.input.previousTo], ['Phiên bản báo cáo', String(saved.input.version), 'Phiên bản nguồn tham chiếu', String(saved.input.sourceRevision)]];
    for (const item of template.sheets.filter(item => saved.input.sheets.includes(item.id))) { rows.push([item.id, item.name], ['Chỉ tiêu', 'Mã số', 'Thuyết minh', 'Kỳ này (VND)', item.id === 'B01-DN' ? 'Đầu kỳ (VND)' : 'Kỳ trước (VND)']); for (const row of item.rows) { const v = saved.computed[item.id][row.code]; rows.push([row.name, row.code, v.note, v.current === '' ? 'Chưa nhập' : `${v.current} VND`, v.previous === '' ? 'Chưa nhập' : `${v.previous} VND`]); } }
    rows.push(['Thuyết minh bổ sung', saved.input.notes], ['Người lập', saved.input.preparedBy, 'Kế toán trưởng', saved.input.chiefAccountant, 'Đại diện pháp luật', saved.input.director], ['Ngày lập', saved.input.preparedDate], ...saved.issues.map(issue => ['Cần kiểm tra', issue.sheet, issue.code, issue.message]));
    downloadCsv(`bctc-nhap-${saved.input.from.slice(0, 4)}-v${saved.input.version}.csv`, rows);
  };
  return <Modal open title={`Báo cáo tài chính · Năm ${current.from.slice(0, 4)}`} onCancel={close} width="calc(100vw - 32px)" style={{ top: 16, maxWidth: 1800, paddingBottom: 0 }} className={styles.editor} maskClosable={false} footer={<div className={styles.actions}><Space wrap><Button onClick={close}>Đóng</Button><ExportMenu text disabled={dirty || !saved.input.version || busy} onExport={format => { if (format === 'csv') return exportCsv(); if (format === 'print') { window.open(`/api/v1/financial-reports/print?id=${saved.input.id}&version=${saved.input.version}`, '_blank', 'noopener,noreferrer'); return; } return downloadApi(`/financial-reports/export?id=${saved.input.id}&version=${saved.input.version}&format=xlsx`, `bctc-nhap-${saved.input.from.slice(0, 4)}-v${saved.input.version}.xlsx`); }} /></Space><Space wrap><Button onClick={() => void run(false)} loading={busy}>Kiểm tra báo cáo</Button>{editable && <Button type="primary" loading={busy} onClick={() => void run(true)}>Cất bản nháp</Button>}</Space></div>}>
    <Form form={form} initialValues={initial.input} layout="vertical" disabled={!editable || busy} onValuesChange={() => { setDirty(true); setChecked(null); }}>
      <div className={styles.header}><div><strong>{saved.companyName}</strong><p>{formatDate(current.from)} – {formatDate(current.to)} · {template.regime} · VND</p><p>So sánh {formatDate(current.previousFrom)} – {formatDate(current.previousTo)} · B01: số đầu kỳ</p></div><Space wrap><Tag color="gold">Bản nháp {dirty ? '· chưa lưu' : `· phiên bản ${saved.input.version}`}</Tag><Button disabled={false} onClick={() => setShowFormulas(value => !value)}>{showFormulas ? 'Ẩn công thức' : 'Xem công thức'}</Button><Button disabled={false} onClick={() => setSourceOpen(true)}>Đối chiếu sổ</Button></Space></div>
      <div className={styles.body}>
        <Alert className={styles.notice} type="info" showIcon title="Nhập chỉ tiêu đã đối chiếu; dòng tổng được tính tự động" description={<details><summary>Hướng dẫn nhập số liệu</summary>Ô trống là chưa có dữ liệu. Khoản giảm trừ (*) và dòng tiền chi nhập số âm. Số liệu nhập tay chưa được tự ánh xạ từ tài khoản. Kiểm tra số học không thay cho duyệt BCTC.</details>} />
        {current.sourceRevision < (data?.revision ?? 0) && <Alert className={styles.notice} type="warning" title={`Sổ hiện tại ở phiên bản ${data?.revision}; bản nháp tham chiếu phiên bản ${current.sourceRevision}. Đối chiếu lại nếu sổ đã thay đổi.`} />}
        {checked && <Alert className={styles.notice} type={checked.issues.length ? 'warning' : 'success'} title={checked.issues.length ? `${checked.issues.length} mục cần kiểm tra` : 'Các phép kiểm tra số học đã khớp'} description={checked.issues.length ? <details><summary>Xem chi tiết</summary><ul>{checked.issues.map((issue, index) => <li key={index}><button type="button" className={appStyles.tableLink} onClick={() => { if (current.sheets.includes(issue.sheet)) setActive(issue.sheet); }}>{issue.sheet} {issue.code}</button> {issue.message}</li>)}</ul></details> : 'Bản nháp cần được kế toán rà soát và phê duyệt trước khi sử dụng.'} />}
        <Tabs activeKey={active} onChange={setActive} items={[...current.sheets.map(id => ({ key: id, label: id })), { key: 'notes', label: 'Thuyết minh' }]} />
        {sheet ? <><div className={styles.caption}><h2>{sheet.name.toLocaleUpperCase('vi')}</h2><span>{sheet.id === 'B01-DN' ? `Tại ngày ${formatDate(current.to)}` : `Kỳ kế toán ${formatDate(current.from)} – ${formatDate(current.to)}`}</span></div>
          <Table<FinancialRow> rowKey="code" dataSource={sheet.rows} pagination={false} rowClassName={row => row.formula.length ? styles.summary : ''} scroll={{ x: 1020 }} columns={[
            { title: 'CHỈ TIÊU', dataIndex: 'name', width: 470, render: (name: string, row) => <div style={{ paddingLeft: row.level * 12 }}>{name}{showFormulas && row.formula.length > 0 && <span className={styles.formula}>{row.code} = {row.formula.map((term, index) => `${index && !term.startsWith('-') ? '+' : ''}${term}`).join(' ')}</span>}</div> },
            { title: 'MÃ SỐ', dataIndex: 'code', width: 85 },
            { title: 'THUYẾT MINH', width: 130, render: (_, row) => row.formula.length ? '' : <Form.Item name={['values', active, row.code, 'note']}><Input aria-label={`${active} ${row.code} thuyết minh`} maxLength={100} /></Form.Item> },
            { title: sheet.id === 'B01-DN' ? 'SỐ CUỐI KỲ' : 'NĂM NAY', width: 200, align: 'right', render: (_, row) => amount(row, 'current') },
            { title: sheet.id === 'B01-DN' ? 'SỐ ĐẦU KỲ' : 'NĂM TRƯỚC', width: 200, align: 'right', render: (_, row) => amount(row, 'previous') },
          ]} />
        </> : <Form.Item name="notes" label="Thuyết minh và giải trình bổ sung"><Input.TextArea rows={18} maxLength={60000} showCount placeholder="Nhập thuyết minh, chính sách kế toán, giải trình và tham chiếu các bảng chi tiết đã đối chiếu…" /></Form.Item>}
        <Form.Item name="address" label="Địa chỉ đơn vị báo cáo"><Input maxLength={160} /></Form.Item>
        <div className={styles.signatures}><Form.Item name="preparedBy" label="Người lập biểu"><Input maxLength={160} /></Form.Item><Form.Item name="chiefAccountant" label="Kế toán trưởng"><Input maxLength={160} /></Form.Item><Form.Item name="director" label="Người đại diện theo pháp luật"><Input maxLength={160} /></Form.Item><Form.Item name="preparedDate" label="Ngày lập" rules={[{ required: true, message: 'Chọn ngày lập.' }]}><DateInput /></Form.Item></div>
      </div>
    </Form>
    <Drawer title="Đối chiếu số liệu sổ đã ghi sổ" open={sourceOpen} onClose={() => setSourceOpen(false)} width="min(1200px, 96vw)"><div className={styles.source}><Select aria-label="Kỳ đối chiếu" value={sourcePeriod} onChange={setSourcePeriod} options={[{ value: 'current', label: `Năm ${current.from.slice(0, 4)}` }, { value: 'previous', label: `Năm ${current.previousFrom.slice(0, 4)}` }]} /><p>{formatDate(from)} – {formatDate(to)} · Phiên bản {source.data?.revision ?? '…'} · VND · Giữ số dư hai bên theo đối tượng</p><Alert type="info" title="Phát sinh bao gồm bút toán kết chuyển. Khi lập B02, cần đối chiếu và loại chuyển số nội bộ theo đúng chỉ tiêu." />{source.error && <Alert type="error" title={source.error.message} />}<Table<ServerReportRow> rowKey="key" dataSource={source.data?.rows ?? []} loading={source.isPending} pagination={{ pageSize: 15, showSizeChanger: false }} scroll={{ x: 1000 }} columns={[{ title: 'TK', dataIndex: 'code', width: 80 }, { title: 'Tên tài khoản', dataIndex: 'name', width: 240 }, ...(['openingDebit', 'openingCredit', 'debit', 'credit', 'closingDebit', 'closingCredit'] as const).map((key, index) => ({ title: ['Dư đầu Nợ', 'Dư đầu Có', 'PS Nợ', 'PS Có', 'Dư cuối Nợ', 'Dư cuối Có'][index], dataIndex: key, align: 'right' as const, width: 145, render: (value: string) => formatMoney(value) }))]} /></div></Drawer>
  </Modal>;
}
