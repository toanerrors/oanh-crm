export const MAX_MONEY = 9223372036854775807n;

export function isMoney(value: unknown): value is string {
  return typeof value === 'string' && value.length <= 19 && /^(0|[1-9]\d*)$/.test(value) && BigInt(value) <= MAX_MONEY;
}

export function parseMoneyInput(value: string): string | undefined {
  if (value === '') return undefined;
  if (/^\d+$/.test(value)) return value.replace(/^0+(?=\d)/, '');
  if (/^\d{1,3}(?:\.\d{3})+$/.test(value)) return value.replace(/\./g, '').replace(/^0+(?=\d)/, '');
  return value;
}

export function formatMoney(value: string | bigint): string {
  const text = typeof value === 'bigint' ? value.toString() : value;
  if (!/^-?(0|[1-9]\d*)$/.test(text)) throw new Error('Số tiền không hợp lệ');
  const negative = text.startsWith('-');
  return `${negative ? '−' : ''}${(negative ? text.slice(1) : text).replace(/\B(?=(\d{3})+(?!\d))/g, '.')}`;
}

export function compactMoney(value: bigint): { amount: string; unit: string } {
  const absolute = value < 0n ? -value : value;
  const divisor = absolute >= 1_000_000_000n ? 1_000_000_000n : absolute >= 1_000_000n ? 1_000_000n : 1n;
  if (divisor === 1n) return { amount: formatMoney(value), unit: 'đ' };
  const fraction = (absolute % divisor).toString().padStart(divisor.toString().length - 1, '0').replace(/0+$/, '');
  return {
    amount: `${value < 0n ? '−' : ''}${formatMoney(absolute / divisor)}${fraction ? `,${fraction}` : ''}`,
    unit: divisor === 1_000_000_000n ? 'tỷ' : 'triệu',
  };
}

export function normalizeSearch(value: string): string {
  return value.normalize('NFD').replace(/[\u0300-\u036f]/g, '').replace(/đ/g, 'd').replace(/Đ/g, 'D').toLowerCase().trim();
}

export function formatDate(value: string): string {
  return value.split('-').reverse().join('/');
}

export function businessToday(): string {
  const parts = new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Ho_Chi_Minh', year: 'numeric', month: '2-digit', day: '2-digit' }).formatToParts(new Date());
  const get = (type: string) => parts.find(part => part.type === type)!.value;
  return `${get('year')}-${get('month')}-${get('day')}`;
}

export function formatQuantity(value: string, scale: number): string {
  if (!/^(0|[1-9]\d*)$/.test(value) || scale < 0 || scale > 6) throw new Error('Số lượng không hợp lệ');
  if (scale === 0) return formatMoney(value);
  const text = value.padStart(scale + 1, '0');
  const whole = text.slice(0, -scale).replace(/^0+(?=\d)/, '');
  const fraction = text.slice(-scale).replace(/0+$/, '');
  return `${formatMoney(whole)}${fraction ? `,${fraction}` : ''}`;
}

export function isCalendarDate(value: string): boolean {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return false;
  const parsed = new Date(`${value}T00:00:00Z`);
  return !Number.isNaN(parsed.getTime()) && parsed.toISOString().slice(0, 10) === value;
}

// Chỉ chuyển tỷ lệ đã giới hạn sang number để vẽ tọa độ, không chuyển số tiền.
export function chartRatio(value: bigint, maximum: bigint): number {
  if (maximum <= 0n || value <= 0n) return 0;
  return Number((value > maximum ? maximum : value) * 1000n / maximum) / 1000;
}
