import { normalizeSearch } from '../accounting/money';

export const searchOptions = {
  filterOption: (input: string, option?: { label?: unknown; value?: unknown }) =>
    normalizeSearch(`${option?.label ?? ''} ${option?.value ?? ''}`).includes(normalizeSearch(input)),
};

export function suggestionOptions(values: string[]) {
  return [...new Set(values.filter(value => value.trim()))].map(value => ({ value, label: value }));
}
