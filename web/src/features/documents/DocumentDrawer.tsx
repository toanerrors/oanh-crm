import { useNavigate } from 'react-router-dom';
import { CalculationDetails } from './CalculationDetails';
import { useCallback, useState } from 'react';
import { Alert, App, AutoComplete, Button, Descriptions, Drawer, Form, Input, Select, Table, Tooltip, Upload } from 'antd';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useBeforeUnload } from 'react-router-dom';
import { useWorkspace } from '../../app/WorkspaceProvider';
import { saveDemoDraft, workspaceKey } from '../../shared/data/workspace';
import { documentTypes } from '../../shared/data/types';
import type { AccountingDocument, JournalLine } from '../../shared/data/types';
import { journalEntryRows, journalEntrySides } from '../../shared/accounting/journal-entry';
import type { JournalSide } from '../../shared/accounting/journal-entry';
import { totals } from '../../shared/accounting/ledger';
import { formatDate, formatMoney, isMoney } from '../../shared/accounting/money';
import { Icon } from '../../shared/ui/Icon';
import { MoneyInput } from '../../shared/ui/MoneyInput';
import { StatusTag } from '../../shared/ui/Page';
import { searchOptions, suggestionOptions } from '../../shared/ui/search';
import styles from '../../app/App.module.css';
import { api, apiScope, command, isDemo } from '../../shared/api/client';

export function DocumentEditor() {
  const { draft, draftOpen, setDraftOpen } = useWorkspace();
  useBeforeUnload(useCallback((event: BeforeUnloadEvent) => { if (draft) { event.preventDefault(); event.returnValue = ''; } }, [draft]));
  return <Drawer className={styles.documentEditor} title={<span className={styles.drawerTitle}><Icon name="file" />{draft?.reference || 'Chứng từ nghiệp vụ khác'}<StatusTag status="draft" /></span>} open={draftOpen} size="100%" onClose={() => setDraftOpen(false)} styles={{ body: { padding: 16 } }} extra={<span className={styles.muted}>Đóng để giữ nháp trong phiên</span>}>
    {draft && <DraftForm key={draft.id} initial={draft} />}
  </Drawer>;
}

