import { useEffect, useRef, useState } from 'react';
import type { ReactNode } from 'react';
import { Alert, App, Button, Checkbox, Input, InputNumber, Modal, Result, Select, Skeleton, Steps, Switch, Table, Tag, Upload } from 'antd';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useLocation } from 'react-router-dom';
import { api, apiScope, command, downloadApi, isDemo } from '../../shared/api/client';
import { workspaceKey } from '../../shared/data/workspace';
import { formatMoney } from '../../shared/accounting/money';
import { Icon } from '../../shared/ui/Icon';
import { DateInput } from '../../shared/ui/DateInput';
import { PageHeading } from '../../shared/ui/Page';
import { ToolbarButton } from '../../shared/ui/ToolbarButton';
import { searchOptions } from '../../shared/ui/search';
import { excelKindForPath, matchExcelHeaders, unusedExcelColumns } from './model';
import type { ExcelBatch, ExcelConfiguration, ExcelField, ExcelPreview, ExcelResult, ExcelSchema, ExcelSource } from './model';
import styles from './ExcelImport.module.css';
import appStyles from '../../app/App.module.css';

const epochHeaders = () => ({ 'X-Dataset-Epoch': apiScope().epoch });
const schemaOptions = () => ({ queryKey: [...workspaceKey(), 'excel-schemas'], queryFn: ({ signal }: { signal: AbortSignal }) => api<ExcelSchema[]>('/excel-imports/schemas', { signal }), staleTime: 60_000, retry: false });

export function ImportExcelButton({ kind, children }: { kind?: string; children?: (open: (kind?: string) => void) => ReactNode }) {
  const [open, setOpen] = useState(false);
  const [selectedKind, setSelectedKind] = useState(kind);
  const location = useLocation();
  const [busy, setBusy] = useState(false);
  if (isDemo || apiScope().permissions === 'viewer') return null;
  const close = () => { if (!busy) setOpen(false); };
  const openImport = (nextKind = kind) => { setSelectedKind(nextKind); setOpen(true); };
  return <>{children ? children(openImport) : <ToolbarButton label="Nhập từ Excel" icon="upload" onClick={() => openImport()} />}<Modal title="Nhập từ Excel" open={open} onCancel={close} maskClosable={false} closable={!busy} width={1120} footer={null} destroyOnHidden><ExcelWizard key={selectedKind ?? excelKindForPath(location.pathname)} initialKind={selectedKind ?? excelKindForPath(location.pathname)} onBusy={setBusy} onDone={close} /></Modal></>;
}

