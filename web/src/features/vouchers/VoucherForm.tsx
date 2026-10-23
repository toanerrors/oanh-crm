import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Alert, App, AutoComplete, Button, ConfigProvider, Form, Input, Modal, Radio, Select, Switch, Table, Tabs, Tag, Upload } from 'antd';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useBeforeUnload, useBlocker, useLocation, useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { useWorkspace } from '../../app/WorkspaceProvider';
import { api, apiScope, command, isDemo } from '../../shared/api/client';
import { automationOptions, banksOptions } from '../../shared/data/operations';
import type { Calculation, InvoiceList, OperationInput, PurchaseCostSource, Voucher } from '../../shared/data/operations';
import type { AccountingDocument } from '../../shared/data/types';
import { workspaceKey } from '../../shared/data/workspace';
import { businessToday, formatMoney, isMoney } from '../../shared/accounting/money';
import { MoneyInput } from '../../shared/ui/MoneyInput';
import { Icon } from '../../shared/ui/Icon';
import { searchOptions, suggestionOptions } from '../../shared/ui/search';
import { BankSelect } from './BankAccounts';
import { WarehouseSelect } from './Warehouses';
import { ItemSelect } from './ItemSelect';
import { DateInput } from '../../shared/ui/DateInput';
import { voucherReturnPath } from '../documents/list-state';
import { editorShortcut } from './editor-shortcut';
import { voucherDraftKey, voucherKind } from './draft-state';
import styles from './VoucherForm.module.css';

type Kind = OperationInput['kind'];
const titles: Partial<Record<Kind, string>> = { receipt: 'Phiếu thu', payment: 'Phiếu chi', transfer: 'Chuyển tiền nội bộ', sale: 'Chứng từ bán hàng', purchase: 'Chứng từ mua hàng', closing: 'Kết chuyển lãi lỗ' };

export function Component() {
  const { id } = useParams();

  const [params] = useSearchParams();
  const routeInput = params.toString();
  const generatedSource = useMemo(() => crypto.randomUUID(), [id, routeInput]);
  const { period, voucherDrafts, setVoucherDraft } = useWorkspace();
  const location = useLocation();
  const draftKey = voucherDraftKey(id, params);
  const query = useQuery({ queryKey: [...workspaceKey(), 'voucher', id], queryFn: ({ signal }) => api<Voucher>(`/documents/${id}/voucher`, { signal }), enabled: Boolean(id) && !isDemo, staleTime: Infinity, retry: false });
  const navigate = useNavigate();
  if (isDemo) return <Alert type="info" title="Biểu mẫu nghiệp vụ dùng dữ liệu doanh nghiệp và backend." />;
  if (id && !query.data) return <Alert type={query.error ? 'error' : 'info'} title={query.error?.message ?? 'Đang tải chứng từ…'} action={<Button onClick={() => navigate('/documents')}>Về danh sách</Button>} />;
  const kind = voucherKind(params);
  const date = businessToday().startsWith(period) ? businessToday() : `${period}-01`;
  const monthEnd = new Date(Date.UTC(Number(period.slice(0, 4)), Number(period.slice(5, 7)), 0)).toISOString().slice(0, 10);
  const retained = voucherDrafts[draftKey];
  const resumed = retained?.input;
  if (resumed && query.data?.document.status === 'posted') return <Alert type="warning" title="Nháp đã được ghi sổ từ một phiên khác" description="Nội dung đang nhập vẫn được giữ để xuất và đối chiếu với bản đã ghi sổ." action={<><Button onClick={() => exportInput(resumed)}>Xuất nội dung đang nhập</Button><Button onClick={() => setVoucherDraft(draftKey, null)}>Xem bản đã ghi sổ</Button></>} />;
  const stale = Boolean(resumed && query.data && resumed.draftVersion !== query.data.document.version);
  const initial: OperationInput = structuredClone(resumed ?? query.data?.input ?? {
    kind, sourceId: generatedSource, date: kind === 'closing' ? monthEnd : date,
    description: kind === 'closing' ? `Kết chuyển lãi lỗ kỳ ${period}` : '',
    voucher: { documentDate: kind === 'closing' ? monthEnd : date }, tradeMode: params.get('service') === '1' ? 'service' : 'goods',
    moneyAccount: params.get('account') === '1121' ? '1121' : '111', destinationAccount: kind === 'transfer' ? '1121' : undefined,
    priceMode: 'exclusive', discount: { mode: 'none', allocation: 'manual' }, lines: [{ itemId: '', quantity: '1', unitPrice: '', vatRate: '', accountId: kind === 'sale' ? '511' : params.get('service') === '1' ? '642' : '156' }],
    treasuryLines: [{ counterAccount: '', description: '', amount: '' }],
  });
  if ((initial.kind === 'receipt' || initial.kind === 'payment') && !initial.treasuryLines?.length && !initial.allocations?.length && initial.counterAccount && initial.amount) {
    initial.treasuryLines = [{ counterAccount: initial.counterAccount, amount: initial.amount, description: initial.description, partnerId: initial.partnerId }];
  }
  return <VoucherForm key={id ? `${id}-${resumed?.draftVersion ?? query.data?.document.version}` : generatedSource} initial={initial} draftKey={draftKey} draftRoute={location.pathname + location.search} initialAllocate={retained?.allocate ?? (!id && (kind === 'receipt' || kind === 'payment') && params.get('settlement') === '1')} resuming={Boolean(resumed)} stale={stale} onReload={() => setVoucherDraft(draftKey, null)} initialDocument={query.data ? { ...query.data.document, version: resumed?.draftVersion ?? query.data.document.version } : undefined} initialCalculation={resumed ? undefined : query.data?.calculation} replacement={query.data?.replacement} reversal={query.data?.reversal} />;
}

function exportInput(input: OperationInput) {
  const url = URL.createObjectURL(new Blob([JSON.stringify({ mode: 'unposted-voucher', currency: 'VND', input }, null, 2)], { type: 'application/json' }));
  const link = document.createElement('a'); link.href = url; link.download = `nhap-${input.sourceId}.json`; link.click(); setTimeout(() => URL.revokeObjectURL(url), 1000);
}

