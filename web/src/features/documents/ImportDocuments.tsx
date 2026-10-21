import { useState } from 'react';
import { Alert, App, Button, Modal, Table, Upload } from 'antd';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api, apiScope, command } from '../../shared/api/client';
import { downloadCsv } from '../../shared/accounting/csv';
import { formatMoney } from '../../shared/accounting/money';
import { totals } from '../../shared/accounting/ledger';
import { workspaceKey } from '../../shared/data/workspace';
import type { AccountingDocument } from '../../shared/data/types';
import styles from '../../app/App.module.css';

interface Preview { documents: AccountingDocument[]; revision: number }
const headers = ['sourceType', 'sourceId', 'type', 'date', 'description', 'partnerId', 'accountId', 'linePartnerId', 'lineDescription', 'debit', 'credit'];

export function ImportDocuments() {
  const [open, setOpen] = useState(false);
  const [csv, setCsv] = useState('');
  const [preview, setPreview] = useState<Preview | null>(null);
  const client = useQueryClient();
  const { message } = App.useApp();
  const check = useMutation({ mutationFn: (text: string) => api<Preview>('/imports/preview', { method: 'POST', body: JSON.stringify({ csv: text }), headers: { 'X-Dataset-Epoch': apiScope().epoch } }), retry: false, onSuccess: (value, text) => { setCsv(text); setPreview(value); }, onError: error => { setPreview(null); void message.error(error.message); } });
  const save = useMutation({ mutationFn: () => command<Preview>('/imports', { csv, expectedRevision: preview!.revision }), retry: false, onSuccess: async result => { setOpen(false); setPreview(null); setCsv(''); void message.success(`Đã nhập ${result.documents.length} chứng từ nháp. Chưa tác động đến sổ.`); try { await client.invalidateQueries({ queryKey: workspaceKey() }); } catch { void message.warning('Đã nhập thành công; dữ liệu hiển thị chưa tải lại được.'); } }, onError: error => void message.error(error.message) });
  return <><Button onClick={() => setOpen(true)}>Nhập chứng từ CSV</Button><Modal open={open} onCancel={() => setOpen(false)} title="Kiểm tra và nhập chứng từ nháp" width={900} footer={<><Button onClick={() => setOpen(false)}>Đóng</Button><Button type="primary" disabled={!preview} loading={save.isPending} onClick={() => save.mutate()}>Nhập các nháp đã kiểm tra</Button></>}>
    <Alert showIcon type="info" title="Toàn bộ gói được xác thực trước khi ghi nhận" description="Dùng mã nguồn thống nhất để chống trùng; tiền là chuỗi số nguyên VND, ngày YYYY-MM-DD. Không chạy công thức hoặc macro. Mỗi gói tối đa 500 chứng từ / 5.000 dòng và 1 MB." />
    <div className={styles.recoveryActions}><Button onClick={() => downloadCsv('mau-nhap-chung-tu.csv', [headers])}>Tải mẫu tiêu đề CSV</Button><Upload accept=".csv" showUploadList={false} beforeUpload={async file => { setPreview(null); if (file.size > 1_000_000) { void message.error('File nhập tối đa 1 MB.'); return Upload.LIST_IGNORE; } check.mutate(await file.text()); return Upload.LIST_IGNORE; }}><Button loading={check.isPending}>Chọn CSV và kiểm tra</Button></Upload></div>
    <p className={styles.muted}>Các dòng cùng sourceType/sourceId là một chứng từ. sourceType phải khớp type; manual được quy về type để chống trùng với nhập tay. type nhận journal, receipt, payment, sale hoặc purchase; accountId và partnerId lấy từ danh mục. Để trống số tiền bên không phát sinh bằng chuỗi 0. Nghiệp vụ kho/tài sản cần lập qua phân hệ tự động.</p>
    {preview && <><p><strong>{preview.documents.length} chứng từ hợp lệ</strong> · Phiên bản dữ liệu {preview.revision}</p><Table rowKey="id" dataSource={preview.documents} pagination={{ pageSize: 8 }} columns={[{ title: 'NGUỒN', dataIndex: 'sourceId' }, { title: 'NGÀY', dataIndex: 'date' }, { title: 'DIỄN GIẢI', dataIndex: 'description' }, { title: 'DÒNG', render: (_, doc) => doc.lines.length }, { title: 'TỔNG NỢ', align: 'right', render: (_, doc) => formatMoney(totals(doc.lines).debit) }, { title: 'TỔNG CÓ', align: 'right', render: (_, doc) => formatMoney(totals(doc.lines).credit) }]} /></>}
  </Modal></>;
}
