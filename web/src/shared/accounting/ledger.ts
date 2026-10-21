import type { Account, AccountingDocument, JournalLine, Workspace } from '../data/types.ts';
import { isCalendarDate, isMoney, MAX_MONEY, normalizeSearch } from './money.ts';

export function totals(lines: JournalLine[]): { debit: bigint; credit: bigint } {
  return lines.reduce((sum, line) => ({ debit: sum.debit + BigInt(line.debit), credit: sum.credit + BigInt(line.credit) }), { debit: 0n, credit: 0n });
}

export function validateDraft(document: AccountingDocument, workspace: Workspace): string[] {
  const errors: string[] = [];
  if (!document.description.trim()) errors.push('Nhập diễn giải chứng từ.');
  if (!isCalendarDate(document.date)) errors.push('Ngày chứng từ không hợp lệ.');
  if (!document.lines.length) errors.push('Thêm ít nhất một dòng hạch toán.');
  document.lines.forEach((line, index) => {
    const account = workspace.accounts.find(item => item.id === line.accountId);
    if (!account) errors.push(`Dòng ${index + 1}: chọn tài khoản hợp lệ.`);
    if (!isMoney(line.debit) || !isMoney(line.credit)) {
      errors.push(`Dòng ${index + 1}: số tiền phải là số nguyên không âm trong giới hạn int64.`);
    } else if (BigInt(line.debit) > 0n && BigInt(line.credit) > 0n) {
      errors.push(`Dòng ${index + 1}: chỉ nhập một bên Nợ hoặc Có.`);
    } else if (line.debit === '0' && line.credit === '0') {
      errors.push(`Dòng ${index + 1}: nhập số tiền Nợ hoặc Có.`);
    }
    if ((account?.requiresPartner || line.partnerId) && !workspace.partners.some(partner => partner.id === line.partnerId && partner.active)) {
      errors.push(`Dòng ${index + 1}: chọn đối tượng đang sử dụng.`);
    }
  });
  if (document.partnerId && !workspace.partners.some(partner => partner.id === document.partnerId && partner.active)) errors.push('Đối tượng chứng từ không hợp lệ.');
  if (document.lines.every(line => isMoney(line.debit) && isMoney(line.credit))) {
    const total = totals(document.lines);
    if (total.debit > MAX_MONEY || total.credit > MAX_MONEY) errors.push('Tổng chứng từ vượt giới hạn int64.');
  }
  return errors;
}

export function periodDocuments(documents: AccountingDocument[], period: string): AccountingDocument[] {
  return documents.filter(item => item.date.startsWith(`${period}-`)).sort((a, b) => b.date.localeCompare(a.date) || a.id.localeCompare(b.id));
}

export function filterDocuments(documents: AccountingDocument[], query: string, partners: Workspace['partners']): AccountingDocument[] {
  const search = normalizeSearch(query);
  if (!search) return documents;
  return documents.filter(item => normalizeSearch(`${item.reference} ${item.description} ${partners.find(partner => partner.id === item.partnerId)?.name ?? ''}`).includes(search));
}

export function accountBalance(documents: AccountingDocument[], accountIds: string[], endPeriod: string): bigint {
  const ids = new Set(accountIds);
  return documents.filter(item => item.status === 'posted' && item.date.slice(0, 7) <= endPeriod)
    .flatMap(item => item.lines).filter(line => ids.has(line.accountId))
    .reduce((sum, line) => sum + BigInt(line.debit) - BigInt(line.credit), 0n);
}

export function periodMovement(documents: AccountingDocument[], accounts: Account[], period: string, group: Account['group']): bigint {
  const ids = new Set(accounts.filter(item => item.group === group).map(item => item.id));
  return periodDocuments(documents, period).filter(item => item.status === 'posted').flatMap(item => item.lines)
    .filter(line => ids.has(line.accountId)).reduce((sum, line) => sum + (group === 'revenue' ? BigInt(line.credit) - BigInt(line.debit) : BigInt(line.debit) - BigInt(line.credit)), 0n);
}

export function partnerBalances(workspace: Workspace, period: string, group: 'receivable' | 'payable') {
  const ids = new Set(workspace.accounts.filter(item => item.group === group).map(item => item.id));
  const lines = workspace.documents.filter(item => item.status === 'posted' && item.date.slice(0, 7) <= period).flatMap(item => item.lines).filter(line => ids.has(line.accountId));
  return workspace.partners.map(partner => ({
    ...partner,
    balance: lines.filter(line => line.partnerId === partner.id).reduce((sum, line) => sum + BigInt(line.debit) - BigInt(line.credit), 0n),
  }));
}

export function trialBalance(workspace: Workspace, period: string) {
  const posted = workspace.documents.filter(item => item.status === 'posted' && item.date.slice(0, 7) <= period);
  return workspace.accounts.map(account => {
    const balances = new Map<string, { opening: bigint; closing: bigint }>();
    let debit = 0n;
    let credit = 0n;
    for (const document of posted) {
      for (const line of document.lines.filter(item => item.accountId === account.id)) {
        const key = account.requiresPartner ? line.partnerId ?? '' : account.id;
        const balance = balances.get(key) ?? { opening: 0n, closing: 0n };
        const net = BigInt(line.debit) - BigInt(line.credit);
        balance.closing += net;
        if (document.date < `${period}-01`) balance.opening += net;
        else { debit += BigInt(line.debit); credit += BigInt(line.credit); }
        balances.set(key, balance);
      }
    }
    return [...balances.values()].reduce((row, balance) => ({
      ...row,
      openingDebit: row.openingDebit + (balance.opening > 0n ? balance.opening : 0n),
      openingCredit: row.openingCredit + (balance.opening < 0n ? -balance.opening : 0n),
      closingDebit: row.closingDebit + (balance.closing > 0n ? balance.closing : 0n),
      closingCredit: row.closingCredit + (balance.closing < 0n ? -balance.closing : 0n),
    }), { key: account.id, code: account.code, name: account.name, debit, credit, openingDebit: 0n, openingCredit: 0n, closingDebit: 0n, closingCredit: 0n });
  });
}
