import { useEffect, useState } from 'react';
import { Alert, App, Button, Form, Input, InputNumber, Modal, Select, Space, Table, Tabs, Tag } from 'antd';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useWorkspace } from '../../app/WorkspaceProvider';
import { api, apiScope, command } from '../../shared/api/client';
import { openingTotals } from '../../shared/accounting/opening';
import { formatDate, formatMoney, isMoney } from '../../shared/accounting/money';
import { automationOptions, banksOptions, warehousesOptions } from '../../shared/data/operations';
import type { Asset } from '../../shared/data/operations';
import type { AccountingDocument, JournalLine } from '../../shared/data/types';
import { workspaceKey } from '../../shared/data/workspace';
import { MoneyInput } from '../../shared/ui/MoneyInput';
import { searchOptions } from '../../shared/ui/search';
import { Icon } from '../../shared/ui/Icon';
import { ToolbarButton } from '../../shared/ui/ToolbarButton';
import { DateInput } from '../../shared/ui/DateInput';
import styles from './OpeningBalances.module.css';

type OpeningLine = JournalLine & { bankAccountId?: string };
interface OpeningStock { itemId: string; warehouseId: string; quantity: string; value: string }
interface OpeningInput { id: string; version: number; cutoff: string; source: string; lines: OpeningLine[]; stocks: OpeningStock[]; assets: (Asset & { allocated: string })[] }
interface OpeningState { input: OpeningInput | null; document: AccountingDocument | null; postedCount: number; epoch: string }
const emptyLine = (): OpeningLine => ({ accountId: '', partnerId: '', description: '', debit: '0', credit: '0' });

