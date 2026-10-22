import { isMoney } from './money.ts';
import type { JournalLine } from '../data/types.ts';

export type JournalSide = 'debit' | 'credit';

export function journalEntrySides(lines: JournalLine[]): JournalSide[] {
  return lines.map((line, index) => line.credit !== '' && line.credit !== '0' ? 'credit' : line.debit !== '' && line.debit !== '0' ? 'debit' : index % 2 === 0 ? 'debit' : 'credit');
}

export function journalEntryRows<T>(fields: T[], sides: JournalSide[]): { key: number; debit?: T; credit?: T }[] {
  const debit = fields.filter((_, index) => sides[index] === 'debit');
  const credit = fields.filter((_, index) => sides[index] === 'credit');
  return Array.from({ length: Math.max(debit.length, credit.length) }, (_, index) => ({ key: index, debit: debit[index], credit: credit[index] }));
}

export function canPairJournalLines(lines: JournalLine[], sides = journalEntrySides(lines)): boolean {
  if (!lines.length || sides.length !== lines.length) return false;
  return journalEntryRows(lines, sides).every(({ debit, credit }) => Boolean(debit && credit
    && (debit.credit || '0') === '0' && (credit.debit || '0') === '0'
    && isMoney(debit.debit || '0') && (debit.debit || '0') === (credit.credit || '0') && debit.description === credit.description));
}
