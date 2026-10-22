export interface ExcelField { key: string; label: string; type: string; required: boolean; basic: boolean; choices?: string[]; default?: string; aliases?: string[] }
export interface ExcelSchema { key: string; label: string; group: string; owner: boolean; version: number; fields: ExcelField[] }
export interface ExcelCell { value: string; kind?: string; formula?: boolean }
export interface ExcelResult { batchId: string; kind: string; count: number; rows: number; documentIds: string[]; revision: number }
export interface ExcelBatch { id: string; filename: string; hash: string; state: string; kind?: string; createdAt: string; importedAt?: string; sheets: { name: string; rows: number }[]; result?: ExcelResult; configuration?: ExcelConfiguration }
export interface ExcelSource { batch: ExcelBatch; sheet: string; headerRow: number; headers: string[]; sample: ExcelCell[][]; sampleRows: number[]; populatedColumns: number[] }
export interface ExcelConfiguration { schemaVersion: number; kind: string; sheet: string; headerRow: number; mapping: Record<string, number>; dateFormat: string; numberFormat: string; mode: string; cutoff: string; source: string; ignoreUnmapped: boolean }
export interface ExcelIssue { row: number; column?: string; code: string; message: string }
export interface ExcelPreviewRow { row: number; source: string; description: string; status: string; debit: string; credit: string }
export interface ExcelPreview { batchId: string; revision: number; rows: ExcelPreviewRow[]; issues: ExcelIssue[]; errorCount: number; valid: number; invalid: number; records: number; debit: string; credit: string; fingerprint: string }

function headerKey(value: string) { return value.normalize('NFD').replace(/[\u0300-\u036f]/g, '').replace(/[đĐ]/g, 'd').toLowerCase().replace(/[^a-z0-9]/g, ''); }
export function matchExcelHeaders(headers: string[], fields: ExcelField[]) {
  const candidates = fields.map(field => headers.flatMap((header, index) => (field.aliases ?? [field.key, field.label]).some(alias => headerKey(alias) === headerKey(header)) ? [index] : []));
  const mapping: Record<string, number> = {};
  for (let i = 0; i < fields.length; i++) {
    const matches = candidates[i];
    if (matches.length === 1 && candidates.filter(values => values.includes(matches[0])).length === 1) mapping[fields[i].key] = matches[0];
  }
  return mapping;
}
export function unusedExcelColumns(source: ExcelSource, mapping: Record<string, number>) {
  const used = new Set(Object.values(mapping));
  return source.headers.flatMap((header, col) => !used.has(col) && source.populatedColumns.includes(col) ? [header] : []);
}
export function excelKindForPath(path: string) {
  if (path.startsWith('/purchases')) return path.endsWith('/partners') ? 'partners' : path.endsWith('/items') ? 'items' : 'purchase';
  if (path.startsWith('/sales')) return path.endsWith('/partners') ? 'partners' : path.endsWith('/items') ? 'items' : 'sale';
  if (path.startsWith('/cash') || path.startsWith('/bank')) return 'receipt';
  if (path.startsWith('/catalog')) return 'partners';
  return 'journal-pairs';
}