function VoucherForm({ initial, draftKey, draftRoute, initialAllocate, initialDocument, initialCalculation, resuming, stale, onReload, replacement, reversal }: { initial: OperationInput; draftKey: string; draftRoute: string; initialAllocate: boolean; initialDocument?: AccountingDocument; initialCalculation?: Calculation; replacement?: AccountingDocument; reversal?: AccountingDocument; resuming: boolean; stale: boolean; onReload: () => void }) {
  const { data, voucherDrafts, setVoucherDraft, period, openDocument } = useWorkspace();
  const { message, modal } = App.useApp();
  const navigate = useNavigate();
  const location = useLocation();
  const allowNavigation = useRef(false);
  const submitting = useRef(false);
  const client = useQueryClient();
  const [form] = Form.useForm<OperationInput>();
  const [saved, setSaved] = useState(initialDocument);
  const [dirty, setDirty] = useState(resuming);
  const [preview, setPreview] = useState<{ input: OperationInput; result: Calculation } | null>(initialCalculation ? { input: initial, result: initialCalculation } : null);
  const [tab, setTab] = useState('entries');
  const [allocate, setAllocate] = useState(initialAllocate || Boolean(initial.allocations?.length));
  const [showNotes, setShowNotes] = useState(Boolean(initial.voucher?.notes));
  const [showAccounts, setShowAccounts] = useState(true);
  const auto = useQuery({ ...automationOptions(), enabled: apiScope().permissions !== 'viewer' });
  const kind = (Form.useWatch('kind', form) as Kind | undefined) ?? initial.kind;
  const date = (Form.useWatch('date', form) as string | undefined) ?? initial.date;
  const bankAccountId = Form.useWatch('bankAccountId', form) as string | undefined;
  const destinationBankAccountId = Form.useWatch('destinationBankAccountId', form) as string | undefined;
  const bankQuery = useQuery(banksOptions(period));
  const bankAccount = bankQuery.data?.accounts.find(item => item.id === bankAccountId);
  const destinationBank = bankQuery.data?.accounts.find(item => item.id === destinationBankAccountId);
  const bankName = bankAccount?.bankName ?? (bankAccountId && bankAccountId === initial.bankAccountId ? initial.voucher?.bankName : undefined);
  const destinationBankName = destinationBank?.bankName ?? (destinationBankAccountId && destinationBankAccountId === initial.destinationBankAccountId ? initial.voucher?.destinationBankName : undefined);
  const partnerId = Form.useWatch('partnerId', form) as string | undefined;
  const immediateAccount = Form.useWatch('paymentAccount', form) as string | undefined;
  const transferAmount = Form.useWatch('amount', form) as string | undefined;
  const moneyAccount = (Form.useWatch('moneyAccount', form) as string | undefined) ?? initial.moneyAccount;
  const mode = (Form.useWatch('tradeMode', form) as OperationInput['tradeMode']) ?? initial.tradeMode;
  const costAllocation = (Form.useWatch('costAllocation', form) as string | undefined) ?? initial.costAllocation ?? 'manual';
  const discountMode = Form.useWatch(['discount', 'mode'], form) as string | undefined;
  const discountAllocation = Form.useWatch(['discount', 'allocation'], form) as string | undefined;
  const costSources = useQuery({ queryKey: [...workspaceKey(), 'purchase-costs', date], queryFn: ({ signal }) => api<PurchaseCostSource[]>(`/purchase-costs?asOf=${date}`, { signal }), enabled: kind === 'purchase' && mode === 'goods', retry: false });
  const transferTo = Form.useWatch('destinationAccount', form) as string | undefined;
  const treasuryRows = Form.useWatch('treasuryLines', form) as OperationInput['treasuryLines'];
  const allocations = Form.useWatch('allocations', form) as OperationInput['allocations'];
  const readonly = saved?.status === 'posted' || apiScope().permissions === 'viewer';
  const trade = kind === 'sale' || kind === 'purchase';
  const transfer = kind === 'transfer';
  const closing = kind === 'closing';
  const bank = Boolean(bankAccount) || moneyAccount === '1121' || moneyAccount === '112' || Boolean(bankQuery.data?.accounts.some(item => item.accountId === moneyAccount));
  const policy = auto.data?.policies.find(item => item.effectiveFrom <= date);
  const partner = data?.partners.find(item => item.id === partnerId);
  const title = trade && mode === 'service' ? `Chứng từ ${kind === 'sale' ? 'bán' : 'mua'} dịch vụ` : bank && !transfer ? `${kind === 'receipt' ? 'Thu' : 'Chi'} tiền gửi` : titles[kind];
  const invoicesParams = new URLSearchParams({ asOf: date, kind: kind === 'receipt' ? 'sale' : 'purchase', partner: partnerId ?? '', pageSize: '100' });
  const invoices = useQuery({ queryKey: [...workspaceKey(), 'invoices', invoicesParams.toString()], queryFn: ({ signal }) => api<InvoiceList>(`/trade-invoices?${invoicesParams}`, { signal }), enabled: allocate && !trade && !transfer && Boolean(partnerId && date) && !readonly, retry: false });
  const files = useQuery({ queryKey: [...workspaceKey(), 'attachments', saved?.id], queryFn: ({ signal }) => api<{ id: string; name: string; size: number }[]>(`/documents/${saved?.id}/attachments`, { signal }), enabled: Boolean(saved), retry: false });
  const calculate = useMutation({ retry: false, mutationFn: (input: OperationInput) => api<Calculation>('/operations/preview', { method: 'POST', body: JSON.stringify(input), headers: { 'X-Dataset-Epoch': apiScope().epoch } }), onSuccess: (result, requestedInput) => { try { if (JSON.stringify(input()) === JSON.stringify(requestedInput)) { setPreview({ input: requestedInput, result }); if (!closing) setTab('preview'); } } catch { setPreview(null); } }, onError: error => void message.error(error.message) });
  const save = useMutation({ retry: false, mutationFn: (input: OperationInput) => command<Voucher>('/voucher-drafts', input), onSuccess: async result => {
    client.setQueryData([...workspaceKey(), 'voucher', result.document.id], result);
    setSaved(result.document); form.setFieldsValue(result.input); setDirty(false); setPreview(null); setVoucherDraft(draftKey, null);
    void message.success('Đã lưu nháp. Sổ kế toán và tồn kho chưa thay đổi.');
    allowNavigation.current = true;
    navigate(`/vouchers/${result.document.id}`, { replace: true, state: { returnTo } });
    try { await client.invalidateQueries({ queryKey: workspaceKey() }); } catch { void message.warning('Đã lưu nháp; dữ liệu danh sách chưa tải lại được.'); }
  }, onError: error => void message.error(error.message) });
  const execute = useMutation({ retry: false, mutationFn: (input: OperationInput) => command<Calculation>('/operations', input), onSuccess: async result => {
    client.setQueryData([...workspaceKey(), 'voucher', result.document.id], { input: preview!.input, document: result.document, calculation: result });
    setSaved(result.document); setDirty(false); setVoucherDraft(draftKey, null); setPreview({ input: preview!.input, result });
    allowNavigation.current = true;
    navigate(`/vouchers/${result.document.id}`, { replace: true, state: { returnTo } });
    void message.success(`Đã ghi sổ ${result.document.reference}${result.paymentDocument ? ` và ${result.paymentDocument.reference}` : ''}.`);
    try { await client.invalidateQueries({ queryKey: workspaceKey() }); } catch { void message.warning('Đã ghi sổ; dữ liệu hiển thị chưa tải lại được.'); }
  }, onError: error => void message.error(error.message) });
  const pending = save.isPending || calculate.isPending || execute.isPending;
  useBeforeUnload(useCallback((event: BeforeUnloadEvent) => { if (dirty || pending) { event.preventDefault(); event.returnValue = ''; } }, [dirty, pending]));
  const returnTo = voucherReturnPath(location.state?.returnTo, trade ? kind === 'sale' ? '/sales' : '/purchases' : closing ? '/documents?source=closing' : bank ? '/bank' : '/cash');
  const blocker = useBlocker(() => !allowNavigation.current && !readonly && (dirty || pending));
  useEffect(() => {
    if (blocker.state !== 'blocked') return;
    if (pending) { blocker.reset(); void message.info('Đang xử lý chứng từ. Vui lòng chờ phản hồi trước khi chuyển màn hình.'); return; }
    setVoucherDraft(draftKey, { input: { ...form.getFieldsValue(true), draftId: saved?.id, draftVersion: saved?.version }, allocate, route: draftRoute, returnTo });
    allowNavigation.current = true;
    blocker.proceed();
  }, [blocker, pending, form, saved?.id, saved?.version, setVoucherDraft, draftKey, draftRoute, allocate, returnTo, message]);
  const partyOptions = data?.partners.filter(item => item.active && (!trade || item.role === 'both' || item.role === (kind === 'sale' ? 'customer' : 'supplier'))).map(item => ({ value: item.id, label: `${item.code} · ${item.name}` })) ?? [];
  const moneyOptions = data?.accounts.filter(item => item.active && (['111', '1121', '112'].includes(item.id) || bankQuery.data?.accounts.some(bank => bank.accountId === item.id))).map(item => ({ value: item.id, label: `${item.code} · ${item.name}`, code: item.code }));
  const counterOptions = data?.accounts.filter(item => item.active && !['cash', 'inventory', 'asset', 'receivable', 'payable'].includes(item.group)).map(item => ({ value: item.id, label: `${item.code} · ${item.name}`, code: item.code }));
  const descriptionOptions = suggestionOptions(data?.documents.map(item => item.description) ?? []);
  const currentRows = allocate ? allocations : treasuryRows;
  const localTotal = currentRows?.every(row => isMoney(row.amount)) ? currentRows.reduce((sum, row) => sum + BigInt(row.amount), 0n).toString() : undefined;
  const total = preview?.result.amounts.total ?? preview?.result.amounts.result ?? (transfer ? transferAmount : !trade && !closing ? localTotal : undefined);

  function input() {
    const value: OperationInput = structuredClone(form.getFieldsValue(true));
    value.draftId = saved?.id; value.draftVersion = saved?.version;
    delete value.expectedPolicyId; delete value.expectedRevision;
    if (!trade && !transfer && !closing) {
      const rows = allocate ? value.allocations : value.treasuryLines;
      value.amount = (rows ?? []).reduce((sum, row) => sum + BigInt(row.amount), 0n).toString();
      if (allocate) { delete value.treasuryLines; delete value.counterAccount; } else { delete value.allocations; delete value.counterAccount; }
    }
    if (trade) { delete value.treasuryLines; delete value.allocations; delete value.moneyAccount; delete value.destinationAccount; delete value.destinationBankAccountId; delete value.amount; }
    else { delete value.lines; delete value.tradeMode; delete value.priceMode; delete value.paymentAccount; delete value.discount; delete value.isPurchaseCost; delete value.purchaseCosts; delete value.costAllocation; }
    if (transfer || closing) { delete value.treasuryLines; delete value.allocations; delete value.counterAccount; }
    return value;
  }
  async function submit(action: 'save' | 'preview') {
    if (pending || submitting.current) return;
    submitting.current = true;
    try { await form.validateFields(); const payload = input(); if (action === 'save') await save.mutateAsync(payload).catch(() => undefined); else await calculate.mutateAsync(payload).catch(() => undefined); }
    catch (error) {
      if (error && typeof error === 'object' && 'errorFields' in error) {
        const first = (error as { errorFields?: { name: (string | number)[] }[] }).errorFields?.[0];
        if (first) { setTab('entries'); requestAnimationFrame(() => form.scrollToField(first.name, { focus: true, block: 'center' })); }
      } else if (error instanceof Error) void message.error(error.message);
    } finally { submitting.current = false; }
  }
  function close() {
    navigate(returnTo);
  }
  function correct() {
    if (!saved) return;
    const next = structuredClone(initial);
    delete next.draftId; delete next.draftVersion; delete next.expectedRevision; delete next.expectedPolicyId;
    next.sourceId = crypto.randomUUID(); next.replacementOf = saved.id; next.replacementReason = '';
    const currentDate = businessToday().startsWith(period) ? businessToday() : `${period}-01`;
    next.date = currentDate < saved.date ? saved.date : currentDate;
    next.voucher = { ...next.voucher, documentDate: next.date, reference: '' };
    const params = new URLSearchParams({ kind, replacement: saved.id });
    if (bank) params.set('account', '1121');
    const route = `/vouchers/new?${params}`;
    const replacementKey = voucherDraftKey(undefined, params);
    if (!voucherDrafts[replacementKey]) setVoucherDraft(replacementKey, { input: next, allocate, route, returnTo });
    navigate(route, { state: { returnTo } });
  }
  function confirmPosting() {
    if (!preview) return;
    modal.confirm({ title: initial.replacementOf ? 'Xác nhận sửa và thay thế phiếu?' : closing ? 'Kết chuyển và khóa kỳ kế toán?' : 'Ghi sổ chứng từ đã kiểm tra?', content: initial.replacementOf ? 'Phiếu cũ được đảo, phiếu đã sửa được ghi trong cùng giao dịch. Lịch sử và lý do thay thế được giữ đầy đủ.' : closing ? 'Server kiểm tra lại dữ liệu, kết chuyển và khóa kỳ trong cùng transaction.' : 'Server tính lại, kiểm tra kỳ và phiên bản. Chứng từ đã ghi sổ được giữ nguyên.', okText: 'Ghi sổ', cancelText: 'Kiểm tra lại', onOk: () => execute.mutateAsync({ ...preview.input, expectedPolicyId: preview.result.policyId, expectedRevision: preview.result.revision }).then(() => undefined) });
  }
  function changed() { setDirty(true); setPreview(null); }
  const totalsBox = <dl className={styles.sums}>
    {trade && <><div><dt>Chiết khấu</dt><dd>{preview ? formatMoney(preview.result.amounts.discount ?? '0') : 'Chưa tính'}</dd></div>{kind === 'purchase' && mode === 'goods' && <div><dt>Chi phí mua hàng</dt><dd>{preview ? formatMoney(preview.result.amounts.purchaseCost ?? '0') : 'Chưa tính'}</dd></div>}<div><dt>{mode === 'service' ? 'Tổng tiền dịch vụ' : 'Tổng tiền hàng'}</dt><dd>{preview ? formatMoney(preview.result.amounts.base) : 'Chưa tính'}</dd></div><div><dt>Thuế GTGT</dt><dd>{preview ? formatMoney(preview.result.amounts.tax) : 'Chưa tính'}</dd></div></>}
    <div><dt>{closing ? 'Kết quả kinh doanh' : 'Tổng tiền thanh toán'}</dt><dd>{total !== undefined ? formatMoney(total) : 'Chưa tính'} VND</dd></div>
  </dl>;

  return <ConfigProvider theme={{ token: { controlHeight: 28 } }}><Modal open closable={false} footer={null} width="100vw" keyboard={!pending} onCancel={close} className={styles.modal} style={{ top: 0, paddingBottom: 0, maxWidth: 'none' }} styles={{ body: { padding: 0 } }} aria-labelledby="voucher-title"><section className={styles.screen} onKeyDown={event => {
    const action = editorShortcut(event.nativeEvent);
    if (!action) return;
    event.preventDefault();
    if (!readonly && !pending && policy && !stale && !(closing && action === 'save')) void submit(action);
  }}>
    <header className={styles.titlebar}><h1 id="voucher-title" className={!readonly && !saved && (kind === 'receipt' || kind === 'payment' || trade) ? styles.mobileSrOnly : undefined}>{title} {saved?.reference}</h1>{(kind === 'receipt' || kind === 'payment') && !readonly && <Form.Item noStyle><Select className={styles.kindSelect} popupMatchSelectWidth={false} aria-label="Loại phiếu" disabled={pending || Boolean(saved) || Boolean(initial.replacementOf)} value={kind} options={[{ value: 'receipt', label: bank ? 'Thu tiền gửi' : 'Phiếu thu' }, { value: 'payment', label: bank ? 'Chi tiền gửi' : 'Phiếu chi' }]} onChange={value => { const params = new URLSearchParams(location.search); params.set('kind', value); navigate(`/vouchers/new?${params}`, { state: { returnTo } }); }} /></Form.Item>}{trade && <Select className={styles.tradeSelect} popupMatchSelectWidth={false} aria-label="Loại hàng hóa dịch vụ" disabled={readonly || pending || Boolean(saved)} value={mode} options={[{ value: 'goods', label: kind === 'sale' ? 'Bán hàng hóa trong nước' : 'Mua hàng trong nước' }, { value: 'service', label: kind === 'sale' ? 'Bán dịch vụ' : 'Mua dịch vụ' }]} onChange={value => { const params = new URLSearchParams(location.search); if (value === 'service') params.set('service', '1'); else params.delete('service'); navigate(`/vouchers/new?${params}`, { state: { returnTo } }); }} />}{!trade && !transfer && !closing && <div className={styles.modeSelect}><Select popupMatchSelectWidth={false} aria-label="Nội dung thu chi" disabled={readonly || pending} value={allocate ? 'invoice' : 'other'} options={[{ value: 'other', label: kind === 'receipt' ? 'Thu khác' : 'Chi khác' }, { value: 'invoice', label: kind === 'receipt' ? 'Thu công nợ theo chứng từ' : 'Trả công nợ theo chứng từ' }]} onChange={value => { if (!saved) { const params = new URLSearchParams(location.search); if (value === 'invoice') params.set('settlement', '1'); else params.delete('settlement'); navigate(`/vouchers/new?${params}`, { state: { returnTo } }); return; } setAllocate(value === 'invoice'); form.setFieldsValue({ allocations: [], treasuryLines: [{ counterAccount: '', description: '', amount: '' }] }); setPreview(null); setDirty(true); }} /></div>}{readonly && <Tag color="green">{saved?.status === 'posted' ? 'Đã ghi sổ' : 'Chỉ xem'}</Tag>}<Button type="text" icon={<Icon name="close" />} aria-label="Đóng chứng từ" disabled={pending} onClick={close} /></header>
    <div className={styles.body}>
      {saved?.status === 'posted' && (replacement || reversal) && <Alert className={styles.info} showIcon type="info" title={replacement ? `Đã thay thế bằng ${replacement.reference}` : `Đã đảo bằng ${reversal!.reference}`} action={<Button onClick={() => replacement ? navigate(`/vouchers/${replacement.id}`) : openDocument(reversal!.id)}>{replacement ? 'Mở phiếu đã sửa' : 'Xem nghiệp vụ đảo'}</Button>} />}
      {initial.replacementOf && <Alert className={styles.info} showIcon type="info" title={saved?.status === 'posted' ? 'Phiếu thay thế đã ghi sổ' : 'Bản sửa thay thế phiếu đã ghi sổ'} description={saved?.status === 'posted' ? 'Phiếu này thay thế phiếu gốc; lý do và chứng từ đảo được giữ trong lịch sử.' : 'Các giá trị cũ đã được điền sẵn. Chọn ngày hạch toán hợp lệ, sửa nội dung và nhập lý do. Chỉ khi ghi sổ, hệ thống mới đảo phiếu cũ và ghi phiếu mới cùng lúc.'} action={<Button onClick={() => navigate(`/vouchers/${initial.replacementOf}`)}>Xem phiếu gốc</Button>} />}

      {stale && <Alert className={styles.info} type="warning" showIcon title="Nháp trên máy chủ đã có phiên bản mới. Nội dung đang nhập được giữ để đối chiếu; chưa thể ghi đè." action={<><Button onClick={() => exportInput(form.getFieldsValue(true))}>Xuất nội dung đang nhập</Button><Button onClick={() => modal.confirm({ title: 'Chuyển sang bản mới trên máy chủ?', content: 'Xuất nội dung đang nhập trước khi chuyển nếu cần giữ lại để đối chiếu.', okText: 'Dùng bản trên máy chủ', cancelText: 'Tiếp tục đối chiếu', onOk: onReload })}>Dùng bản trên máy chủ</Button></>} />}
      {auto.error && <Alert className={styles.info} type="error" title={auto.error.message} action={<Button onClick={() => void auto.refetch()}>Tải lại</Button>} />}
      {!policy && !auto.isPending && !readonly && <Alert className={styles.info} type="warning" title="Chưa có chính sách hiệu lực. Xác nhận chính sách trong Cài đặt trước khi lưu hoặc tính." />}
      <Form<OperationInput> className={styles.form} form={form} initialValues={initial} layout="vertical" disabled={readonly || pending} onValuesChange={changed} onFinish={() => void submit('preview')}>
        {initial.replacementOf && <><Form.Item name="replacementOf" hidden><Input /></Form.Item><Form.Item name="replacementReason" label="Lý do sửa phiếu" rules={[{ required: true, whitespace: true, max: 900 }]}><Input.TextArea aria-label="Lý do sửa phiếu" autoSize={{ minRows: 2, maxRows: 4 }} maxLength={900} /></Form.Item></>}
        <Form.Item name="kind" hidden><Input /></Form.Item><Form.Item name="tradeMode" hidden><Input /></Form.Item>
        {trade && <div className={styles.options}><Radio.Group value={immediateAccount ? 'now' : 'later'} onChange={event => { form.setFieldsValue({ paymentAccount: event.target.value === 'now' ? '111' : undefined, bankAccountId: undefined }); setPreview(null); setDirty(true); }} options={[{ value: 'later', label: kind === 'sale' ? 'Chưa thu tiền' : 'Chưa thanh toán' }, { value: 'now', label: kind === 'sale' ? 'Thu tiền ngay' : 'Thanh toán ngay' }]} /><Form.Item name="paymentAccount"><Select aria-label="Tài khoản thanh toán ngay" onChange={() => form.setFieldValue('bankAccountId', undefined)} allowClear placeholder="Chọn tài khoản thanh toán ngay" options={moneyOptions} /></Form.Item>{immediateAccount && immediateAccount !== '111' && <Form.Item name="bankAccountId" rules={[{ required: true, message: 'Chọn tài khoản ngân hàng thanh toán' }]}><BankSelect accountId={immediateAccount} label="Tài khoản ngân hàng thanh toán" disabled={readonly || pending} onSelected={account => { form.setFieldValue('paymentAccount', account?.accountId); }} /></Form.Item>}<Form.Item name="priceMode"><Select aria-label="Đơn giá nhập" options={[{ value: 'exclusive', label: 'Đơn giá chưa gồm VAT' }, { value: 'inclusive', label: 'Đơn giá đã gồm VAT' }]} /></Form.Item></div>}
        <div className={styles.header}>
          <div className={styles.party}>
            {!closing && !transfer && <><Form.Item name="partnerId" label={trade ? kind === 'sale' ? 'Mã khách hàng' : 'Mã nhà cung cấp' : 'Mã đối tượng'} rules={[{ required: trade || allocate, message: 'Chọn đối tượng' }]}><Select allowClear showSearch={searchOptions} options={partyOptions} onChange={() => { if (allocate) form.setFieldValue('allocations', []); }} /></Form.Item><Form.Item label={trade ? kind === 'sale' ? 'Tên khách hàng' : 'Tên nhà cung cấp' : 'Tên đối tượng'}><Input readOnly value={partner?.name ?? ''} /></Form.Item><Form.Item name={['voucher', 'contact']} label={kind === 'receipt' ? 'Người nộp' : kind === 'payment' ? 'Người nhận' : kind === 'sale' ? 'Người liên hệ' : 'Người giao hàng'}><Input maxLength={200} /></Form.Item><Form.Item name={['voucher', 'address']} label="Địa chỉ"><Input maxLength={500} /></Form.Item></>}
            {!trade && !closing && !transfer && !bank && <Form.Item name="moneyAccount" hidden><Input /></Form.Item>}
            {bank && !transfer && !trade && !closing && <><Form.Item name="bankAccountId" label={kind === 'receipt' ? 'Nộp vào tài khoản' : 'Chi từ tài khoản'} rules={[{ required: !readonly, message: 'Chọn tài khoản ngân hàng' }]}><BankSelect accountId={moneyAccount} label="Tài khoản ngân hàng thu chi" disabled={readonly || pending} onSelected={account => form.setFieldValue('moneyAccount', account?.accountId)} /></Form.Item><Form.Item label="Tên ngân hàng"><Input readOnly value={bankName ?? 'Chưa gắn tài khoản ngân hàng'} /></Form.Item><Form.Item name="moneyAccount" label="Tài khoản hạch toán" rules={[{ required: true }]}><Select showSearch={searchOptions} options={moneyOptions} onChange={() => form.setFieldValue('bankAccountId', undefined)} /></Form.Item></>}
            {transfer && <><Form.Item label="Tài khoản đi"><div className={styles.bankAccount}><Form.Item name="moneyAccount" noStyle rules={[{ required: true }]}><Select aria-label="Tài khoản hạch toán chuyển đi" optionLabelProp="code" popupMatchSelectWidth={320} options={moneyOptions} onChange={() => form.setFieldValue('bankAccountId', undefined)} /></Form.Item>{moneyAccount !== '111' ? <Form.Item name="bankAccountId" noStyle rules={[{ required: !readonly }]}><BankSelect accountId={moneyAccount} label="Tài khoản ngân hàng chuyển đi" disabled={readonly || pending} onSelected={account => form.setFieldValue('moneyAccount', account?.accountId)} /></Form.Item> : <Input readOnly value="Tiền mặt" />}</div></Form.Item><Form.Item label="Tên ngân hàng chuyển đi"><Input readOnly value={bankName ?? ''} /></Form.Item><Form.Item label="Tài khoản đến"><div className={styles.bankAccount}><Form.Item name="destinationAccount" noStyle rules={[{ required: true }]}><Select aria-label="Tài khoản hạch toán chuyển đến" optionLabelProp="code" popupMatchSelectWidth={320} options={moneyOptions} onChange={() => form.setFieldValue('destinationBankAccountId', undefined)} /></Form.Item>{transferTo !== '111' ? <Form.Item name="destinationBankAccountId" noStyle rules={[{ required: !readonly }]}><BankSelect accountId={transferTo} label="Tài khoản ngân hàng chuyển đến" disabled={readonly || pending} onSelected={account => form.setFieldValue('destinationAccount', account?.accountId)} /></Form.Item> : <Input readOnly value="Tiền mặt" />}</div></Form.Item><Form.Item label="Ngân hàng nhận"><Input readOnly value={destinationBankName ?? ''} /></Form.Item></>}
            {!closing && !transfer && (!bank || trade) && <Form.Item name={['voucher', 'employee']} label={trade ? kind === 'sale' ? 'Nhân viên bán hàng' : 'Nhân viên mua hàng' : 'Nhân viên'}><Input maxLength={200} /></Form.Item>}
            <Form.Item className={closing || transfer ? styles.full : undefined} name="description" label={transfer ? 'Lý do chuyển' : kind === 'receipt' ? 'Lý do nộp' : kind === 'payment' ? 'Lý do chi' : 'Diễn giải'} rules={[{ required: true, whitespace: true, message: 'Nhập diễn giải' }]}><AutoComplete showSearch={searchOptions} options={descriptionOptions} defaultActiveFirstOption={false}><Input maxLength={1000} /></AutoComplete></Form.Item>
            <details className={styles.references}><summary>Tham chiếu và thông tin bổ sung</summary><div className={styles.party}><Form.Item name={['voucher', 'reference']} label="Tham chiếu"><Input maxLength={500} /></Form.Item><Form.Item name="sourceId" label="Mã chứng từ nguồn" rules={[{ required: true }, { pattern: /^[A-Za-z0-9][A-Za-z0-9_.:/-]{0,99}$/, message: 'Dùng mã không dấu, tối đa 100 ký tự' }]}><Input maxLength={100} /></Form.Item><Form.Item name={['voucher', 'originalCount']} label="Số chứng từ gốc kèm theo"><Input maxLength={20} inputMode="numeric" /></Form.Item>{bank && <Form.Item name={['voucher', 'employee']} label="Nhân viên thu/chi"><Input maxLength={200} /></Form.Item>}</div></details>
            {trade && <><Form.Item name="dueDate" label="Hạn thanh toán"><DateInput /></Form.Item><Form.Item name={['voucher', 'contractNumber']} label="Số hợp đồng / đơn hàng"><Input maxLength={100} /></Form.Item></>}
          </div>
          <div className={styles.dates}><Form.Item name="date" label="Ngày hạch toán" rules={[{ required: true }]}><DateInput /></Form.Item><Form.Item name={['voucher', 'documentDate']} label="Ngày chứng từ" rules={[{ required: true }]}><DateInput /></Form.Item><Form.Item label="Số chứng từ"><Input readOnly value={saved?.reference ?? 'Tự cấp khi ghi sổ'} /></Form.Item></div>
          <div className={styles.total}><span>{closing ? 'Kết quả kinh doanh' : 'Tổng tiền'}</span><strong>{total !== undefined ? formatMoney(total) : '—'}</strong><small>VND{!preview && trade ? ' · Chưa tính' : ''}</small></div>
        </div>

        {trade && <div className={styles.options}>
          <Form.Item name={['discount', 'mode']} label="Chiết khấu"><Select aria-label="Loại chiết khấu" value={discountMode ?? 'none'} onChange={() => { const lines = (form.getFieldValue('lines') as NonNullable<OperationInput['lines']>).map(line => ({ ...line, discountRate: undefined, discountAmount: undefined })); form.setFieldValue('lines', lines); changed(); }} options={[{ value: 'none', label: 'Không chiết khấu' }, { value: 'line', label: 'Theo mặt hàng' }, { value: 'percent', label: 'Theo % hóa đơn' }, { value: 'total', label: 'Theo số tiền hóa đơn' }]} /></Form.Item>
          {discountMode === 'percent' && <Form.Item name={['discount', 'rate']} label="Tỷ lệ CK (%)" rules={[{ required: true }]}><Input aria-label="Tỷ lệ chiết khấu hóa đơn" inputMode="decimal" /></Form.Item>}
          {discountMode === 'total' && <><Form.Item name={['discount', 'amount']} label="Tiền chiết khấu" rules={[{ required: true }]}><MoneyInput label="Tổng tiền chiết khấu" /></Form.Item><Form.Item name={['discount', 'allocation']} label="Phân bổ chiết khấu" rules={[{ required: true }]}><Select aria-label="Phương pháp phân bổ chiết khấu" onChange={() => { form.setFieldValue('lines', (form.getFieldValue('lines') as NonNullable<OperationInput['lines']>).map(line => ({ ...line, discountRate: undefined, discountAmount: undefined }))); changed(); }} options={[{ value: 'manual', label: 'Tự nhập từng dòng' }, { value: 'quantity', label: 'Theo số lượng' }, { value: 'value', label: 'Theo giá trị' }]} /></Form.Item></>}
          {kind === 'purchase' && mode === 'service' && <Form.Item name="isPurchaseCost" label="Là chi phí mua hàng" valuePropName="checked"><Switch /></Form.Item>}
          <small>Chiết khấu trừ trước khi tính thuế; đơn giá gồm VAT thì nhập CK gồm VAT. Phân bổ tự động giữ đủ tổng đồng, ưu tiên phần lẻ lớn rồi thứ tự dòng.</small>
        </div>}
        {closing && <Alert className={styles.info} showIcon type="warning" title="Kết chuyển doanh thu, chi phí qua 911 và kết quả sang 421; khóa kỳ khi thành công." description="Kiểm tra thuế và đối soát trước khi ghi sổ. Ngày hạch toán phải là ngày cuối kỳ; mở lại kỳ sẽ tạo chứng từ đảo kết chuyển." />}
        <section className={styles.panel}>
          <Tabs className={styles.tabs} activeKey={tab} onChange={setTab} items={[{ key: 'entries', label: trade && mode === 'goods' ? 'Hàng tiền' : 'Hạch toán' }, ...(trade ? [{ key: 'tax', label: 'Thuế / hóa đơn' }, ...(kind === 'purchase' && mode === 'goods' ? [{ key: 'costs', label: 'Chi phí mua hàng' }] : [])] : []), ...(preview ? [{ key: 'preview', label: 'Bút toán server tính' }] : [])]} />
          {tab === 'entries' && kind === 'purchase' && mode === 'goods' && <small>TK 156: nhập kho. TK chi phí hoặc dở dang: không nhập kho.</small>}
          {tab === 'entries' && trade && <Form.List name="lines" rules={[{ validator: async (_, rows: unknown[]) => { if (!rows?.length) throw new Error('Thêm ít nhất một dòng hàng.'); } }]}>{(fields, { add, remove }, { errors }) => <>
            <Table dataSource={fields} rowKey="key" pagination={false} size="small" scroll={{ x: showAccounts ? 1560 : 1250 }} columns={[
              { title: '#', width: 42, render: (_, __, index) => index + 1 },
              { title: mode === 'service' ? 'Mã dịch vụ' : 'Mã hàng', width: 230, render: (_, field) => <Form.Item name={[field.name, 'itemId']} rules={[{ required: true, message: 'Chọn hàng/dịch vụ' }]}><ItemSelect label={`Hàng dịch vụ dòng ${field.name + 1}`} kind={mode === 'service' ? 'service' : 'goods'} disabled={readonly || pending} onSelected={item => { form.setFieldValue(['lines', field.name, 'description'], item.name); setPreview(null); setDirty(true); }} /></Form.Item> },
              { title: mode === 'service' ? 'Tên dịch vụ / diễn giải' : 'Tên hàng / diễn giải', width: 260, render: (_, field) => <Form.Item name={[field.name, 'description']}><Input aria-label={`Diễn giải dòng ${field.name + 1}`} maxLength={1000} /></Form.Item> },
              ...(showAccounts ? [{ title: kind === 'sale' ? 'TK doanh thu' : 'TK hạch toán', width: 180, render: (_: unknown, field: { name: number }) => <Form.Item name={[field.name, 'accountId']}><Select aria-label={`Tài khoản dòng ${field.name + 1}`} showSearch={searchOptions} options={data?.accounts.filter(a => a.active && (kind === 'sale' ? a.group === 'revenue' : ['expense', 'wip'].includes(a.group) || mode === 'goods' && a.id === '156')).map(a => ({ value: a.id, label: `${a.code} · ${a.name}` }))} /></Form.Item> }, { title: 'TK công nợ', width: 95, render: () => kind === 'sale' ? '131' : '331' }] : []),
              ...(mode === 'goods' ? [{ title: 'Kho', width: 210, render: (_: unknown, field: { name: number }) => <Form.Item noStyle shouldUpdate={(previous, current) => previous.lines?.[field.name]?.accountId !== current.lines?.[field.name]?.accountId}>{() => { const stock = kind === 'sale' || !form.getFieldValue(['lines', field.name, 'accountId']) || form.getFieldValue(['lines', field.name, 'accountId']) === '156'; return stock ? <Form.Item name={[field.name, 'warehouseId']} initialValue="default" rules={[{ required: true }]}><WarehouseSelect label={`Kho dòng ${field.name + 1}`} disabled={readonly || pending} /></Form.Item> : <span>Không nhập kho</span>; }}</Form.Item> }] : []),
              { title: 'ĐVT', width: 80, render: (_, field) => auto.data?.items.find(item => item.id === form.getFieldValue(['lines', field.name, 'itemId']))?.unit },
              { title: 'Số lượng', width: 115, render: (_, field) => <Form.Item name={[field.name, 'quantity']} rules={[{ required: true }]}><Input aria-label={`Số lượng dòng ${field.name + 1}`} inputMode="decimal" /></Form.Item> },
              { title: 'Đơn giá', width: 150, render: (_, field) => <Form.Item name={[field.name, 'unitPrice']} rules={[{ required: true }]}><Input aria-label={`Đơn giá dòng ${field.name + 1}`} inputMode="decimal" /></Form.Item> },
              ...((discountMode === 'line' || discountMode === 'total' && discountAllocation === 'manual') ? [{ title: 'Tỷ lệ CK (%)', width: 115, render: (_: unknown, field: { name: number }) => <Form.Item name={[field.name, 'discountRate']}><Input aria-label={`Tỷ lệ chiết khấu dòng ${field.name + 1}`} inputMode="decimal" onChange={() => form.setFieldValue(['lines', field.name, 'discountAmount'], undefined)} /></Form.Item> }, { title: 'Tiền chiết khấu', width: 150, render: (_: unknown, field: { name: number }) => <Form.Item name={[field.name, 'discountAmount']}><MoneyInput label={`Tiền chiết khấu dòng ${field.name + 1}`} onChange={() => form.setFieldValue(['lines', field.name, 'discountRate'], undefined)} /></Form.Item> }] : []),
              { title: 'Thành tiền sau CK', align: 'right', width: 150, render: (_, field) => preview?.result.lineAmounts?.[field.name] ? formatMoney(preview.result.lineAmounts[field.name].base) : '—' },
              { title: '% Thuế GTGT', width: 115, render: (_, field) => <Form.Item name={[field.name, 'vatRate']} rules={[{ required: true }]}><Select aria-label={`VAT dòng ${field.name + 1}`} options={policy?.vatRates.map(value => ({ value, label: value }))} /></Form.Item> },
              { title: 'Tiền thuế GTGT', align: 'right', width: 145, render: (_, field) => preview?.result.lineAmounts?.[field.name] ? formatMoney(preview.result.lineAmounts[field.name].tax) : '—' },
              ...(kind === 'purchase' && policy?.vatMethod === 'deduction' ? [{ title: 'Đủ điều kiện khấu trừ', width: 180, render: (_: unknown, field: { name: number }) => <Form.Item name={[field.name, 'deductible']} rules={[{ required: true }]}><Select aria-label={`Khấu trừ VAT dòng ${field.name + 1}`} options={[{ value: true, label: 'Có' }, { value: false, label: 'Không' }]} /></Form.Item> }] : []),
              { width: 44, render: (_, field) => !readonly && <Button type="text" danger icon={<Icon name="trash" size={16} />} aria-label={`Xóa dòng ${field.name + 1}`} onClick={() => { remove(field.name); setPreview(null); setDirty(true); }} /> },
            ]} />
            <Form.ErrorList errors={errors} />{!readonly && <div className={styles.tableActions}><Button icon={<Icon name="plus" size={15} />} onClick={() => { add({ itemId: '', quantity: '1', unitPrice: '', vatRate: '', accountId: kind === 'sale' ? '511' : mode === 'service' ? '642' : '156' }); setPreview(null); setDirty(true); }}>Thêm dòng</Button><Button danger onClick={() => { form.setFieldValue('lines', []); setPreview(null); setDirty(true); }}>Xóa hết dòng</Button><Button onClick={() => setShowNotes(true)}>Thêm ghi chú</Button><small>Tổng số: {fields.length} · Tiền sẽ được server tính khi xem trước</small></div>}
          </>}</Form.List>}
          {tab === 'entries' && !trade && !closing && !transfer && !allocate && <Form.List name="treasuryLines" rules={[{ validator: async (_, rows: unknown[]) => { if (!rows?.length) throw new Error('Thêm ít nhất một dòng thu/chi.'); } }]}>{(fields, { add, remove }, { errors }) => <>
            <Table rowKey="key" dataSource={fields} pagination={false} size="small" scroll={{ x: 1100 }} columns={[
              { title: '#', width: 40, render: (_, __, index) => index + 1 },
              { title: 'Diễn giải', render: (_, field) => <Form.Item name={[field.name, 'description']}><Input aria-label={`Diễn giải dòng ${field.name + 1}`} maxLength={1000} /></Form.Item> },
              { title: 'TK Nợ', width: 130, render: (_, field) => kind === 'receipt' ? moneyAccount : <Form.Item name={[field.name, 'counterAccount']} rules={[{ required: true }]}><Select aria-label={`Tài khoản đối ứng dòng ${field.name + 1}`} optionLabelProp="code" showSearch={searchOptions} options={counterOptions} /></Form.Item> },
              { title: 'TK Có', width: 130, render: (_, field) => kind === 'payment' ? moneyAccount : <Form.Item name={[field.name, 'counterAccount']} rules={[{ required: true }]}><Select aria-label={`Tài khoản đối ứng dòng ${field.name + 1}`} optionLabelProp="code" showSearch={searchOptions} options={counterOptions} /></Form.Item> },
              { title: 'Số tiền', width: 170, align: 'right', render: (_, field) => <Form.Item name={[field.name, 'amount']} rules={[{ required: true }, { validator: async (_, value: string) => { if (!isMoney(value) || value === '0') throw new Error('Nhập số nguyên dương'); } }]}><MoneyInput label={`Số tiền dòng ${field.name + 1}`} /></Form.Item> },
              { title: 'Đối tượng', width: 350, render: (_, field) => <Form.Item name={[field.name, 'partnerId']}><Select aria-label={`Đối tượng dòng ${field.name + 1}`} allowClear showSearch={searchOptions} options={partyOptions} /></Form.Item> },
              { width: 44, render: (_, field) => !readonly && <Button type="text" danger aria-label={`Xóa dòng ${field.name + 1}`} icon={<Icon name="trash" size={16} />} onClick={() => { remove(field.name); setPreview(null); setDirty(true); }} /> },
            ]} summary={() => <Table.Summary.Row><Table.Summary.Cell index={0} colSpan={4}>Tổng</Table.Summary.Cell><Table.Summary.Cell index={4} align="right">{localTotal ? formatMoney(localTotal) : '—'}</Table.Summary.Cell><Table.Summary.Cell index={5} colSpan={2} /></Table.Summary.Row>} />
            <Form.ErrorList errors={errors} />{!readonly && <div className={styles.tableActions}><Button onClick={() => add({ counterAccount: '', description: form.getFieldValue('description'), amount: '' })} icon={<Icon name="plus" size={15} />}>Thêm dòng</Button><Button danger onClick={() => { form.setFieldValue('treasuryLines', []); setDirty(true); setPreview(null); }}>Xóa hết dòng</Button><Button onClick={() => setShowNotes(true)}>Thêm ghi chú</Button><small>Tổng số: {fields.length}</small></div>}
          </>}</Form.List>}
          {tab === 'entries' && allocate && !trade && !transfer && <>
            {invoices.error && <Alert type="error" title={invoices.error.message} />}
            <Form.List name="allocations" rules={[{ validator: async (_, rows: unknown[]) => { if (!rows?.length) throw new Error('Thêm chứng từ thanh toán.'); } }]}>{(fields, { add, remove }, { errors }) => <><Table rowKey="key" dataSource={fields} pagination={false} scroll={{ x: 750 }} columns={[{ title: '#', width: 40, render: (_, __, index) => index + 1 }, { title: 'Chứng từ công nợ nguồn', render: (_, field) => <Form.Item name={[field.name, 'documentId']} rules={[{ required: true }]}><AutoComplete aria-label={`Chứng từ công nợ dòng ${field.name + 1}`} showSearch={searchOptions} options={invoices.data?.invoices.filter(item => item.remaining !== '0' && !item.unallocated).map(item => ({ value: item.documentId, label: `${item.reference} · Còn ${formatMoney(item.remaining)} VND` }))}><Input /></AutoComplete></Form.Item> }, { title: 'Số tiền phân bổ', width: 220, render: (_, field) => <Form.Item name={[field.name, 'amount']} rules={[{ required: true }, { validator: async (_, value: string) => { if (!isMoney(value) || value === '0') throw new Error('Nhập số nguyên dương'); } }]}><MoneyInput label={`Số phân bổ dòng ${field.name + 1}`} /></Form.Item> }, { width: 44, render: (_, field) => !readonly && <Button type="text" danger icon={<Icon name="trash" size={16} />} aria-label={`Xóa phân bổ ${field.name + 1}`} onClick={() => remove(field.name)} /> }]} /><Form.ErrorList errors={errors} />{!readonly && <div className={styles.tableActions}><Button onClick={() => add({ documentId: '', amount: '' })}>Thêm chứng từ thanh toán</Button><small>Chọn nguồn cùng đối tượng; server chặn thanh toán vượt công nợ.</small></div>}</>}</Form.List>
          </>}
          {tab === 'entries' && transfer && <Table rowKey={() => 'transfer'} dataSource={[{}]} pagination={false} scroll={{ x: 700 }} columns={[{ title: '#', width: 40, render: () => 1 }, { title: 'Diễn giải', render: () => form.getFieldValue('description') }, { title: 'TK Nợ', width: 160, render: () => transferTo }, { title: 'TK Có', width: 160, render: () => moneyAccount }, { title: 'Số tiền', width: 200, render: () => <Form.Item name="amount" rules={[{ required: true }]}><MoneyInput label="Số tiền chuyển" /></Form.Item> }]} />}
          {tab === 'costs' && kind === 'purchase' && mode === 'goods' && <div className={styles.preview}>
            <p>Chọn chứng từ mua dịch vụ đã đánh dấu “Là chi phí mua hàng”. Nhập số phân bổ từ từng nguồn và số phân bổ vào từng dòng hàng bằng nhau.</p>
            {costSources.error && <Alert type="error" title={costSources.error.message} />}
            <Form.Item name="costAllocation" label="Phương pháp phân bổ chi phí" initialValue="manual"><Select aria-label="Phương pháp phân bổ chi phí" onChange={() => { form.setFieldValue('lines', (form.getFieldValue('lines') as NonNullable<OperationInput['lines']>).map(line => ({ ...line, purchaseCost: undefined }))); changed(); }} options={[{ value: 'manual', label: 'Tự nhập giá trị' }, { value: 'quantity', label: 'Theo số lượng' }, { value: 'value', label: 'Theo thành tiền sau CK, chưa VAT' }, { value: 'percent', label: 'Tự nhập tỷ lệ %' }]} /></Form.Item>
            <Form.List name="purchaseCosts">{(fields, { add, remove }) => <><Table rowKey="key" dataSource={fields} pagination={false} scroll={{ x: 700 }} columns={[
              { title: 'Chứng từ chi phí / dòng nguồn', render: (_, field) => <><Form.Item name={[field.name, 'documentId']} rules={[{ required: true }]} getValueProps={value => ({ value: value ? `${value}#${form.getFieldValue(['purchaseCosts', field.name, 'line']) ?? 0}` : undefined })} getValueFromEvent={(value: string) => value.split('#')[0]}><Select aria-label={`Nguồn chi phí dòng ${field.name + 1}`} showSearch={searchOptions} options={costSources.data?.map(source => ({ value: `${source.documentId}#${source.line}`, label: `${source.reference} · Dòng ${source.line + 1} · ${source.description} · Còn ${formatMoney(source.remaining)}` }))} onChange={(value: string) => form.setFieldValue(['purchaseCosts', field.name, 'line'], Number(value.split('#')[1]))} /></Form.Item><Form.Item name={[field.name, 'line']} hidden><Input /></Form.Item></> },
              { title: 'Số phân bổ lần này', width: 200, render: (_, field) => <Form.Item name={[field.name, 'amount']} rules={[{ required: true }]}><MoneyInput label={`Chi phí nguồn dòng ${field.name + 1}`} /></Form.Item> },
              { width: 60, render: (_, field) => !readonly && <Button danger type="text" aria-label={`Bỏ nguồn chi phí ${field.name + 1}`} onClick={() => remove(field.name)}>Bỏ</Button> },
            ]} />{!readonly && <Button onClick={() => add({ line: 0, amount: '' })}>Chọn chứng từ chi phí</Button>}</>}</Form.List>
            <Form.List name="lines">{fields => <Table rowKey="key" dataSource={fields} pagination={false} columns={[{ title: 'Dòng hàng nhập kho', render: (_, field) => `${field.name + 1}. ${form.getFieldValue(['lines', field.name, 'description']) ?? ''}` }, { title: 'Chi phí phân bổ vào hàng', width: 220, render: (_, field) => costAllocation === 'manual' ? <Form.Item name={[field.name, 'purchaseCost']}><MoneyInput label={`Chi phí mua hàng dòng ${field.name + 1}`} /></Form.Item> : costAllocation === 'percent' ? <Form.Item name={[field.name, 'costPercent']} rules={[{ required: true }]}><Input aria-label={`Tỷ lệ phân bổ chi phí dòng ${field.name + 1}`} inputMode="decimal" /></Form.Item> : <span>{preview?.result.lineAmounts?.[field.name]?.purchaseCost ? formatMoney(preview.result.lineAmounts[field.name].purchaseCost!) : 'Tính khi xem trước'}</span> }]} />}</Form.List>
            <p className={styles.note}>Chỉ tổng thanh toán hàng hóa ghi vào công nợ chứng từ này. Chi phí nguồn có công nợ riêng; khoản phân bổ được chuyển từ tài khoản nguồn vào giá trị kho, không ghi nhận chi phí hoặc VAT lần hai.</p>
          </div>}
          {tab === 'tax' && trade && <div className={styles.preview}><div className={styles.party}><Form.Item name="invoiceNumber" label="Số hóa đơn"><Input maxLength={100} /></Form.Item><Form.Item name="invoiceSeries" label="Ký hiệu hóa đơn"><Input maxLength={100} /></Form.Item><Form.Item name="invoiceDate" label="Ngày hóa đơn"><DateInput /></Form.Item><Form.Item name={['voucher', 'taxCode']} label="Mã số thuế"><Input maxLength={100} /></Form.Item><Form.Item name={['voucher', 'invoiceLookupCode']} label="Mã tra cứu HĐĐT"><Input maxLength={200} /></Form.Item><Form.Item name={['voucher', 'invoiceLookupUrl']} label="Đường dẫn tra cứu HĐĐT"><Input maxLength={1000} /></Form.Item></div><p className={styles.note}>Thông tin hóa đơn dùng để đối chiếu chứng từ. Thuế suất và điều kiện khấu trừ nhập tại từng dòng; server tính lại theo chính sách hiệu lực.</p></div>}
          {(tab === 'preview' || closing) && <div className={styles.preview}>{preview ? <><h2>Bút toán server tính · Chính sách v{preview.result.policyVersion}</h2><Table rowKey={(_, index) => String(index)} dataSource={preview.result.document.lines} pagination={false} scroll={{ x: 850 }} columns={[{ title: '#', width: 40, render: (_, __, index) => index + 1 }, { title: 'Diễn giải', dataIndex: 'description' }, { title: 'Tài khoản', dataIndex: 'accountId', width: 100 }, { title: 'Đối tượng', dataIndex: 'partnerId', width: 130 }, { title: 'Nợ', dataIndex: 'debit', align: 'right', width: 170, render: formatMoney }, { title: 'Có', dataIndex: 'credit', align: 'right', width: 170, render: formatMoney }]} />{preview.result.paymentDocument && <><h2>Phiếu {kind === 'sale' ? 'thu' : 'chi'} liên kết {readonly ? preview.result.paymentDocument.reference : 'sẽ được ghi cùng chứng từ'}</h2><Table rowKey={(_, index) => String(index)} dataSource={preview.result.paymentDocument.lines} pagination={false} scroll={{ x: 750 }} columns={[{ title: 'Tài khoản', dataIndex: 'accountId' }, { title: 'Diễn giải', dataIndex: 'description' }, { title: 'Nợ', dataIndex: 'debit', align: 'right', render: formatMoney }, { title: 'Có', dataIndex: 'credit', align: 'right', render: formatMoney }]} />{readonly && <Button onClick={() => navigate(`/vouchers/${preview.result.paymentDocument!.id}`)}>Mở phiếu {kind === 'sale' ? 'thu' : 'chi'} liên kết</Button>}</>}{preview.result.notes.map(note => <p className={styles.note} key={note}>{note}</p>)}</> : <p className={styles.note}>Chọn “Tính và xem trước” để lấy các dòng hạch toán từ server.</p>}</div>}
          <div className={styles.bottom}><div className={styles.extras}>{showNotes && <Form.Item name={['voucher', 'notes']} label="Ghi chú"><Input.TextArea rows={2} maxLength={5000} /></Form.Item>}<h3>Đính kèm <small>· Tối đa 10 MB/tệp</small></h3><Upload.Dragger disabled={!saved || pending || readonly} showUploadList={false} beforeUpload={async file => { if (!saved) return Upload.LIST_IGNORE; try { const body = new FormData(); body.append('file', file); await api(`/documents/${saved.id}/attachments`, { method: 'POST', body, headers: { 'X-Dataset-Epoch': apiScope().epoch, 'Idempotency-Key': crypto.randomUUID() } }); await client.invalidateQueries({ queryKey: [...workspaceKey(), 'attachments', saved.id] }); void message.success('Đã lưu tệp đính kèm.'); } catch (error) { void message.error((error as Error).message); } return Upload.LIST_IGNORE; }}><Icon name="download" size={22} /><p className="ant-upload-text">Chọn tệp hoặc kéo và thả tệp vào đây</p><small>{saved ? 'Tệp được lưu cùng chứng từ đã lưu' : 'Lưu nháp trước khi thêm tệp đính kèm'}</small></Upload.Dragger>{files.data?.map(file => <p key={file.id}><a href={`/api/v1/attachments/${file.id}`} download>{file.name}</a></p>)}</div>{totalsBox}</div>
        </section>
      </Form>
    </div>
    <footer className={styles.footer}><small>{policy ? `Chính sách v${policy.version} · Kỳ ${period}` : preview ? `Chính sách v${preview.result.policyVersion} · Kỳ ${period}` : auto.isFetching ? 'Đang tải chính sách…' : auto.error ? 'Chưa tải được chính sách' : 'Chưa có chính sách hiệu lực'}{dirty && !readonly ? ' · Nội dung chưa lưu' : ''}</small><div>{trade && <Button onClick={() => setShowAccounts(value => !value)}>{showAccounts ? 'Ẩn tài khoản' : 'Hiện tài khoản'}</Button>}{dirty && !readonly && <Button onClick={() => exportInput(form.getFieldsValue(true))}>Xuất nháp</Button>}{dirty && !readonly && <Button danger type="text" disabled={pending} onClick={() => modal.confirm({ title: 'Bỏ nội dung chưa lưu?', content: 'Chứng từ đã cất nháp vẫn được giữ trên máy chủ.', okText: 'Bỏ nội dung', cancelText: 'Tiếp tục nhập', onOk: () => { allowNavigation.current = true; setVoucherDraft(draftKey, null); setDirty(false); navigate(returnTo); } })}>Bỏ nội dung chưa lưu</Button>}<Button disabled={pending} onClick={close}>Đóng</Button>{saved && <Button disabled={dirty} onClick={() => window.open(`/api/v1/documents/${saved.id}/print`, '_blank', 'noopener,noreferrer')}>In chứng từ</Button>}{saved?.status === 'posted' && !saved.reversalOf && !reversal && !replacement && ['receipt', 'payment', 'transfer'].includes(kind) && apiScope().permissions !== 'viewer' && <Button type="primary" onClick={correct}>Sửa / thay thế phiếu</Button>}{!readonly && <>{!closing && <Button title="Ctrl/⌘ S · Lưu nháp" loading={save.isPending} disabled={pending || !policy} onClick={() => void submit('save')}>Cất nháp <kbd aria-hidden="true" className={styles.shortcut}>Ctrl/⌘ S</kbd></Button>}<Button title="Ctrl/⌘ Enter · Tính và xem trước" loading={calculate.isPending} disabled={pending || !policy} onClick={() => void submit('preview')}>Tính và xem trước <kbd aria-hidden="true" className={styles.shortcut}>Ctrl/⌘ ↵</kbd></Button>{preview && <Button type="primary" loading={execute.isPending} disabled={pending || closing && apiScope().permissions !== 'owner'} onClick={confirmPosting}>Ghi sổ</Button>}</>}</div></footer>
  </section></Modal></ConfigProvider>;
}
