import type { ReactNode } from 'react';
import { ImportExcelButton } from '../imports/ExcelImport';

export function ImportDocuments({ children }: { children?: (open: (kind?: string) => void) => ReactNode }) {
  return <ImportExcelButton>{children}</ImportExcelButton>;
}
