import { formatDate, isCalendarDate } from './money.ts';

export function daysInMonth(year: number, month: number): number {
  if (month === 2) return year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0) ? 29 : 28;
  return [4, 6, 9, 11].includes(month) ? 30 : 31;
}

export function monthDateRange(period: string): { from: string; to: string } {
  const year = Number(period.slice(0, 4));
  const month = Number(period.slice(5, 7));
  const lastDay = String(daysInMonth(year, month)).padStart(2, '0');
  return { from: `${period}-01`, to: `${period}-${lastDay}` };
}

export function isFullMonthRange(from: string, to: string): string | null {
  if (!isCalendarDate(from) || !isCalendarDate(to) || from.slice(0, 7) !== to.slice(0, 7)) return null;
  const ym = from.slice(0, 7);
  const expected = monthDateRange(ym);
  return from === expected.from && to === expected.to ? ym : null;
}

export function normalizeDateRange(from: string, to: string, changed: 'from' | 'to' = 'to'): { from: string; to: string } {
  if (changed === 'from' && from > to) return { from, to: from };
  if (changed === 'to' && to < from) return { from: to, to };
  return { from, to };
}

export function clampRangeFromConversion(from: string, to: string, conversionDate?: string): { from: string; to: string } {
  if (conversionDate && isCalendarDate(conversionDate) && from < conversionDate) {
    return { from: conversionDate, to: to < conversionDate ? conversionDate : to };
  }
  return { from, to };
}

export function periodPresetOptions(anchorYear: number, demo: boolean): { value: string; label: string }[] {
  const presets = [
    { value: 'custom', label: 'Tùy chọn ngày' },
    { value: 'ytd', label: 'Đầu năm đến nay' },
    { value: `year:${anchorYear}`, label: `Cả năm ${anchorYear}` },
    { value: `q1:${anchorYear}`, label: `Quý 1, ${anchorYear}` },
    { value: `q2:${anchorYear}`, label: `Quý 2, ${anchorYear}` },
    { value: `q3:${anchorYear}`, label: `Quý 3, ${anchorYear}` },
    { value: `q4:${anchorYear}`, label: `Quý 4, ${anchorYear}` },
  ];
  const months = Array.from({ length: demo ? 12 : 84 }, (_, index) => {
    const year = anchorYear + (demo ? 0 : Math.floor(index / 12) - 3);
    const month = String((index % 12) + 1).padStart(2, '0');
    return { value: `${year}-${month}`, label: `Tháng ${month}, ${year}` };
  });
  return [...presets, ...months];
}

export interface PresetRangeContext {
  today: string;
  demo?: boolean;
  conversionDate?: string;
}

export function resolvePresetRange(preset: string, context: PresetRangeContext): { from: string; to: string } | null {
  if (preset === 'custom') return null;
  if (preset === 'ytd') {
    const effectiveToday = context.demo ? '2026-10-31' : context.today;
    return clampRangeFromConversion(`${effectiveToday.slice(0, 4)}-01-01`, effectiveToday, context.conversionDate);
  }
  if (/^year:\d{4}$/.test(preset)) {
    const year = preset.slice(5);
    return clampRangeFromConversion(`${year}-01-01`, `${year}-12-31`, context.conversionDate);
  }
  if (/^q[1-4]:\d{4}$/.test(preset)) {
    const quarter = Number(preset[1]);
    const year = preset.slice(3);
    const bounds: Record<number, [string, string]> = {
      1: [`${year}-01-01`, `${year}-03-31`],
      2: [`${year}-04-01`, `${year}-06-30`],
      3: [`${year}-07-01`, `${year}-09-30`],
      4: [`${year}-10-01`, `${year}-12-31`],
    };
    const [from, to] = bounds[quarter];
    return clampRangeFromConversion(from, to, context.conversionDate);
  }
  if (/^\d{4}-(0[1-9]|1[0-2])$/.test(preset)) {
    return monthDateRange(preset);
  }
  return null;
}

export function activePeriodPreset(
  from: string,
  to: string,
  preferredPreset: string | null,
  context: PresetRangeContext,
): string {
  if (preferredPreset && preferredPreset !== 'custom') {
    const resolved = resolvePresetRange(preferredPreset, context);
    if (resolved && resolved.from === from && resolved.to === to) return preferredPreset;
  }
  const ym = isFullMonthRange(from, to);
  if (ym) return ym;
  const year = to.slice(0, 4);
  for (const candidate of ['ytd', `year:${year}`, `q1:${year}`, `q2:${year}`, `q3:${year}`, `q4:${year}`]) {
    const resolved = resolvePresetRange(candidate, context);
    if (resolved && resolved.from === from && resolved.to === to) return candidate;
  }
  return 'custom';
}

export function formatAccountingRange(from: string, to: string): string {
  const ym = isFullMonthRange(from, to);
  if (ym) return `Kỳ ${ym.slice(5, 7)}/${ym.slice(0, 4)}`;
  return `Từ ${formatDate(from)} đến ${formatDate(to)}`;
}

export function accountingRangeSlug(from: string, to: string): string {
  return isFullMonthRange(from, to) ?? `${from}_${to}`;
}
