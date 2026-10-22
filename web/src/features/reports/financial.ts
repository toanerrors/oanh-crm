export interface FinancialRow { code: string; name: string; level: number; formula: string[] }
export interface FinancialSheet { id: string; name: string; rows: FinancialRow[] }
export interface FinancialTemplate { version: string; regime: string; effectiveFrom: string; source: string; sheets: FinancialSheet[] }
export interface FinancialValue { current: string; previous: string; note: string }
export interface FinancialInput {
  id: string; version: number; template: string; from: string; to: string; previousFrom: string; previousTo: string;
  preparedBy: string; chiefAccountant: string; director: string; preparedDate: string; address: string;
  sheets: string[]; values: Record<string, Record<string, FinancialValue>>; notes: string; sourceRevision: number;
}
export interface FinancialReport { input: FinancialInput; computed: Record<string, Record<string, FinancialValue>>; issues: { sheet: string; code: string; message: string }[]; updatedAt: string; companyName: string }
export function financialPreview(input: Pick<FinancialInput, 'values'>, sheet: FinancialSheet): Record<string, FinancialValue> {
  const values: Record<string, FinancialValue> = {};
  const compute = (code: string): FinancialValue => {
    if (values[code]) return values[code];
    const row = sheet.rows.find(item => item.code === code)!;
    const value = input.values?.[sheet.id]?.[code] ?? { current: '', previous: '', note: '' };
    if (!row.formula.length) { values[code] = value; return value; }
    const sum = (field: 'current' | 'previous') => {
      let total = 0n;
      for (const term of row.formula) {
        const text = compute(term.replace(/^-/, ''))[field];
        if (!text || !/^-?(0|[1-9]\d{0,29})$/.test(text)) return '';
        total += BigInt(text) * (term.startsWith('-') ? -1n : 1n);
      }
      return total.toString();
    };
    return values[code] = { current: sum('current'), previous: sum('previous'), note: '' };
  };
  sheet.rows.forEach(row => compute(row.code));
  return values;
}

export function formatFinancialAmount(value: string): string {
  const [whole, fraction] = value.split('.');
  const negative = whole === '-0';
  const integer = whole === '-0' ? '0' : whole;
  const formatted = integer.replace(/\B(?=(\d{3})+(?!\d))/g, '.').replace(/^-/, '−');
  return `${negative ? '−' : ''}${formatted}${fraction ? `,${fraction}` : ''}`;
}
