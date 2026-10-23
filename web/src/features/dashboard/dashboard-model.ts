import { accountBalance, partnerBalances, periodDocuments, periodMovement, totals } from '../../shared/accounting/ledger.ts';
import { chartRatio } from '../../shared/accounting/money.ts';
import type { Workspace } from '../../shared/data/types.ts';

const opening = new Set(['opening', 'opening_adjustment', 'opening_adjustment_reversal']);
const optionalMoney = (value: string | undefined) => value === undefined ? null : BigInt(value);
export function dashboardModel(data: Workspace, period: string) {
  const documents = (data.server ? data.documents.filter(d => d.date.startsWith(`${period}-`)) : periodDocuments(data.documents, period)).filter(d => !opening.has(d.sourceType ?? ''));
  const drafts = documents.filter(d => d.status === 'draft');
  const balance = (group: 'cash' | 'inventory') => accountBalance(data.documents, data.accounts.filter(a => a.group === group).map(a => a.id), period);
  const revenue = data.server ? BigInt(data.server.revenue) : periodMovement(data.documents, data.accounts, period, 'revenue');
  const expense = data.server ? BigInt(data.server.expense) : periodMovement(data.documents, data.accounts, period, 'expense');
  const months = data.server ? data.server.months.map(m => ({ ...m, revenue: BigInt(m.revenue), expense: BigInt(m.expense) })) : Array.from({ length: 6 }, (_, i) => {
    const date = new Date(Date.UTC(Number(period.slice(0, 4)), Number(period.slice(5)) - 6 + i, 1));
    const key = date.toISOString().slice(0, 7);
    return { key, revenue: periodMovement(data.documents, data.accounts, key, 'revenue'), expense: periodMovement(data.documents, data.accounts, key, 'expense') };
  });
  return {
    documents, months, revenue, expense, difference: revenue - expense,
    money: data.server ? optionalMoney(data.server.moneyBalance) : balance('cash'),
    inventory: data.server ? optionalMoney(data.server.inventoryBalance) : balance('inventory'),
    bank: data.server ? optionalMoney(data.server.bankBalance) : accountBalance(data.documents, ['112', '1121'], period),
    receivable: data.server ? BigInt(data.server.receivable) : partnerBalances(data, period, 'receivable').reduce((s, p) => s + (p.balance > 0n ? p.balance : 0n), 0n),
    payable: data.server ? optionalMoney(data.server.payable) : partnerBalances(data, period, 'payable').reduce((s, p) => s + (p.balance < 0n ? -p.balance : 0n), 0n),
    count: data.server?.documentCount ?? documents.length,
    posted: data.server?.postedCount ?? documents.filter(d => d.status === 'posted').length,
    draftCount: data.server?.draftCount ?? drafts.length,
    unbalanced: data.server ? data.server.unbalancedDraftCount ?? null : drafts.filter(d => { const t = totals(d.lines); return t.debit !== t.credit; }).length,
  };
}
export function chartDomain(values: bigint[]) {
  let positive = 0n, negative = 0n;
  for (const value of values) { if (value > positive) positive = value; if (-value > negative) negative = -value; }
  const span = positive + negative || 1n;
  return { positive, negative, span, zero: 8 + chartRatio(positive, span) * 120, empty: positive === 0n && negative === 0n };
}
export function chartBar(value: bigint, domain: ReturnType<typeof chartDomain>) {
  const height = chartRatio(value < 0n ? -value : value, domain.span) * 120;
  return { y: value < 0n ? domain.zero : domain.zero - height, height };
}
