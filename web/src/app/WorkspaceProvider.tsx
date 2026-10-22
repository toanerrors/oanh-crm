import { createContext, useContext, useState } from 'react';
import type { ReactNode } from 'react';
import { useQuery } from '@tanstack/react-query';
import { workspaceOptions } from '../shared/data/workspace';
import type { AccountingDocument, DocumentType, Workspace } from '../shared/data/types';
import { apiScope, isDemo } from '../shared/api/client';
import type { OperationInput } from '../shared/data/operations';
import { businessToday } from '../shared/accounting/money';

interface WorkspaceContextValue {
  voucherDraft: OperationInput | null;
  setVoucherDraft: (draft: OperationInput | null) => void;
  period: string;
  setPeriod: (period: string) => void;
  data: Workspace | undefined;
  isPending: boolean;
  isFetching: boolean;
  error: Error | null;
  refresh: () => void;
  draft: AccountingDocument | null;
  setDraft: (draft: AccountingDocument | null) => void;
  draftOpen: boolean;
  setDraftOpen: (open: boolean) => void;
  newDraft: (type?: DocumentType) => void;
  editDraft: (document: AccountingDocument) => void;
  detailId: string | null;
  openDocument: (id: string | null) => void;
}

const WorkspaceContext = createContext<WorkspaceContextValue | null>(null);

export function WorkspaceProvider({ children }: { children: ReactNode }) {
  const [period, setPeriod] = useState(() => isDemo ? '2026-10' : businessToday().slice(0, 7));
  const [draft, setDraft] = useState<AccountingDocument | null>(null);
  const [voucherDraft, setVoucherDraft] = useState<OperationInput | null>(null);
  const [draftOpen, setDraftOpen] = useState(false);
  const [detailId, openDocument] = useState<string | null>(null);
  const query = useQuery(workspaceOptions(period));

  function newDraft(type: DocumentType = 'journal') {
    if (!draft) {
      const accounts: Record<DocumentType, [string, string]> = { receipt: ['1121', '131'], payment: ['642', '1121'], sale: ['131', '511'], purchase: ['156', '331'], journal: ['', ''] };
      const today = businessToday();
      setDraft({ datasetEpoch: isDemo ? undefined : apiScope().epoch, id: crypto.randomUUID(), reference: '', type, date: today.startsWith(period) ? today : `${period}-01`, description: '', status: 'draft', version: 0, lines: accounts[type].map(accountId => ({ accountId, description: '', debit: '0', credit: '0' })) });
    }
    setDraftOpen(true);
  }

  function editDraft(document: AccountingDocument) {
    if (draft && draft.id !== document.id) { setDraftOpen(true); return; }
    setDraft({ ...structuredClone(document), datasetEpoch: isDemo ? undefined : apiScope().epoch });
    openDocument(null);
    setDraftOpen(true);
  }

  return <WorkspaceContext.Provider value={{ voucherDraft, setVoucherDraft, period, setPeriod, data: query.data, isPending: query.isPending, isFetching: query.isFetching, error: query.error, refresh: () => { void query.refetch(); }, draft, setDraft, draftOpen, setDraftOpen, newDraft, editDraft, detailId, openDocument }}>{children}</WorkspaceContext.Provider>;
}

export function useWorkspace() {
  const context = useContext(WorkspaceContext);
  if (!context) throw new Error('Thiếu WorkspaceProvider');
  return context;
}