export function OpeningBalances() {
  const { data, period, refresh } = useWorkspace();
  const client = useQueryClient();
  const { message } = App.useApp();
  const [initial, setInitial] = useState<OpeningState | null>(null);
  const state = useQuery({ queryKey: [...workspaceKey(), 'opening-balances', data?.revision], queryFn: ({ signal }) => api<OpeningState>('/opening-balances', { signal }), retry: false });
  const owner = apiScope().permissions === 'owner';
  return <>
    <ToolbarButton type="primary" loading={state.isPending} disabled={!state.data || !owner && !state.data.input} onClick={() => { if (!state.data) return; if (!state.data.input && state.data.postedCount) { void message.error('Đã có chứng từ ghi sổ. Chỉ khởi tạo số dư trước lần ghi sổ đầu tiên để tránh cộng trùng lịch sử.'); return; } setInitial(state.data); }} label={state.data?.document?.status === 'posted' ? 'Xem số dư đầu kỳ' : state.data?.input ? 'Tiếp tục nhập số dư' : 'Nhập số dư đầu kỳ'} icon="book" />
    {state.error && <ToolbarButton danger onClick={() => void state.refetch()} label="Lỗi tải số dư · Thử lại" icon="refresh" />}
    {initial && <OpeningEditor initial={initial} period={period} onClose={() => setInitial(null)} onSaved={saved => { client.setQueryData([...workspaceKey(), 'opening-balances', data?.revision], saved); void client.invalidateQueries({ queryKey: workspaceKey() }); void refresh(); }} />}
  </>;
}
function OpeningEditor({ initial, period, onClose, onSaved }: { initial: OpeningState; period: string; onClose: () => void; onSaved: (state: OpeningState) => void }) {
  const [form] = Form.useForm<OpeningInput>();
  const [saved, setSaved] = useState<OpeningInput>(() => initial.input ?? { id: crypto.randomUUID(), version: 0, cutoff: `${period}-01`, source: '', lines: [emptyLine(), emptyLine()], stocks: [], assets: [] });
  const [posted, setPosted] = useState(initial.document?.status === 'posted');
  const [dirty, setDirty] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [tab, setTab] = useState('lines');
  const { data } = useWorkspace();
  const client = useQueryClient();
  const { message, modal } = App.useApp();
  const values = Form.useWatch([], { form, preserve: true }) as OpeningInput | undefined;
  const current = { ...saved, ...values };
  const total = openingTotals(current);
  const automation = useQuery(automationOptions());
  const banks = useQuery(banksOptions(period));
  const warehouses = useQuery(warehousesOptions());
  const editable = !posted && apiScope().permissions === 'owner' && initial.postedCount === 0;
  useEffect(() => { const warn = (event: BeforeUnloadEvent) => { if (dirty) event.preventDefault(); }; window.addEventListener('beforeunload', warn); return () => window.removeEventListener('beforeunload', warn); }, [dirty]);
  const close = () => { if (!dirty) { onClose(); return; } modal.confirm({ title: 'Số dư chưa lưu', content: 'Đóng sẽ bỏ phần thay đổi chưa lưu.', okText: 'Bỏ thay đổi', cancelText: 'Tiếp tục nhập', onOk: onClose }); };
  if (!data) return null;
  const accounts = data.accounts.filter(a => a.active && !['inventory', 'asset', 'revenue', 'expense'].includes(a.group) && a.code !== '911').map(a => ({ value: a.id, label: `${a.code} · ${a.name}` }));
  const conversionPeriod = (data.periods ?? []).find(p => p.period === current.cutoff.slice(0, 7));
  const periodOpen = conversionPeriod?.closed === false;
  const openPeriod = async () => { setBusy(true); setError(''); try { await command('/periods', { period: current.cutoff.slice(0, 7), closed: false }, initial.epoch); await client.invalidateQueries({ queryKey: workspaceKey() }); } catch (e) { setError((e as Error).message); } finally { setBusy(false); } };
  const partners = data.partners.filter(p => p.active).map(p => ({ value: p.id, label: `${p.code} · ${p.name}` }));
  const amount = (_list: 'lines' | 'stocks' | 'assets', index: number, field: string, label: string) => <Form.Item name={[index, field]} rules={[{ validator: (_, value: unknown) => value === undefined || value === '' || typeof value === 'string' && isMoney(value) ? Promise.resolve() : Promise.reject(new Error('Nhập số tiền VND hợp lệ.')) }]}><MoneyInput label={`${label} dòng ${index + 1}`} /></Form.Item>;
  const remove = (index: number, action: (index: number) => void, label: string) => <Button type="text" danger aria-label={`Xóa ${label} dòng ${index + 1}`} disabled={!editable} onClick={() => { action(index); setDirty(true); }} icon={<Icon name="trash" size={16} />} />;
  const save = async () => {
    setBusy(true); setError('');
    try {
      await form.validateFields();
      const input: OpeningInput = { ...saved, ...form.getFieldsValue(true) };
      input.lines = input.lines.map(line => ({ ...line, debit: line.debit || '0', credit: line.credit || '0' }));
      const invalid = input.lines.findIndex(line => (BigInt(line.debit) > 0n) === (BigInt(line.credit) > 0n));
      if (invalid >= 0) throw new Error(`Dòng tài khoản ${invalid + 1}: nhập số tiền dương ở một bên Nợ hoặc Có.`);
      const result = await command<OpeningState>('/opening-balances', { ...input, version: saved.version }, initial.epoch);
      setSaved(result.input!); form.setFieldsValue(result.input!); setDirty(false); onSaved(result); void message.success('Đã lưu nháp số dư đầu kỳ; chưa tác động sổ.');
    } catch (e) { const field = (e as { errorFields?: { name: (string | number)[] }[] }).errorFields?.[0]?.name[0]; if (typeof field === 'string' && ['lines', 'stocks', 'assets'].includes(field)) setTab(field); setError(e instanceof Error ? e.message : 'Kiểm tra các trường nhập liệu được đánh dấu.'); }
    finally { setBusy(false); }
  };
  const discard = () => modal.confirm({ title: 'Bỏ nháp số dư đầu kỳ', content: 'Bản nháp chưa tác động sổ. Bỏ bản nháp đã lưu và các thay đổi đang nhập?', okText: 'Bỏ nháp', cancelText: 'Giữ nháp', onOk: async () => {
    try { const result = await command<OpeningState>('/opening-balances/discard', { id: saved.id, version: saved.version }, initial.epoch); onSaved(result); setDirty(false); onClose(); }
    catch (e) { setError((e as Error).message); throw e; }
  } });
  const confirm = () => modal.confirm({ title: 'Xác nhận số dư đầu kỳ', content: `Mốc chuyển đổi ${formatDate(saved.cutoff)} · Tổng Nợ và Có ${formatMoney(total?.debit ?? 0n)} VND. Số dư được khóa sau xác nhận; dữ liệu lịch sử trước mốc không được nhập lại.`, okText: 'Xác nhận số dư', cancelText: 'Kiểm tra lại', onOk: async () => {
    setBusy(true); setError('');
    try { const result = await command<OpeningState>('/opening-balances/confirm', { id: saved.id, version: saved.version }, initial.epoch); setSaved(result.input!); form.setFieldsValue(result.input!); setPosted(true); onSaved(result); void message.success('Đã xác nhận. Số dư đã cập nhật vào cột Dư đầu kỳ.'); }
    catch (e) { setError((e as Error).message); throw e; }
    finally { setBusy(false); }
  } });
  return <Modal open title={<Space>Số dư đầu kỳ<Tag color={posted ? 'green' : 'gold'}>{posted ? 'Đã xác nhận' : 'Bản nháp'}</Tag></Space>} onCancel={close} width="calc(100vw - 32px)" style={{ top: 16, maxWidth: 1600, paddingBottom: 0 }} className={styles.editor} maskClosable={false} footer={<div className={styles.actions}><Space><Button onClick={close}>Đóng</Button>{!posted && saved.version > 0 && apiScope().permissions === 'owner' && <Button danger onClick={discard} disabled={busy}>Bỏ nháp</Button>}</Space>{editable && <Space wrap><Button onClick={() => void save()} loading={busy}>Lưu nháp</Button><Button type="primary" onClick={confirm} disabled={dirty || !saved.version || !total?.balanced || !periodOpen} loading={busy}>Xác nhận số dư</Button></Space>}</div>}>
    <Alert type={posted ? 'success' : 'info'} showIcon title={posted ? `Số dư đã xác nhận tại ${formatDate(saved.cutoff)}` : 'Khởi tạo một lần trước khi ghi sổ chứng từ đầu tiên'} description={posted ? 'Số dư thuộc cột Dư đầu kỳ, không tính vào phát sinh trong kỳ. Kỳ sau được chuyển tiếp từ cùng nguồn sổ.' : 'Nhập số cuối ngày trước mốc chuyển đổi từ sổ đã đối chiếu. Lưu nháp để kiểm tra; chỉ xác nhận khi tổng Nợ bằng tổng Có và đã đủ chi tiết.'} />
    <Form form={form} initialValues={saved} layout="vertical" disabled={!editable || busy} onValuesChange={() => { setDirty(true); setError(''); }}>
      <div className={styles.parameters}><Form.Item name="cutoff" label="Ngày bắt đầu dữ liệu" rules={[{ required: true, pattern: /^\d{4}-\d{2}-01$/, message: 'Chọn ngày đầu tháng chuyển đổi.' }]}><DateInput aria-label="Ngày bắt đầu số dư" /></Form.Item><Form.Item name="source" label="Nguồn số dư / biên bản đối chiếu" rules={[{ required: true, whitespace: true, max: 900 }]}><Input aria-label="Nguồn số dư đầu kỳ" placeholder="Sổ đã đối chiếu đến ngày trước mốc chuyển đổi" /></Form.Item></div>
      {!posted && !periodOpen && <Alert className={styles.error} type="warning" title={conversionPeriod?.closed ? 'Kỳ chuyển đổi đã khóa; xử lý mở lại kỳ trong Thiết lập trước khi xác nhận.' : 'Chưa mở kỳ kế toán cho ngày chuyển đổi.'} action={!conversionPeriod && editable ? <Button disabled={busy} onClick={() => void openPeriod()}>Mở kỳ {current.cutoff.slice(0, 7).split('-').reverse().join('/')}</Button> : undefined} />}
      <Tabs activeKey={tab} onChange={setTab} items={[{ key: 'lines', label: 'Tài khoản & công nợ' }, { key: 'stocks', label: 'Tồn kho' }, { key: 'assets', label: 'Tài sản & trả trước' }]} />
      <div hidden={tab !== 'lines'}><p className={styles.help}>Nhập riêng từng đối tượng và từng tài khoản ngân hàng. Kho/tài sản nhập tại tab chi tiết, bút toán tương ứng được tạo tự động.</p><Form.List name="lines">{(fields, actions) => <><Table rowKey="key" dataSource={fields} pagination={false} scroll={{ x: 1150 }} columns={[
        { title: '#', width: 45, render: (_, f) => f.name + 1 },
        { title: 'TÀI KHOẢN', width: 240, render: (_, f) => <Form.Item name={[f.name, 'accountId']} rules={[{ required: true, message: 'Chọn tài khoản.' }]}><Select showSearch={searchOptions} aria-label={`Tài khoản số dư dòng ${f.name + 1}`} options={accounts} onChange={() => form.setFieldValue(['lines', f.name, 'bankAccountId'], undefined)} /></Form.Item> },
        { title: 'ĐỐI TƯỢNG', width: 220, render: (_, f) => <Form.Item name={[f.name, 'partnerId']}><Select showSearch={searchOptions} aria-label={`Đối tượng số dư dòng ${f.name + 1}`} allowClear options={partners} /></Form.Item> },
        { title: 'TÀI KHOẢN NGÂN HÀNG', width: 240, render: (_, f) => <Form.Item name={[f.name, 'bankAccountId']}><Select showSearch={searchOptions} aria-label={`Ngân hàng số dư dòng ${f.name + 1}`} allowClear options={(banks.data?.accounts ?? []).filter(b => b.active && b.accountId === current.lines[f.name]?.accountId).map(b => ({ value: b.id, label: `${b.number} · ${b.bankName}` }))} /></Form.Item> },
        { title: 'DƯ NỢ (VND)', width: 180, render: (_, f) => amount('lines', f.name, 'debit', 'Dư Nợ') }, { title: 'DƯ CÓ (VND)', width: 180, render: (_, f) => amount('lines', f.name, 'credit', 'Dư Có') },
        { title: '', width: 45, render: (_, f) => remove(f.name, actions.remove, 'tài khoản') },
      ]} />{editable && <Button className={styles.add} onClick={() => { actions.add(emptyLine()); setDirty(true); }}>+ Thêm dòng tài khoản</Button>}</>}</Form.List></div>
      <div hidden={tab !== 'stocks'}><p className={styles.help}>Tồn hàng hóa TK156: nhập số lượng và tổng giá trị đã đối chiếu theo từng hàng/kho; giá vốn dùng chính sách FIFO/bình quân đã xác nhận trong Cài đặt.</p><Form.List name="stocks">{(fields, actions) => <><Table rowKey="key" dataSource={fields} pagination={false} scroll={{ x: 900 }} columns={[
        { title: 'HÀNG HÓA', width: 280, render: (_, f) => <Form.Item name={[f.name, 'itemId']} rules={[{ required: true }]}><Select showSearch={searchOptions} aria-label={`Hàng tồn dòng ${f.name + 1}`} options={(automation.data?.items ?? []).filter(i => i.active && i.tracked).map(i => ({ value: i.id, label: `${i.code} · ${i.name}` }))} /></Form.Item> },
        { title: 'KHO', width: 230, render: (_, f) => <Form.Item name={[f.name, 'warehouseId']} rules={[{ required: true }]}><Select showSearch={searchOptions} aria-label={`Kho tồn dòng ${f.name + 1}`} options={(warehouses.data?.warehouses ?? []).filter(w => w.active).map(w => ({ value: w.id, label: `${w.code} · ${w.name}` }))} /></Form.Item> },
        { title: 'SỐ LƯỢNG', width: 160, render: (_, f) => <Form.Item name={[f.name, 'quantity']} rules={[{ required: true, pattern: /^(0|[1-9]\d*)(\.\d{1,6})?$/, message: 'Nhập số lượng chuẩn.' }]}><Input aria-label={`Số lượng tồn dòng ${f.name + 1}`} inputMode="decimal" /></Form.Item> },
        { title: 'GIÁ TRỊ (VND)', width: 200, render: (_, f) => amount('stocks', f.name, 'value', 'Giá trị tồn') },
        { title: '', width: 45, render: (_, f) => remove(f.name, actions.remove, 'tồn kho') },
      ]} />{editable && <Button className={styles.add} onClick={() => { actions.add({ itemId: '', warehouseId: 'default', quantity: '', value: '0' }); setDirty(true); }}>+ Thêm hàng tồn</Button>}</>}</Form.List></div>
      <div hidden={tab !== 'assets'}><p className={styles.help}>TSCĐ TK211/214 và trả trước TK242. Nhập nguyên giá gốc, lũy kế đến trước mốc và tổng thời gian sử dụng; các kỳ sau tiếp tục đường thẳng, phần dư tại tháng cuối.</p><Form.List name="assets">{(fields, actions) => <><Table rowKey="key" dataSource={fields} pagination={false} scroll={{ x: 1510 }} columns={[
        { title: 'MÃ', width: 130, render: (_, f) => <Form.Item name={[f.name, 'code']} rules={[{ required: true }]}><Input aria-label={`Mã tài sản dòng ${f.name + 1}`} /></Form.Item> },
        { title: 'TÊN TÀI SẢN / KHOẢN TRẢ TRƯỚC', width: 230, render: (_, f) => <Form.Item name={[f.name, 'name']} rules={[{ required: true }]}><Input aria-label={`Tên tài sản dòng ${f.name + 1}`} /></Form.Item> },
        { title: 'LOẠI', width: 170, render: (_, f) => <Form.Item name={[f.name, 'kind']}><Select aria-label={`Loại tài sản dòng ${f.name + 1}`} options={[{ value: 'fixed', label: 'Tài sản cố định' }, { value: 'prepaid', label: 'Chi phí trả trước' }]} /></Form.Item> },
        { title: 'NGUYÊN GIÁ', width: 185, render: (_, f) => amount('assets', f.name, 'cost', 'Nguyên giá') },
        { title: 'LŨY KẾ ĐÃ PHÂN BỔ', width: 185, render: (_, f) => amount('assets', f.name, 'allocated', 'Lũy kế') },
        { title: 'GIÁ TRỊ THU HỒI', width: 160, render: (_, f) => amount('assets', f.name, 'residual', 'Giá trị thu hồi') },
        { title: 'TỪ THÁNG', width: 170, render: (_, f) => <Form.Item name={[f.name, 'startPeriod']} rules={[{ required: true }]}><DateInput picker="month" aria-label={`Kỳ bắt đầu tài sản dòng ${f.name + 1}`} /></Form.Item> },
        { title: 'TỔNG SỐ THÁNG', width: 145, render: (_, f) => <Form.Item name={[f.name, 'months']} rules={[{ required: true }]}><InputNumber min={1} max={1200} precision={0} aria-label={`Số tháng tài sản dòng ${f.name + 1}`} /></Form.Item> },
        { title: '', width: 45, render: (_, f) => remove(f.name, actions.remove, 'tài sản') },
      ]} />{editable && <Button className={styles.add} onClick={() => { actions.add({ id: crypto.randomUUID(), code: '', name: '', kind: 'fixed', cost: '0', allocated: '0', residual: '0', months: 12, startPeriod: saved.cutoff.slice(0, 7) }); setDirty(true); }}>+ Thêm tài sản / trả trước</Button>}</>}</Form.List></div>
    </Form>
    {(automation.error || banks.error || warehouses.error) && <Alert className={styles.error} type="error" title="Chưa tải đủ danh mục chi tiết" description={(automation.error || banks.error || warehouses.error)?.message} />}
    {error && <Alert className={styles.error} type="error" showIcon title={error} />}
    <div className={styles.totals}><span>Tổng dư Nợ<strong>{total ? formatMoney(total.debit) : '—'}</strong></span><span>Tổng dư Có<strong>{total ? formatMoney(total.credit) : '—'}</strong></span><span>Chênh lệch Nợ − Có<strong>{total ? formatMoney(total.difference) : '—'}</strong></span><Tag color={total?.balanced ? 'green' : 'gold'}>{total?.balanced ? 'Cân đối' : 'Chưa cân đối'}</Tag></div>
  </Modal>;
}