interface WizardProps { initialKind?: string; initialBatch?: string; onBusy?: (busy: boolean) => void; onDone?: () => void }
export function ExcelWizard({ initialKind = 'journal-pairs', initialBatch = '', onBusy, onDone }: WizardProps) {
  const { message } = App.useApp();
  const client = useQueryClient();
  const schemas = useQuery(schemaOptions());
  const loadedBatch = useRef('');
  const [kind, setKind] = useState(initialKind);
  const [batchId, setBatchId] = useState(initialBatch);
  const [sheet, setSheet] = useState('');
  const [headerRow, setHeaderRow] = useState(1);
  const [step, setStep] = useState(0);
  const [mapState, setMapState] = useState<{ key: string; value: Record<string, number> }>({ key: '', value: {} });
  const [allFields, setAllFields] = useState(false);
  const [dateFormat, setDateFormat] = useState('dmy');
  const [numberFormat, setNumberFormat] = useState('plain');
  const [mode, setMode] = useState('add');
  const [cutoff, setCutoff] = useState('');
  const [openingSource, setOpeningSource] = useState('');
  const [ignoreUnmapped, setIgnoreUnmapped] = useState(false);
  const [preview, setPreview] = useState<ExcelPreview | null>(null);
  const [result, setResult] = useState<ExcelResult | null>(null);
  const [filter, setFilter] = useState('all');
  const [error, setError] = useState('');
  const schema = schemas.data?.find(item => item.key === kind);
  const source = useQuery({ queryKey: [...workspaceKey(), 'excel-source', batchId, sheet, headerRow], queryFn: ({ signal }) => api<ExcelSource>(`/excel-imports/${encodeURIComponent(batchId)}?${new URLSearchParams({ sheet, headerRow: String(headerRow) })}`, { signal, headers: epochHeaders() }), enabled: Boolean(batchId), retry: false });
  const mapKey = `${batchId}:${source.data?.sheet ?? ''}:${headerRow}:${kind}`;
  const mapping = mapState.key === mapKey ? mapState.value : matchExcelHeaders(source.data?.headers ?? [], schema?.fields ?? []);
  const config: ExcelConfiguration = { schemaVersion: schema?.version ?? 1, kind, sheet: source.data?.sheet ?? '', headerRow, mapping, dateFormat, numberFormat, mode, cutoff, source: openingSource, ignoreUnmapped };
  const upload = useMutation({ retry: false, mutationFn: ({ file, key }: { file: File; key: string }) => { const form = new FormData(); form.append('file', file); return api<ExcelBatch>('/excel-imports/files', { method: 'POST', body: form, headers: { ...epochHeaders(), 'Idempotency-Key': key } }); }, onSuccess: value => { setBatchId(value.id); setSheet(''); setHeaderRow(1); setStep(0); setPreview(null); setResult(null); setError(''); void client.invalidateQueries({ queryKey: workspaceKey() }); }, onError: err => setError(err.message) });
  const check = useMutation({ retry: false, mutationFn: async (configuration: ExcelConfiguration) => { await command(`/excel-imports/${encodeURIComponent(batchId)}/configuration`, configuration); return api<ExcelPreview>(`/excel-imports/${encodeURIComponent(batchId)}/preview`, { method: 'POST', body: JSON.stringify(configuration), headers: epochHeaders() }); }, onSuccess: value => { setPreview(value); setStep(2); setError(''); }, onError: err => setError(err.message) });
  const save = useMutation({ retry: false, mutationFn: () => command<ExcelResult>(`/excel-imports/${encodeURIComponent(batchId)}/commit`, { configuration: config, expectedRevision: preview!.revision, fingerprint: preview!.fingerprint }), onSuccess: async value => { setResult(value); setStep(3); setError(''); void message.success(`Đã nhập ${value.count} bản ghi.${schema?.group === 'Danh mục' ? '' : ' Chứng từ và số dư đang ở trạng thái nháp.'}`); try { await client.invalidateQueries({ queryKey: workspaceKey() }); } catch { void message.warning('Đã nhập thành công; danh sách chưa tải lại được. Không nhập lại gói dữ liệu.'); } }, onError: err => setError(err.message) });
  const discard = useMutation({ retry: false, mutationFn: () => command(`/excel-imports/${encodeURIComponent(batchId)}/discard`, {}), onSuccess: async () => { setBatchId(''); setSheet(''); setPreview(null); setStep(0); await client.invalidateQueries({ queryKey: workspaceKey() }); }, onError: err => setError(err.message) });
  const busy = upload.isPending || check.isPending || save.isPending || discard.isPending;
  useEffect(() => { onBusy?.(busy); return () => onBusy?.(false); }, [busy, onBusy]);
  useEffect(() => { if (schemas.data?.length && !schemas.data.some(item => item.key === kind)) setKind(schemas.data[0].key); }, [schemas.data, kind]);
  useEffect(() => { if (source.data?.batch.result) { setResult(source.data.batch.result); setKind(source.data.batch.result.kind); setStep(3); } }, [source.data]);
  useEffect(() => {
    if (!source.data || loadedBatch.current === batchId) return;
    loadedBatch.current = batchId;
    const saved = source.data.batch.configuration;
    if (saved && !source.data.batch.result) { setKind(saved.kind); setSheet(saved.sheet); setHeaderRow(saved.headerRow); setDateFormat(saved.dateFormat); setNumberFormat(saved.numberFormat); setMode(saved.mode); setCutoff(saved.cutoff ?? ''); setOpeningSource(saved.source ?? ''); setIgnoreUnmapped(saved.ignoreUnmapped); setMapState({ key: `${batchId}:${saved.sheet}:${saved.headerRow}:${saved.kind}`, value: saved.mapping }); }
  }, [source.data, batchId]);
  const change = (action: () => void) => { action(); setPreview(null); setError(''); };
  const unlinked = source.data ? unusedExcelColumns(source.data, mapping) : [];
  const missing = schema?.fields.filter(field => field.required && mapping[field.key] === undefined) ?? [];
  const template = (advanced: boolean) => { void downloadApi(`/excel-imports/template?${new URLSearchParams({ kind, advanced: String(advanced) })}`, `mau-${kind}-${advanced ? 'nang-cao' : 'co-ban'}.xlsx`).catch(err => setError((err as Error).message)); };
  if (schemas.isPending) return <Skeleton active />;
  if (schemas.error) return <Alert type="error" title={schemas.error.message} action={<Button onClick={() => void schemas.refetch()}>Thử lại</Button>} />;
  return <div className={styles.content}>
    <Steps className={styles.steps} current={step} size="small" responsive={false} items={['Chọn tệp', 'Ghép dữ liệu', 'Kiểm tra dữ liệu', 'Kết quả'].map(title => ({ title }))} />
    {error && <Alert type="error" showIcon title={error} style={{ marginBottom: 12 }} />}
    {step < 3 && <div className={styles.grid}>
      <label className={styles.field}>Loại dữ liệu<Select aria-label="Loại dữ liệu nhập Excel" showSearch={searchOptions} value={kind} disabled={busy || step > 0} onChange={value => change(() => { setKind(value); setIgnoreUnmapped(false); })} options={[...new Set(schemas.data?.map(item => item.group))].map(group => ({ label: group, options: schemas.data!.filter(item => item.group === group).map(item => ({ value: item.key, label: item.label })) }))} /></label>
      {schema?.group === 'Danh mục' && <label className={styles.field}>Cách nhập<Select aria-label="Cách nhập danh mục" value={mode} disabled={busy || step > 1} onChange={value => change(() => setMode(value))} options={[{ value: 'add', label: 'Thêm mới · báo lỗi mã đã có' }, { value: 'update', label: 'Chỉ cập nhật theo mã đã có' }]} /></label>}
    </div>}
    {step === 0 && <>
      <p className={styles.help}>Hỗ trợ XLSX, CSV UTF-8 · Tối đa 20 MB, 5.000 dòng và 500 chứng từ/bản ghi mỗi lần. Tệp XLS cũ cần lưu lại thành XLSX.</p>
      <Upload.Dragger className={styles.upload} accept=".xlsx,.csv" multiple={false} showUploadList={false} disabled={busy} beforeUpload={file => { setError(''); if (file.size > 20 * 1024 * 1024 || !/\.(xlsx|csv)$/i.test(file.name)) { setError('Chọn XLSX hoặc CSV tối đa 20 MB.'); return Upload.LIST_IGNORE; } upload.mutate({ file, key: crypto.randomUUID() }); return Upload.LIST_IGNORE; }}>
        <span className={styles.uploadIcon}><Icon name="upload" size={26} /></span><p>{upload.isPending ? 'Đang đọc và kiểm tra tệp…' : 'Chọn tệp hoặc kéo thả tệp vào đây'}</p><small>Chưa ghi nhận dữ liệu kế toán khi tải tệp lên.</small>
      </Upload.Dragger>
      <div className={styles.templates}><Button icon={<Icon name="download" size={16} />} disabled={!schema} onClick={() => template(false)}>Tải mẫu cơ bản</Button><Button disabled={!schema} onClick={() => template(true)}>Tải mẫu nâng cao</Button><span className={styles.help}>Mẫu có hướng dẫn cột và được đặt dạng Text để giữ mã và số tiền chính xác.</span></div>
    </>}
    {batchId && step < 3 && <>
      {source.isPending && <Skeleton active paragraph={{ rows: 2 }} />}
      {source.error && <Alert type="error" title={source.error.message} action={<Button onClick={() => void source.refetch()}>Thử lại</Button>} />}
      {source.data && <div className={styles.source}><Icon name="file" /><div><strong>{source.data.batch.filename}</strong><small>Tệp đã lưu trong vùng chờ · Chưa nhập dữ liệu</small></div>{step === 0 && <Button danger type="text" disabled={busy} onClick={() => discard.mutate()}>Bỏ tệp</Button>}</div>}
    </>}
    {step === 0 && source.data && <>
      <div className={styles.grid}><label className={styles.field}>Sheet nhập khẩu<Select aria-label="Sheet nhập khẩu" value={sheet || source.data.sheet} disabled={busy} onChange={value => change(() => { setSheet(value); setIgnoreUnmapped(false); })} options={source.data.batch.sheets.map(item => ({ value: item.name, label: `${item.name} · ${item.rows} dòng` }))} /></label><label className={styles.field}>Dòng tiêu đề<InputNumber aria-label="Dòng tiêu đề" min={1} max={50} precision={0} value={headerRow} disabled={busy} onChange={value => change(() => { setHeaderRow(value ?? 1); setIgnoreUnmapped(false); })} /></label></div>
      <p className={styles.help}>Xem trước tối đa 10 dòng đầu có dữ liệu. Cột được ghép theo tiêu đề; dòng trống được bỏ qua.</p>
      <Table size="small" rowKey={row => String(row.row)} loading={source.isFetching} pagination={false} scroll={{ x: Math.max(700, source.data.headers.length * 140) }} dataSource={source.data.sample.map((cells, i) => ({ row: source.data!.sampleRows[i], cells }))} columns={[{ title: 'Dòng Excel', dataIndex: 'row', width: 85, fixed: 'left' }, ...source.data.headers.map((header, i) => ({ title: `${i + 1}. ${header}`, key: String(i), width: 160, ellipsis: true, render: (_: unknown, row: { cells: ExcelSource['sample'][number] }) => row.cells[i]?.formula ? <Tag color="red">Có công thức</Tag> : row.cells[i]?.value ?? '' }))]} />
    </>}
    {step === 1 && source.data && schema && <>
      <div className={styles.grid}><label className={styles.field}>Ngày viết dạng<Select aria-label="Định dạng ngày nhập" value={dateFormat} disabled={busy} onChange={value => change(() => setDateFormat(value))} options={[{ value: 'dmy', label: 'DD/MM/YYYY · 22/10/2026' }, { value: 'iso', label: 'YYYY-MM-DD · 2026-10-22' }]} /></label><label className={styles.field}>Số viết dạng<Select aria-label="Định dạng số nhập" value={numberFormat} disabled={busy} onChange={value => change(() => setNumberFormat(value))} options={[{ value: 'plain', label: 'Số chuẩn · 1234567.89' }, { value: 'vi', label: 'Việt Nam · 1.234.567,89' }, { value: 'en', label: 'Quốc tế · 1,234,567.89' }]} /></label></div>
      {kind === 'opening' && <><Alert type="info" showIcon title="Nhập bổ sung vào nháp số dư, chưa xác nhận" description="Chỉ dùng trước lần ghi sổ đầu tiên. Chi tiết đã có được báo trùng để tránh cộng hai lần; muốn sửa chi tiết cũ hãy mở biểu mẫu số dư." /><div className={styles.grid}><label className={styles.field}>Ngày chuyển đổi<DateInput aria-label="Ngày chuyển đổi nhập số dư" value={cutoff} disabled={busy} onChange={value => change(() => setCutoff(value))} /></label><label className={styles.field}>Nguồn số dư đã đối chiếu<Input aria-label="Nguồn số dư nhập Excel" value={openingSource} maxLength={900} disabled={busy} onChange={event => change(() => setOpeningSource(event.target.value))} placeholder="Sổ hoặc biên bản đối chiếu số dư" /></label></div></>}
      <div className={styles.mappingHeader}><span><strong>{Object.keys(mapping).length} trường đã ghép</strong> · {missing.length} trường bắt buộc còn thiếu</span><Switch checked={allFields} onChange={setAllFields} checkedChildren="Tất cả trường" unCheckedChildren="Trường cơ bản" aria-label="Hiện tất cả trường nhập Excel" /></div>
      <Table<ExcelField> rowKey="key" size="small" pagination={false} scroll={{ x: 680, y: 420 }} dataSource={schema.fields.filter(field => allFields || field.basic || field.required || mapping[field.key] !== undefined)} columns={[{ title: 'Cột trên phần mềm', width: 230, render: (_, field) => <>{field.label}{field.required && <span className={styles.required}>*</span>}{field.choices && <small className={styles.choiceHint}>{field.choices.join(' / ')}</small>}{field.default && <small className={styles.choiceHint}>Để trống: {field.default}</small>}</> }, { title: 'Cột trên tệp', width: 260, render: (_, field) => <Select className={styles.mappingSelect} allowClear showSearch={searchOptions} aria-label={`Ghép cột ${field.label}`} disabled={busy} value={mapping[field.key]} status={field.required && mapping[field.key] === undefined ? 'error' : undefined} placeholder="Chọn cột trong tệp" onChange={value => change(() => { const next = { ...mapping }; if (value === undefined) delete next[field.key]; else next[field.key] = value; setMapState({ key: mapKey, value: next }); setIgnoreUnmapped(false); })} options={source.data!.headers.map((label, value) => ({ value, label: `${value + 1}. ${label}` }))} /> }, { title: 'Dữ liệu mẫu', ellipsis: true, render: (_, field) => mapping[field.key] === undefined ? '—' : [...new Set(source.data!.sample.map(row => row[mapping[field.key]]?.value ?? '').filter(Boolean))].slice(0, 3).join(' · ') }]} />
      {unlinked.length > 0 && <Alert style={{ marginTop: 12 }} type="warning" showIcon title="Có cột dữ liệu chưa ghép" description={<><p>{unlinked.join(', ')}</p><Checkbox checked={ignoreUnmapped} disabled={busy} onChange={event => change(() => setIgnoreUnmapped(event.target.checked))}>Tôi đã kiểm tra và đồng ý bỏ qua các cột chưa ghép.</Checkbox></>} />}
      <p className={styles.help}>Không tự thêm mã danh mục, đổi tài khoản hoặc chạy công thức. Số tiền, tỷ lệ và số lượng sẽ được máy chủ kiểm tra lại.</p>
    </>}
    {step === 2 && preview && <>
      <Alert type={preview.errorCount ? 'error' : 'success'} showIcon title={preview.errorCount ? `Có ${preview.errorCount} lỗi cần sửa · chưa nhập dữ liệu` : 'Dữ liệu hợp lệ · sẵn sàng nhập'} description={preview.errorCount ? 'Quay lại ghép cột hoặc sửa tệp gốc rồi tải lại. Toàn bộ gói phải hợp lệ trước khi nhập.' : schema?.group === 'Danh mục' ? 'Máy chủ sẽ kiểm tra lại quyền, mã và phiên bản dữ liệu trước khi ghi nhận toàn bộ danh mục.' : 'Chứng từ và số dư sẽ được lưu nháp để tiếp tục đối chiếu; chưa ghi sổ hay xác nhận số dư.'} />
      <div className={styles.summary}><div><small>Dòng dữ liệu</small><strong>{preview.rows.length}</strong></div><div><small>Dòng hợp lệ</small><strong>{preview.valid}</strong></div><div><small>Lỗi kiểm tra</small><strong className={preview.errorCount ? styles.errorCount : undefined}>{preview.errorCount}</strong></div>{schema?.group !== 'Danh mục' && <><div><small>{kind === 'opening' ? 'Tổng Nợ nháp sau nhập · VND' : 'Tổng Nợ hợp lệ · VND'}</small><strong>{formatMoney(preview.debit)}</strong></div><div><small>{kind === 'opening' ? 'Tổng Có nháp sau nhập · VND' : 'Tổng Có hợp lệ · VND'}</small><strong>{formatMoney(preview.credit)}</strong></div></>}</div>
      {kind === 'opening' && <Alert type={preview.debit === preview.credit ? 'info' : 'warning'} showIcon title={preview.debit === preview.credit ? 'Tổng số dư gồm chi tiết đã có trong nháp và chi tiết mới từ tệp.' : 'Nháp số dư sau nhập chưa cân đối Nợ/Có.'} description="Có thể lưu nháp để tiếp tục bổ sung; phải đối chiếu đủ và cân đối trước khi xác nhận số dư." />}
      <div className={styles.previewToolbar}><Select aria-label="Lọc kết quả nhập Excel" value={filter} onChange={setFilter} options={[{ value: 'all', label: 'Tất cả dòng' }, { value: 'valid', label: 'Dòng hợp lệ' }, { value: 'invalid', label: 'Dòng có lỗi' }]} />{preview.issues.length > 0 && <Button icon={<Icon name="download" size={15} />} onClick={() => { void downloadApi(`/excel-imports/${encodeURIComponent(batchId)}/errors`, 'loi-nhap-excel.xlsx', { method: 'POST', headers: { ...epochHeaders(), 'Content-Type': 'application/json' }, body: JSON.stringify(config) }).catch(err => setError((err as Error).message)); }}>Tải danh sách lỗi</Button>}</div>
      <Table rowKey="row" size="small" dataSource={preview.rows.filter(row => filter === 'all' || row.status === filter)} pagination={{ pageSize: 10, showSizeChanger: false }} scroll={{ x: 750 }} columns={[{ title: 'Dòng Excel', dataIndex: 'row', width: 95 }, { title: 'Mã nguồn / mã', dataIndex: 'source', width: 170 }, { title: 'Diễn giải / tên', dataIndex: 'description' }, { title: 'Tình trạng', dataIndex: 'status', width: 110, render: value => <Tag color={value === 'valid' ? 'green' : 'red'}>{value === 'valid' ? 'Hợp lệ' : 'Có lỗi'}</Tag> }, ...(schema?.group !== 'Danh mục' ? [{ title: 'Tổng Nợ', dataIndex: 'debit', align: 'right' as const, render: (value: string, row: ExcelPreview['rows'][number]) => row.status === 'valid' ? formatMoney(value) : '—' }, { title: 'Tổng Có', dataIndex: 'credit', align: 'right' as const, render: (value: string, row: ExcelPreview['rows'][number]) => row.status === 'valid' ? formatMoney(value) : '—' }] : [])]} />
      {preview.issues.length > 0 && <Table className={styles.errorTable} size="small" rowKey={(_, i) => String(i)} dataSource={preview.issues} pagination={{ pageSize: 8, showSizeChanger: false }} scroll={{ x: 600 }} columns={[{ title: 'Dòng Excel', dataIndex: 'row', width: 95, render: value => value || 'Toàn tệp' }, { title: 'Cột', dataIndex: 'column', width: 180 }, { title: 'Lý do không hợp lệ', dataIndex: 'message' }]} />}
      {preview.errorCount > 1000 && <p className={styles.help}>Chỉ hiển thị 1.000 lỗi đầu tiên. Sửa tệp rồi kiểm tra lại để xem các lỗi còn lại.</p>}
      <p className={styles.help}>Phiên bản dữ liệu: {preview.revision}. Nếu dữ liệu thay đổi trong lúc kiểm tra, hệ thống sẽ yêu cầu kiểm tra lại.</p>
    </>}
    {step === 3 && result && <Result status="success" title={`Đã nhập ${result.count} bản ghi`} subTitle={`${result.rows} dòng nguồn đã được xử lý. ${schema?.group === 'Danh mục' ? 'Danh mục đã được cập nhật.' : 'Đã lưu nháp; chưa ghi sổ hoặc xác nhận số dư.'}`} extra={<Button type="primary" onClick={() => { if (onDone) onDone(); else { setBatchId(''); setResult(null); setPreview(null); setStep(0); } }}>Hoàn tất</Button>} />}
    {step < 3 && <div className={styles.actions}><Button disabled={busy || step === 0} onClick={() => { setStep(value => Math.max(0, value - 1)); setError(''); }}>Quay lại</Button><div>{onDone && <Button disabled={busy} onClick={onDone}>Đóng</Button>}{step === 0 && <Button type="primary" disabled={!schema || !source.data || !source.data.headers.length || source.isFetching || busy} onClick={() => setStep(1)}>Tiếp tục</Button>}{step === 1 && <Button type="primary" loading={check.isPending} disabled={busy || missing.length > 0 || unlinked.length > 0 && !ignoreUnmapped || kind === 'opening' && (!cutoff || !openingSource.trim())} onClick={() => { setError(''); check.mutate(config); }}>Kiểm tra dữ liệu</Button>}{step === 2 && <><Button disabled={busy} onClick={() => check.mutate(config)}>Kiểm tra lại</Button><Button type="primary" loading={save.isPending} disabled={busy || !preview || preview.errorCount > 0} onClick={() => save.mutate()}>Nhập dữ liệu</Button></>}</div></div>}
  </div>;
}

