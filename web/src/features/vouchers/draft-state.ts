import type { OperationInput } from '../../shared/data/operations';

export interface VoucherDraft {
  input: OperationInput;
  allocate: boolean;
  route: string;
  returnTo: string;
}
export type VoucherDrafts = Record<string, VoucherDraft>;
const kinds = ['receipt', 'payment', 'transfer', 'sale', 'purchase', 'closing'] as const;
export type VoucherKind = typeof kinds[number];

export function voucherKind(params: URLSearchParams): VoucherKind {
  const kind = params.get('kind');
  return kinds.find(value => value === kind) ?? 'receipt';
}
export function voucherDraftKey(id: string | undefined, params: URLSearchParams): string {
  if (id) return `document:${id}`;
  const kind = voucherKind(params);
  const trade = kind === 'sale' || kind === 'purchase';
  const treasury = kind === 'receipt' || kind === 'payment' || kind === 'transfer';
  return JSON.stringify(['new', kind, trade ? params.get('service') === '1' : false, treasury ? params.get('account') === '1121' ? 'bank' : 'cash' : '', (kind === 'receipt' || kind === 'payment') && params.get('settlement') === '1', params.get('replacement') ?? '']);
}
export function updateVoucherDraft(drafts: VoucherDrafts, key: string, draft: VoucherDraft | null): VoucherDrafts {
  const next = { ...drafts };
  if (draft) next[key] = structuredClone(draft);
  else delete next[key];
  return next;
}
export function voucherDraftModule(draft: VoucherDraft): string {
  const { input } = draft;
  if (input.kind === 'sale') return '/sales';
  if (input.kind === 'purchase') return '/purchases';
  if (input.kind === 'closing') return '/documents';
  const bank = Boolean(input.moneyAccount && input.moneyAccount !== '111') || new URL(draft.route, 'https://oanh.invalid').searchParams.get('account') === '1121';
  return bank ? '/bank' : '/cash';
}
export function voucherDraftLabel(draft: VoucherDraft): string {
  const { input } = draft;
  const bank = voucherDraftModule(draft) === '/bank';
  const labels: Partial<Record<OperationInput['kind'], string>> = { sale: 'Bán hàng', purchase: 'Mua hàng', transfer: 'Chuyển tiền nội bộ', closing: 'Kết chuyển lãi lỗ', receipt: 'Phiếu thu', payment: 'Phiếu chi' };
  let label = labels[input.kind] ?? 'Chứng từ';
  if (input.tradeMode === 'service' && (input.kind === 'sale' || input.kind === 'purchase')) label = input.kind === 'sale' ? 'Bán dịch vụ' : 'Mua dịch vụ';
  if (bank && (input.kind === 'receipt' || input.kind === 'payment')) label = input.kind === 'receipt' ? 'Thu tiền gửi' : 'Chi tiền gửi';
  return `${label}${draft.allocate ? ' theo chứng từ' : ''}${input.replacementOf ? ' · Bản sửa thay thế' : ''}${input.description ? ` · ${input.description}` : ''}`;
}