function DraftForm({ initial }: { initial: AccountingDocument }) {
  const { data, setDraft, setDraftOpen, openDocument } = useWorkspace();
  const { message, modal } = App.useApp();
  const [form] = Form.useForm<AccountingDocument>();
  const [sides, setSides] = useState(() => journalEntrySides(initial.lines));
  const client = useQueryClient();
  const lines = Form.useWatch('lines', form) as JournalLine[] | undefined;
  const validLines = (lines ?? initial.lines).map(line => ({ ...line, debit: line.debit || '0', credit: line.credit || '0' }));
  const total = validLines.every(line => isMoney(line.debit) && isMoney(line.credit)) ? totals(validLines) : null;
  const mutation = useMutation({
    mutationFn: saveDemoDraft,
    retry: false,
    onSuccess: async saved => {
      setDraft(null);
      setDraftOpen(false);
      void message.success(isDemo ? 'Đã lưu nháp trong phiên mẫu. Nháp chưa tác động đến sổ.' : 'Đã lưu nháp vào cơ sở dữ liệu. Kiểm tra chi tiết trước khi ghi sổ.');
      try { await client.invalidateQueries({ queryKey: workspaceKey() }); if (!isDemo) openDocument(saved.id); } catch { void message.warning('Đã lưu thành công; dữ liệu hiển thị chưa tải lại được.'); }
    },
    onError: (error) => { void message.error(error.message); },
  });
  if (!data) return null;
  const accountOptions = data.accounts.map(item => ({ value: item.id, label: `${item.code} · ${item.name}` }));
  const descriptions = suggestionOptions(data.documents.map(item => item.description));
  const partnerOptions = data.partners.filter(item => item.active).map(item => ({ value: item.id, label: `${item.code} · ${item.name}` }));

  function exportDraft() {
    const document = { ...initial, ...form.getFieldsValue(true), lines: validLines };
    const url = URL.createObjectURL(new Blob([JSON.stringify({ mode: isDemo ? 'demo-draft' : 'unposted-draft', currency: 'VND', scale: 0, document }, null, 2)], { type: 'application/json' }));
    const link = window.document.createElement('a');
    link.href = url;
    link.download = `nhap-${initial.id}.json`;
    link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }

  return <>
    {isDemo && <Alert type="info" showIcon title={isDemo ? 'Đang làm việc với dữ liệu mẫu' : 'Chứng từ nháp chưa tác động đến sổ'} description={isDemo ? 'Nháp chỉ được giữ trong phiên trình duyệt. Xuất file nháp trước khi tải lại hoặc đóng ứng dụng. Chức năng ghi sổ cần backend kiểm tra và lưu dữ liệu.' : 'Lưu nháp để bảo toàn dữ liệu, sau đó kiểm tra và ghi sổ tại chi tiết chứng từ. Dữ liệu đang nhập chưa lưu vẫn được giữ khi chuyển trang trong phiên.'} className={styles.formAlert} />}
    <Form form={form} layout="vertical" initialValues={initial} onValuesChange={(_, all) => setDraft({ ...initial, ...all })} onFinish={(values) => mutation.mutate({ ...initial, ...values, reference: initial.reference || `NHAP-${initial.id.slice(0, 8).toUpperCase()}`, lines: validLines })}>
      <div className={styles.manualHeader}>
        <div className={styles.manualParty}>
          <Form.Item name="type" label="Loại chứng từ" rules={[{ required: true }]}><Select options={Object.entries(documentTypes).map(([value, label]) => ({ value, label }))} /></Form.Item>
          <Form.Item name="partnerId" label="Đối tượng"><Select allowClear showSearch={searchOptions} placeholder="Chọn khách hàng / nhà cung cấp" options={partnerOptions} /></Form.Item>
          <Form.Item name="description" label="Diễn giải" rules={[{ required: true, whitespace: true, message: 'Nhập diễn giải chứng từ' }]}><AutoComplete options={descriptions} showSearch={searchOptions} defaultActiveFirstOption={false}><Input placeholder="Nhập hoặc chọn diễn giải đã dùng" maxLength={500} /></AutoComplete></Form.Item>
          {!isDemo && <Form.Item name="sourceId" label="Mã chứng từ nguồn (nếu có)"><Input placeholder="Mã nguồn dùng thống nhất khi nhập tay / import" maxLength={100} /></Form.Item>}
        </div>
        <div className={styles.manualDates}><Form.Item name="date" label="Ngày chứng từ" rules={[{ required: true, message: 'Chọn ngày chứng từ' }]}><Input type="date" /></Form.Item><Form.Item label="Số chứng từ"><Input readOnly value={initial.reference || 'Tự cấp khi ghi sổ'} /></Form.Item></div>
        <div className={styles.manualTotal}><span>Tổng Nợ</span><strong>{total ? formatMoney(total.debit) : '—'}</strong><small>VND</small></div>
      </div>
      <h3 className={styles.sectionTitle}>Chi tiết hạch toán <span>VND · Số nguyên đến đồng</span></h3>
      <p className={styles.muted}>Nhập tài khoản và đối tượng riêng cho từng bên. Các dòng Nợ/Có được giữ độc lập khi lưu chứng từ.</p>
      <Form.List name="lines">{(fields, { add, remove }) => {
        type Field = (typeof fields)[number];
        type EntryRow = { key: number; debit?: Field; credit?: Field };
        const addLine = (side: JournalSide) => { setSides(current => [...current, side]); add({ accountId: '', description: '', debit: '0', credit: '0' }); };
        const removeLine = (field: Field) => { setSides(current => current.filter((_, index) => index !== field.name)); remove(field.name); };
        const columns = (side: JournalSide) => {
          const title = side === 'debit' ? 'Nợ' : 'Có';
          return [
            { title: `TK ${title}`, width: 160, render: (_: unknown, row: EntryRow) => row[side] ? <Form.Item name={[row[side]!.name, 'accountId']} rules={[{ required: true, message: `Chọn TK ${title}` }]}><Select aria-label={`TK ${title} dòng ${row.key + 1}`} showSearch={searchOptions} options={accountOptions} placeholder={`Chọn TK ${title}`} /></Form.Item> : '—' },
            { title: `Đối tượng ${title}`, width: 160, render: (_: unknown, row: EntryRow) => row[side] ? <Form.Item name={[row[side]!.name, 'partnerId']}><Select aria-label={`Đối tượng ${title} dòng ${row.key + 1}`} allowClear showSearch={searchOptions} options={partnerOptions} placeholder={`Chọn đối tượng ${title}`} /></Form.Item> : '—' },
            { title: `Diễn giải ${title}`, width: 130, render: (_: unknown, row: EntryRow) => row[side] ? <Form.Item name={[row[side]!.name, 'description']}><AutoComplete options={descriptions} showSearch={searchOptions} defaultActiveFirstOption={false}><Input aria-label={`Diễn giải ${title} dòng ${row.key + 1}`} placeholder="Nhập hoặc chọn diễn giải" /></AutoComplete></Form.Item> : '—' },
            { title: `Số tiền ${title}`, width: 170, render: (_: unknown, row: EntryRow) => row[side] ? <Form.Item name={[row[side]!.name, side]} rules={[{ validator: async (_, value: unknown) => { if (!isMoney(value) || value === '0') throw new Error('Nhập số tiền nguyên dương hợp lệ'); } }]}><MoneyInput label={`Số tiền ${title} dòng ${row.key + 1}`} /></Form.Item> : '—' },
            { width: 32, render: (_: unknown, row: EntryRow) => row[side] ? <Button type="text" danger aria-label={`Xóa dòng ${title} ${row.key + 1}`} icon={<Icon name="trash" size={16} />} onClick={() => removeLine(row[side]!)} /> : null },
          ];
        };
        return <>
          <div className={styles.editTable}>
            <Table<EntryRow> pagination={false} size="small" rowKey="key" dataSource={journalEntryRows(fields, sides)} scroll={{ x: 1349 }} columns={[
              { title: '#', width: 45, render: (_, row) => row.key + 1 },
              { title: 'BÊN NỢ', children: columns('debit') },
              { title: 'BÊN CÓ', children: columns('credit') },
            ]} />
          </div>
          <div className={styles.journalAddActions}>
            <Button type="dashed" icon={<Icon name="plus" size={16} />} onClick={() => { setSides(current => [...current, 'debit', 'credit']); add({ accountId: '', description: '', debit: '0', credit: '0' }); add({ accountId: '', description: '', debit: '0', credit: '0' }); }}>Thêm dòng Nợ/Có</Button>
            <Button onClick={() => addLine('debit')}>Thêm bên Nợ</Button>
            <Button onClick={() => addLine('credit')}>Thêm bên Có</Button>
          </div>
        </>;
      }}</Form.List>
      <div className={styles.formTotals}><span>Tổng Nợ<strong>{total ? formatMoney(total.debit) : 'Không hợp lệ'}</strong></span><span>Tổng Có<strong>{total ? formatMoney(total.credit) : 'Không hợp lệ'}</strong></span><span className={total && total.debit === total.credit && total.debit > 0n ? styles.positive : styles.warning}>{total && total.debit === 0n && total.credit === 0n ? <>Chưa nhập số tiền</> : total && total.debit === total.credit ? <><Icon name="check" size={16} /> Cân đối Nợ / Có</> : <>Chênh lệch: {total ? formatMoney(total.debit - total.credit) : '—'}</>}</span></div>
      {total && total.debit !== total.credit && <Alert type="warning" showIcon title="Nháp đang lệch Nợ/Có. Có thể lưu để bổ sung, chưa đủ điều kiện ghi sổ." />}
      <div className={`${styles.formFooter} ${styles.draftActions}`}><Button danger type="text" onClick={() => modal.confirm({ title: 'Bỏ nội dung nháp đang nhập?', content: 'Nội dung chưa lưu trong biểu mẫu sẽ bị bỏ. Chứng từ đã lưu vẫn giữ nguyên.', okText: 'Bỏ nháp', cancelText: 'Tiếp tục nhập', onOk: () => { setDraft(null); setDraftOpen(false); } })}>Bỏ nháp</Button><div><Button icon={<Icon name="download" size={16} />} onClick={exportDraft}>Xuất nháp</Button>{isDemo && <Tooltip title="Cần kết nối backend và kiểm tra nghiệp vụ"><Button disabled>Ghi sổ</Button></Tooltip>}<Button type="primary" htmlType="submit" loading={mutation.isPending}>{isDemo ? 'Lưu nháp mẫu' : 'Lưu nháp'}</Button></div></div>
    </Form>
  </>;
}

