import { InputNumber } from 'antd';
import { formatMoney, isMoney, MAX_MONEY, parseMoneyInput } from '../accounting/money';

export function MoneyInput({ value, onChange, label }: { value?: string; onChange?: (value: string | undefined) => void; label: string }) {
  return <InputNumber<string>
    aria-label={label}
    stringMode
    value={value}
    min="0"
    max={MAX_MONEY.toString()}
    precision={0}
    controls={false}
    keyboard={false}
    changeOnWheel={false}
    changeOnBlur={false}
    inputMode="numeric"
    step="1"
    placeholder="0"
    style={{ width: '100%' }}
    formatter={(text, info) => {
      if (info.userTyping) return info.input;
      const current = value ?? String(text ?? '');
      return isMoney(current) ? formatMoney(current) : current;
    }}
    parser={(text) => parseMoneyInput(text ?? '') ?? ''}
    onInput={(text) => onChange?.(parseMoneyInput(text))}
  />;
}
