import { useState } from 'react';
import { DatePicker } from 'antd';
import type { DatePickerProps } from 'antd';
import type { Dayjs } from 'dayjs';
import { calendarFormats, parseCalendarValue, serializeCalendarValue } from './calendar';
import type { CalendarPicker } from './calendar';

type DateInputProps = Omit<DatePickerProps<Dayjs, false>, 'value' | 'defaultValue' | 'onChange' | 'picker' | 'format' | 'multiple' | 'showTime'> & {
  value?: string;
  defaultValue?: string;
  onChange?: (value: string) => void;
  picker?: CalendarPicker;
};

export function DateInput(props: DateInputProps) {
  const { value, defaultValue, onChange, picker = 'date', style, ...pickerProps } = props;
  const [internalValue, setInternalValue] = useState(defaultValue ?? '');
  const controlled = Object.hasOwn(props, 'value');
  return <DatePicker placeholder={picker === 'month' ? 'Chọn tháng' : 'Chọn ngày'} {...pickerProps} picker={picker} format={calendarFormats[picker]} style={{ width: '100%', ...style }}
    value={parseCalendarValue(controlled ? value : internalValue, picker)}
    onChange={date => {
      const next = serializeCalendarValue(date, picker);
      if (!controlled) setInternalValue(next);
      onChange?.(next);
    }} />;
}
