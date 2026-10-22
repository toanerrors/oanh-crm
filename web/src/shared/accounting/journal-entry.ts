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
