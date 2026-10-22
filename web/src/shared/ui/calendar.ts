import dayjs from 'dayjs';
import type { Dayjs } from 'dayjs';
import customParseFormat from 'dayjs/plugin/customParseFormat.js';
import 'dayjs/locale/vi.js';

dayjs.extend(customParseFormat);
dayjs.locale('vi');

export type CalendarPicker = 'date' | 'month';
export const calendarFormats = { date: ['DD/MM/YYYY', 'YYYY-MM-DD'], month: ['MM/YYYY', 'YYYY-MM'] };

export function parseCalendarValue(value: string | undefined, picker: CalendarPicker = 'date'): Dayjs | null {
  if (!value) return null;
  const parsed = dayjs(value, calendarFormats[picker][1], true);
  return parsed.isValid() ? parsed : null;
}

export function serializeCalendarValue(value: Dayjs | null, picker: CalendarPicker = 'date'): string {
  return value?.isValid() ? value.format(calendarFormats[picker][1]) : '';
}
