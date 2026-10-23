import { useState } from 'react';
import { Alert, Button, Modal, Space, Table, Tag, Typography } from 'antd';
import { useQuery } from '@tanstack/react-query';
import { useWorkspace } from '../../app/WorkspaceProvider';
import { api } from '../../shared/api/client';
import { workspaceKey } from '../../shared/data/workspace';
import { formatDate, formatMoney } from '../../shared/accounting/money';
import { openingTotals } from '../../shared/accounting/opening';
import type { AccountingDocument } from '../../shared/data/types';
import type { OpeningInput } from './OpeningBalances';
import styles from './OpeningBalances.module.css';

export interface OpeningAdjustment {
  input: OpeningInput; before: OpeningInput; baseVersion: number; reason: string;
  document: AccountingDocument; revision: number; confirmedVersion: number | null;
  actor: string; createdAt: string; confirmedAt: string;
  blockers: { code: string; message: string; documentId?: string }[];
}
export function OpeningReview({ value, onClose, onConfirm, onReload }: { value: OpeningAdjustment; onClose: () => void; onConfirm?: () => Promise<void>; onReload?: () => Promise<void> }) {
  const { data, openDocument } = useWorkspace();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const before = openingTotals(value.before), after = openingTotals(value.input);
  const act = async (action: () => Promise<void>) => { setBusy(true); setError(''); try { await action(); } catch (e) { setError((e as Error).message); } finally { setBusy(false); } };
  const snapshot = (input: OpeningInput, title: string) => <div className={styles.snapshot}>
    <Typography.Title level={5}>{title}</Typography.Title>
    <Table size="small" rowKey="key" pagination={false} scroll={{ x: 420, y: 360 }} dataSource={[
      ...input.lines.map((l, i) => { const a = data?.accounts.find(a => a.id === l.accountId), p = data?.partners.find(p => p.id === l.partnerId); return { key: `l-${i}`, name: `${a?.code ?? l.accountId} · ${a?.name ?? ''}`, detail: [p ? `${p.code} · ${p.name}` : '', l.bankAccountId ? `Ngân hàng: ${l.bankAccountId}` : ''].filter(Boolean).join(' · '), debit: l.debit, credit: l.credit }; }),
      ...input.stocks.map((s, i) => ({ key: `s-${i}`, name: `Tồn kho · ${s.itemId}`, detail: `Kho ${s.warehouseId} · Số lượng ${s.quantity}`, debit: s.value, credit: '0' })),
      ...input.assets.map((a, i) => ({ key: `a-${i}`, name: `${a.code} · ${a.name}`, detail: `${a.kind === 'fixed' ? 'TSCĐ' : 'Trả trước'} · ${a.startPeriod} · ${a.months} tháng · Thu hồi ${formatMoney(a.residual)}`, debit: a.cost, credit: a.allocated })),
    ]} columns={[{ title: 'Tài khoản / chi tiết', dataIndex: 'name', render: (name: string, row: { detail: string }) => <>{name}{row.detail && <div><Typography.Text type="secondary">{row.detail}</Typography.Text></div>}</> }, { title: 'Nợ / nguyên giá', dataIndex: 'debit', align: 'right', render: (v: string) => formatMoney(v) }, { title: 'Có / lũy kế', dataIndex: 'credit', align: 'right', render: (v: string) => formatMoney(v) }]} />
  </div>;
  return <Modal open title={value.confirmedVersion === null ? 'Đối chiếu điều chỉnh số dư' : `Lịch sử số dư · Phiên bản ${value.confirmedVersion}`} width={1200} onCancel={onClose} maskClosable={false} footer={<Space wrap><Button onClick={onClose}>Quay lại</Button>{onReload && <Button disabled={busy} onClick={() => void act(onReload)}>Đối chiếu lại</Button>}{onConfirm && <Button type="primary" loading={busy} disabled={value.blockers.length > 0} onClick={() => void act(onConfirm)}>Xác nhận điều chỉnh</Button>}</Space>}>
    <Alert showIcon type="info" title={`${formatDate(value.input.cutoff)} · ${value.reason}`} description={value.confirmedVersion === null ? 'Xác nhận sẽ thay phiên bản số dư đang dùng; không làm tăng phát sinh trong kỳ. Bản trước và các chứng từ điều chỉnh được lưu đầy đủ.' : `${value.actor}${value.confirmedAt ? ` · ${new Date(value.confirmedAt).toLocaleString('vi-VN', { timeZone: 'Asia/Ho_Chi_Minh' })}` : ''}`} />
    <div className={styles.totals}><span>Tổng trước<strong>{before ? formatMoney(before.debit) : '—'}</strong></span><span>Tổng sau<strong>{after ? formatMoney(after.debit) : '—'}</strong></span><span>Chênh lệch<strong>{before && after ? formatMoney(after.debit - before.debit) : '—'}</strong></span><Tag color={after?.balanced ? 'green' : 'gold'}>{after?.balanced ? 'Cân đối' : 'Chưa cân đối'}</Tag></div>
    {value.blockers.map((b, i) => <Alert key={i} className={styles.error} type="warning" showIcon title={b.message} action={b.documentId ? <Button onClick={() => openDocument(b.documentId!)}>Mở chứng từ liên quan</Button> : undefined} />)}
    {error && <Alert className={styles.error} type="error" showIcon title={error} />}
    <div className={styles.comparison}>{snapshot(value.before, 'Trước điều chỉnh')}{snapshot(value.input, 'Sau điều chỉnh')}</div>
  </Modal>;
}
interface HistoryRow { id: string; version: number; reference: string; reason: string; at: string; actor: string }
export function OpeningHistory({ rootId, onClose }: { rootId: string; onClose: () => void }) {
  const [page, setPage] = useState(1);
  const [selected, setSelected] = useState('');
  const { data } = useWorkspace();
  const history = useQuery({ queryKey: [...workspaceKey(), 'opening-history', data?.revision, page], queryFn: ({ signal }) => api<{ items: HistoryRow[]; total: number }>(`/opening-balances/adjustments?page=${page}`, { signal }), retry: false });
  const detail = useQuery({ queryKey: [...workspaceKey(), 'opening-adjustment', selected], queryFn: ({ signal }) => api<OpeningAdjustment>(`/opening-balances/adjustments/${selected}`, { signal }), enabled: !!selected, retry: false });
  return <><Modal open title="Lịch sử số dư đầu kỳ" width={1000} onCancel={onClose} footer={<Button onClick={onClose}>Đóng</Button>}>
    <Button onClick={() => setSelected(rootId)}>Xem số dư ban đầu · Phiên bản 0</Button>
    {(history.error || detail.error) && <Alert type="error" title={(history.error || detail.error)?.message} />}
    <Table loading={history.isFetching || detail.isFetching} rowKey="id" dataSource={history.data?.items ?? []} pagination={{ current: page, pageSize: 20, total: history.data?.total ?? 0, onChange: setPage, showSizeChanger: false }} columns={[
      { title: 'Phiên bản', dataIndex: 'version', width: 95 }, { title: 'Chứng từ', dataIndex: 'reference', render: (v: string, r: HistoryRow) => <Button type="link" onClick={() => setSelected(r.id)}>{v}</Button> }, { title: 'Lý do', dataIndex: 'reason' }, { title: 'Người xác nhận', dataIndex: 'actor' }, { title: 'Thời gian', dataIndex: 'at', render: (v: string) => new Date(v).toLocaleString('vi-VN', { timeZone: 'Asia/Ho_Chi_Minh' }) },
    ]} />
  </Modal>{selected && detail.data && <OpeningReview value={detail.data} onClose={() => setSelected('')} />}</>;
}
