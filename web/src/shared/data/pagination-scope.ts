export function samePaginationScope(current: readonly unknown[], previous?: readonly unknown[]) {
  if (!previous || current.length !== previous.length || current.length < 2) return false;
  if (!current.slice(0, -1).every((value, index) => value === previous[index])) return false;
  const normalize = (value: unknown) => {
    if (typeof value !== 'string') return null;
    const params = new URLSearchParams(value);
    params.delete('page'); params.sort();
    return params.toString();
  };
  const scope = normalize(current.at(-1));
  return scope !== null && scope === normalize(previous.at(-1));
}
