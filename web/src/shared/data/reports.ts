import type { AccountingDocument } from './types';

export interface ServerMetrics { revenue: string; expense: string; receivable: string; balances: Record<string, string>; documentCount: number; postedCount: number; draftCount: number; months: { key: string; revenue: string; expense: string }[] }
export interface DocumentList { documents: AccountingDocument[]; total: number; amount: string; counts: Record<'all' | 'posted' | 'draft', number>; revision: number }
export interface ServerReportRow { key: string; code: string; name: string; debit: string; credit: string; openingDebit: string; openingCredit: string; closingDebit: string; closingCredit: string; date?: string; documentId?: string; description?: string; account?: string }
export interface ServerReport { rows: ServerReportRow[]; total: number; debit: string; credit: string; openingDebit: string; openingCredit: string; closingDebit: string; closingCredit: string; accountBalance?: ServerReportRow; revision: number }
export const exactRow = (row: ServerReportRow) => ({ ...row, debit: BigInt(row.debit), credit: BigInt(row.credit), openingDebit: BigInt(row.openingDebit || '0'), openingCredit: BigInt(row.openingCredit || '0'), closingDebit: BigInt(row.closingDebit || '0'), closingCredit: BigInt(row.closingCredit || '0') });
