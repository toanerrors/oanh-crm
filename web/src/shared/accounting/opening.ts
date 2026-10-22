import { isMoney } from './money.ts';

export function openingTotals(input: { lines: { debit?: string; credit?: string }[]; stocks: { value?: string }[]; assets: { kind: string; cost?: string; allocated?: string }[] }) {
  let debit = 0n; let credit = 0n;
  for (const line of input.lines) {
    if (!isMoney(line.debit ?? '0') || !isMoney(line.credit ?? '0')) return null;
    debit += BigInt(line.debit || '0'); credit += BigInt(line.credit || '0');
  }
  for (const stock of input.stocks) { if (!isMoney(stock.value ?? '0')) return null; debit += BigInt(stock.value || '0'); }
  for (const asset of input.assets) {
    if (!isMoney(asset.cost ?? '0') || !isMoney(asset.allocated ?? '0')) return null;
    const cost = BigInt(asset.cost || '0'); const allocated = BigInt(asset.allocated || '0');
    if (allocated > cost) return null;
    debit += asset.kind === 'fixed' ? cost : cost - allocated;
    if (asset.kind === 'fixed') credit += allocated;
  }
  return { debit, credit, difference: debit - credit, balanced: debit > 0n && debit === credit };
}
