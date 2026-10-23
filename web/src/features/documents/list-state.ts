export function documentListState(params: URLSearchParams, demo: boolean) {
  const page = Number(params.get('page'));
  const size = Number(params.get('pageSize'));
  const type = params.get('type') ?? 'all';
  const status = params.get('status') ?? 'all';
  return {
    page: Number.isSafeInteger(page) && page > 0 ? page : 1,
    pageSize: [10, 20, 50, 100, ...(demo ? [8] : [])].includes(size) ? size : demo ? 8 : 20,
    type: ['receipt', 'payment', 'sale', 'purchase', 'journal'].includes(type) ? type : 'all',
    status: ['posted', 'draft'].includes(status) ? status : 'all',
    sort: params.get('sort') === 'amount' ? 'amount' : 'date',
    direction: params.get('direction') === 'asc' ? 'asc' as const : 'desc' as const,
  };
}

export function voucherReturnPath(value: unknown, fallback: string) {
  if (typeof value !== 'string' || !value.startsWith('/') || value.startsWith('//') || value.includes('\\')) return fallback;
  const path = value.split(/[?#]/, 1)[0];
  return path === '/' || /^\/(cash|bank|sales|purchases|documents|inventory)(\/reports)?$/.test(path) || /^\/(sales|purchases)\/(settlements|partners|items)$/.test(path) || /^\/(reports|receivables|settlements|catalog|automation|imports|settings|help)$/.test(path) ? value : fallback;
}
