import assert from 'node:assert/strict';
import test from 'node:test';
import { calendarFormats, parseCalendarValue, serializeCalendarValue } from '../src/shared/ui/calendar.ts';

test('ngày và tháng hiển thị Việt Nam, giữ chuỗi canonical khi đổi giá trị', () => {
  const date = parseCalendarValue('2024-02-29');
  assert.equal(date?.format(calendarFormats.date[0]), '29/02/2024');
  assert.equal(serializeCalendarValue(date), '2024-02-29');
  const month = parseCalendarValue('2026-12', 'month');
  assert.equal(month?.format(calendarFormats.month[0]), '12/2026');
  assert.equal(serializeCalendarValue(month, 'month'), '2026-12');
  assert.equal(date?.locale(), 'vi');
});

test('không tự đổi ngày sai sang ngày hoặc tháng khác; ô trống vẫn rỗng', () => {
  for (const value of ['', undefined, '2026-02-29', '2026-04-31', '2026-13-01', '2026-1-01', '22/10/2026', '2026-10-22T00:00:00Z']) assert.equal(parseCalendarValue(value), null, String(value));
  for (const value of ['', undefined, '2026-00', '2026-13', '2026-1', '10/2026', '2026-10-01']) assert.equal(parseCalendarValue(value, 'month'), null, String(value));
  assert.equal(serializeCalendarValue(null), '');
  assert.equal(serializeCalendarValue(null, 'month'), '');
});

test('ngày lịch và kỳ không lệch khi timezone nằm hai phía UTC', () => {
  const previous = process.env.TZ;
  try {
    for (const timezone of ['Asia/Ho_Chi_Minh', 'Pacific/Honolulu', 'Pacific/Kiritimati']) {
      process.env.TZ = timezone;
      for (const value of ['2026-01-01', '2026-12-31', '2024-02-29']) assert.equal(serializeCalendarValue(parseCalendarValue(value)), value, timezone);
      assert.equal(serializeCalendarValue(parseCalendarValue('2026-01', 'month'), 'month'), '2026-01', timezone);
    }
  } finally {
    if (previous === undefined) delete process.env.TZ;
    else process.env.TZ = previous;
  }
});
