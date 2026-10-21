function escapeCell(value: string): string {
  const safe = /^[\s]*[=+\-@\t\r]/.test(value) ? `'${value}` : value;
  return `"${safe.replace(/"/g, '""')}"`;
}

export function serializeCsv(rows: string[][]): string {
  return '\uFEFF' + rows.map(row => row.map(escapeCell).join(',')).join('\r\n');
}

export function downloadCsv(name: string, rows: string[][]): void {
  const url = URL.createObjectURL(new Blob([serializeCsv(rows)], { type: 'text/csv;charset=utf-8;' }));
  const link = document.createElement('a');
  link.href = url;
  link.download = name;
  link.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