export function Component() {
  const { modal, message } = App.useApp();
  const client = useQueryClient();
  const [active, setActive] = useState<string | null>(null);
  const [generation, setGeneration] = useState(0);
  const [busy, setBusy] = useState(false);
  const history = useQuery({ queryKey: [...workspaceKey(), 'excel-history'], queryFn: ({ signal }) => api<ExcelBatch[]>('/excel-imports', { signal, headers: epochHeaders() }), enabled: !isDemo && apiScope().permissions !== 'viewer', retry: false });
  const remove = useMutation({ retry: false, mutationFn: (id: string) => command(`/excel-imports/${encodeURIComponent(id)}/discard`, {}), onSuccess: async () => { await client.invalidateQueries({ queryKey: workspaceKey() }); }, onError: err => void message.error(err.message) });
  if (isDemo || apiScope().permissions === 'viewer') return <Alert type="info" title={isDemo ? 'Nhập Excel sử dụng dữ liệu doanh nghiệp và backend.' : 'Tài khoản chỉ xem không có quyền nhập dữ liệu.'} />;
  return <><PageHeading title="Nhập từ Excel" description="Chọn tệp, ghép cột và kiểm tra dữ liệu trước khi ghi nhận." /><section className={appStyles.panel}><div className={styles.intro}><strong>Nhập từ Excel</strong><Button onClick={() => { setActive(null); setGeneration(value => value + 1); }} disabled={busy}>Nhập tệp mới</Button></div><div className={styles.history}><ExcelWizard key={`${active ?? 'new'}:${generation}`} initialBatch={active ?? ''} onBusy={setBusy} /></div></section><section className={appStyles.panel}><div className={styles.intro}><strong>Lịch sử nhập của bạn</strong><ToolbarButton label="Tải lại lịch sử nhập" icon="refresh" onClick={() => void history.refetch()} /></div>{history.error && <Alert type="error" title={history.error.message} />}<div className={styles.history}><Table<ExcelBatch> rowKey="id" loading={history.isPending} dataSource={history.data ?? []} pagination={{ pageSize: 8, showSizeChanger: false }} scroll={{ x: 700 }} columns={[{ title: 'Tệp', dataIndex: 'filename' }, { title: 'Thời điểm tải', dataIndex: 'createdAt', render: value => new Intl.DateTimeFormat('vi-VN', { dateStyle: 'short', timeStyle: 'short', timeZone: 'Asia/Ho_Chi_Minh' }).format(new Date(value)) }, { title: 'Trạng thái', dataIndex: 'state', render: value => <Tag color={value === 'imported' ? 'green' : 'gold'}>{value === 'imported' ? 'Đã nhập' : 'Chờ nhập'}</Tag> }, { title: 'Bản ghi', render: (_, row) => row.result?.count ?? '—' }, { title: 'Chức năng', render: (_, row) => <><Button type="link" disabled={busy} onClick={() => setActive(row.id)}>{row.state === 'imported' ? 'Xem kết quả' : 'Tiếp tục'}</Button>{row.state === 'staged' && <Button type="link" danger disabled={busy || remove.isPending} onClick={() => modal.confirm({ title: 'Bỏ tệp chờ nhập?', content: row.filename, okText: 'Bỏ tệp', cancelText: 'Giữ lại', onOk: () => remove.mutateAsync(row.id) })}>Bỏ tệp</Button>}</> }]} /></div></section></>;
}
