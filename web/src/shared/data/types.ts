export type DocumentType = 'receipt' | 'payment' | 'sale' | 'purchase' | 'journal';
export type DocumentStatus = 'posted' | 'draft';

export interface JournalLine {
  accountId: string;
  partnerId?: string;
  description: string;
  debit: string;
  credit: string;
}

export interface AccountingDocument {
  voucher?: boolean;
  datasetEpoch?: string;
  sourceType?: string;
  sourceId?: string;
  policyId?: string;
  reversalOf?: string;
  id: string;
  reference: string;
  type: DocumentType;
  date: string;
  description: string;
  partnerId?: string;
  status: DocumentStatus;
  version: number;
  lines: JournalLine[];
}

export interface Partner {
  id: string;
  code: string;
  name: string;
  role: 'customer' | 'supplier' | 'both';
  email: string;
  phone: string;
  active: boolean;
}

export interface Account {
  active?: boolean;
  id: string;
  code: string;
  name: string;
  group: 'cash' | 'receivable' | 'payable' | 'revenue' | 'expense' | 'inventory' | 'capital' | 'asset' | 'tax' | 'liability' | 'wip';
  requiresPartner?: boolean;
}

export interface Workspace {
  regime?: string;
  server?: import('./reports').ServerMetrics;
  companyName?: string;
  mode?: 'live' | 'demo';
  permissions?: string;
  periods?: { period: string; closed: boolean }[];
  companyId: string;
  epoch: string;
  actor: string;
  revision: number;
  documents: AccountingDocument[];
  partners: Partner[];
  accounts: Account[];
}

export const documentTypes: Record<DocumentType, string> = {
  receipt: 'Phiếu thu', payment: 'Phiếu chi', sale: 'Bán hàng', purchase: 'Mua hàng', journal: 'Chứng từ khác',
};

export const documentStatuses: Record<DocumentStatus, string> = { posted: 'Đã ghi sổ', draft: 'Bản nháp' };
