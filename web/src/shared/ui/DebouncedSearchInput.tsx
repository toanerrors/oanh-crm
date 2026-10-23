import { useEffect, useRef, useState } from 'react';
import { Input } from 'antd';
import type { InputProps } from 'antd';

type Props = Omit<InputProps, 'value' | 'onChange' | 'onPressEnter' | 'onCompositionStart' | 'onCompositionEnd'> & { value: string; scopeKey: string; onSearch: (value: string) => void };

export function DebouncedSearchInput({ value, scopeKey, onSearch, ...props }: Props) {
  const [text, setText] = useState(value);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const composing = useRef(false);
  const commit = useRef(onSearch);
  commit.current = onSearch;
  const cancel = () => { clearTimeout(timer.current); timer.current = undefined; };
  useEffect(() => { cancel(); setText(value); }, [value, scopeKey]);
  useEffect(() => cancel, []);
  function flush(next: string) { cancel(); if (next !== value) commit.current(next); }
  function change(next: string) {
    setText(next); cancel();
    if (composing.current) return;
    if (!next) flush(next);
    else timer.current = setTimeout(() => flush(next), 250);
  }
  return <Input {...props} value={text} onChange={event => change(event.target.value)} onPressEnter={event => { if (!event.nativeEvent.isComposing && !composing.current) flush(text); }} onBlur={event => { if (!composing.current) flush(text); props.onBlur?.(event); }} onCompositionStart={() => { composing.current = true; cancel(); }} onCompositionEnd={event => { composing.current = false; change(event.currentTarget.value); }} />;
}