export function DocumentDetail() {
  const navigate = useNavigate();
  const { detailId, openDocument, data, editDraft } = useWorkspace();
  const { message, modal } = App.useApp();
  const client = useQueryClient();
  const [posting, setPosting] = useState(false);
  const detail = useQuery({ queryKey: [...workspaceKey(), 'document', detailId], queryFn: ({ signal }) => api<AccountingDocument>(`/documents/${detailId}`, { signal }), enabled: !isDemo && Boolean(detailId), retry: false });
  const files = useQuery({ queryKey: [...workspaceKey(), 'attachments', detailId], queryFn: ({ signal }) => api<{ id: string; name: string; size: number }[]>(`/documents/${detailId}/attachments`, { signal }), enabled: !isDemo && Boolean(detailId), retry: false });
  const document = isDemo ? data?.documents.find(item => item.id === detailId) : detail.data;
  const total = document ? totals(document.lines) : null;
  const canWrite = isDemo || apiScope().permissions !== 'viewer';
  async function post() {
    if (!document) return;
    setPosting(true);
    try {
      const saved = await command<AccountingDocument>(`/documents/${document.id}/post`, { id: document.id, version: document.version });
      client.setQueryData([...workspaceKey(), 'document', saved.id], saved);
      void message.success(`Đã ghi sổ ${saved.reference}.`);
      try { await client.invalidateQueries({ queryKey: workspaceKey() }); } catch { void message.warning('Đã ghi sổ thành công; dữ liệu hiển thị chưa tải lại được.'); }
    } catch (error) { void message.error((error as Error).message); } finally { setPosting(false); }
  }
  function reverse() {
    if (!document) return;
    let reason = ''; let date = document.date;
    modal.confirm({ title: `Đảo chứng từ ${document.reference}`, content: <div><p>Chọn ngày thuộc kỳ mở. Giữ liên kết và toàn bộ chứng từ gốc.</p><Input type="date" defaultValue={date} onChange={event => { date = event.target.value; }} aria-label="Ngày chứng từ đảo" /><Input.TextArea placeholder="Lý do đảo chứng từ" onChange={event => { reason = event.target.value; }} aria-label="Lý do đảo chứng từ" style={{ marginTop: 12 }} /></div>, okText: 'Lập và ghi sổ chứng từ đảo', cancelText: 'Hủy', onOk: async () => {
      const saved = await command<AccountingDocument>(`/documents/${document.id}/reverse`, { id: document.id, date, reason });
      void message.success(`Đã ghi sổ ${saved.reference}.`); await client.invalidateQueries({ queryKey: workspaceKey() }); openDocument(saved.id);
    } });
  }
  return <Drawer title="Chi tiết chứng từ" open={Boolean(detailId)} size={900} onClose={() => openDocument(null)} extra={document && <Button icon={<Icon name="print" size={16} />} onClick={() => window.print()}>In chứng từ</Button>}>
    {document && data && <article className={styles.printSheet}>
      <div className={styles.documentHeading}><div><span className={styles.eyebrow}>{isDemo ? 'CHỨNG TỪ KẾ TOÁN · DỮ LIỆU MẪU' : data.companyName}</span><h2>{document.reference}</h2><span>{documentTypes[document.type]}</span></div><StatusTag status={document.status} /></div>
      <Descriptions column={2} className={styles.documentMeta} items={[
        { key: 'date', label: 'Ngày chứng từ', children: formatDate(document.date) },
        { key: 'currency', label: 'Đồng tiền', children: 'VND · Đơn vị đồng' },
        { key: 'partner', label: 'Đối tượng', children: data.partners.find(item => item.id === document.partnerId)?.name ?? 'Không áp dụng' },
        { key: 'version', label: 'Phiên bản nháp / nguồn', children: document.version },
        { key: 'description', label: 'Diễn giải', children: document.description, span: 2 },
      ]} />
      <Table dataSource={document.lines.map((line, index) => ({ ...line, key: index }))} pagination={false} scroll={{ x: 650 }} columns={[
        { title: 'Tài khoản', dataIndex: 'accountId', render: (id: string) => <><strong>{id}</strong><small className={styles.cellSub}>{data.accounts.find(item => item.id === id)?.name}</small></> },
        { title: 'Đối tượng', dataIndex: 'partnerId', render: (id: string | undefined) => data.partners.find(item => item.id === id)?.name ?? '—' },
        { title: 'Nợ', dataIndex: 'debit', align: 'right', render: (value: string) => formatMoney(value) },
        { title: 'Có', dataIndex: 'credit', align: 'right', render: (value: string) => formatMoney(value) },
      ]} summary={() => <Table.Summary.Row><Table.Summary.Cell index={0} colSpan={2}><strong>Tổng cộng</strong></Table.Summary.Cell><Table.Summary.Cell index={2} align="right"><strong>{total && formatMoney(total.debit)}</strong></Table.Summary.Cell><Table.Summary.Cell index={3} align="right"><strong>{total && formatMoney(total.credit)}</strong></Table.Summary.Cell></Table.Summary.Row>} />
      <p className={styles.muted}>{isDemo ? 'Nguồn: bộ chứng từ giả lập' : `Nguồn: ${document.sourceType ?? 'chứng từ'} / ${document.sourceId ?? document.id}`} · Phiên bản dữ liệu {data.revision}. {document.status === 'draft' ? 'Chứng từ nháp chưa tác động đến sổ.' : isDemo ? 'Trạng thái ghi sổ thuộc dữ liệu mẫu, chưa phải giao dịch thực tế.' : 'Chứng từ đã ghi sổ được giữ nguyên; sửa sai bằng chứng từ có liên kết.'}</p>
      <div className={styles.headingActions}>{!isDemo && canWrite && document.policyId && ['receipt', 'payment', 'transfer'].includes(document.sourceType ?? '') && !document.reversalOf && <Button onClick={() => { openDocument(null); navigate(`/automation?kind=treasury_reversal&original=${encodeURIComponent(document.id)}`); }}>Đảo phiếu tiền</Button>}{!isDemo && canWrite && document.status === 'posted' && document.policyId && !document.reversalOf && <CalculationDetails id={document.id} />}{!isDemo && (document.voucher || document.status === 'posted' && ['receipt', 'payment', 'transfer', 'sale', 'purchase', 'closing'].includes(document.sourceType ?? '') && document.policyId) && <Button onClick={() => { openDocument(null); navigate(`/vouchers/${document.id}`); }}>Mở biểu mẫu đầy đủ</Button>}{document.status === 'draft' && !document.voucher && canWrite && <><Button onClick={() => editDraft(document)}>Sửa bản nháp</Button>{!isDemo && <Button type="primary" loading={posting} onClick={() => modal.confirm({ title: 'Ghi sổ chứng từ?', content: 'Server sẽ kiểm tra Nợ/Có, quyền, kỳ, phiên bản nháp và nguồn nghiệp vụ trước khi commit.', okText: 'Ghi sổ', cancelText: 'Kiểm tra lại', onOk: post })}>Ghi sổ</Button>}</>}{!isDemo && canWrite && document.status === 'posted' && !document.policyId && !document.reversalOf && <Button onClick={reverse}>Lập chứng từ đảo</Button>}</div>
      {!isDemo && <section style={{ marginTop: 30 }}><h3>Chứng từ đính kèm</h3>{files.data?.map(file => <p key={file.id}><a href={`/api/v1/attachments/${file.id}`} download>{file.name}</a> <small>{file.size} byte</small></p>)}{canWrite && <Upload showUploadList={false} beforeUpload={async file => { try { const body = new FormData(); body.append('file', file); await api(`/documents/${document.id}/attachments`, { method: 'POST', body, headers: { 'X-Dataset-Epoch': apiScope().epoch, 'Idempotency-Key': crypto.randomUUID() } }); await client.invalidateQueries({ queryKey: [...workspaceKey(), 'attachments', detailId] }); void message.success('Đã lưu file đính kèm.'); } catch (error) { void message.error((error as Error).message); } return Upload.LIST_IGNORE; }}><Button>Thêm file đính kèm (tối đa 10 MB)</Button></Upload>}</section>}
    </article>}
    {!document && detailId && <Alert type={detail.error ? 'error' : 'info'} title={detail.error?.message ?? 'Đang tải chứng từ…'} />}
  </Drawer>;
}
