import { createContext, useCallback, useContext, useState } from 'react';
import type { ReactNode } from 'react';
import { useQuery } from '@tanstack/react-query';
import { workspaceOptions } from '../shared/data/workspace';
import type { AccountingDocument, DocumentType, Workspace } from '../shared/data/types';
import { apiScope, isDemo } from '../shared/api/client';
import { updateVoucherDraft } from '../features/vouchers/draft-state';
import type { VoucherDraft, VoucherDrafts } from '../features/vouchers/draft-state';
import { businessToday, isCalendarDate } from '../shared/accounting/money';
import { activePeriodPreset, monthDateRange } from '../shared/accounting/period-range';

interface WorkspaceContextValue {
  voucherDrafts: VoucherDrafts;
  setVoucherDraft: (key: string, draft: VoucherDraft | null) => void;
  period: string;
  fromDate: string;
  toDate: string;
  periodPreset: string;
  setPeriod: (period: string) => void;
  setDateRange: (from: string, to: string, preset?: string | null) => void;
  data: Workspace | undefined;
  isPending: boolean;
  isFetching: boolean;
  error: Error | null;
  refresh: () => void;
  documentDrafts: Record<string, AccountingDocument>;
  resumeDocumentDraft: (key: string) => void;
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
  const [range, setRange] = useState(() => ({
    ...monthDateRange(isDemo ? '2026-10' : businessToday().slice(0, 7)),
    preset: null as string | null,
  }));
  const fromDate = range.from;
  const toDate = range.to;
  const period = toDate.slice(0, 7);
  const setPeriod = useCallback((nextPeriod: string) => {
    setRange({ ...monthDateRange(nextPeriod), preset: null });
  }, []);
  const setDateRange = useCallback((from: string, to: string, preset: string | null = null) => {
    if (!isCalendarDate(from) || !isCalendarDate(to)) return;
    setRange({ ...(from <= to ? { from, to } : { from: to, to: from }), preset });
  }, []);
  const [documentDrafts, setDocumentDrafts] = useState<Record<string, AccountingDocument>>({});
  const [activeDraftKey, setActiveDraftKey] = useState<string | null>(null);
  const draft = activeDraftKey ? documentDrafts[activeDraftKey] ?? null : null;
  function setDraft(value: AccountingDocument | null) {
    if (!activeDraftKey) return;
    setDocumentDrafts(current => { const next = { ...current }; if (value) next[activeDraftKey] = structuredClone(value); else delete next[activeDraftKey]; return next; });
    if (!value) setActiveDraftKey(null);
  }
  function resumeDocumentDraft(key: string) { setActiveDraftKey(key); setDraftOpen(true); }
  const [voucherDrafts, setVoucherDrafts] = useState<VoucherDrafts>({});
  const setVoucherDraft = useCallback((key: string, value: VoucherDraft | null) => { setVoucherDrafts(current => updateVoucherDraft(current, key, value)); }, []);
  const [draftOpen, setDraftOpen] = useState(false);
  const [detailId, openDocument] = useState<string | null>(null);
  const query = useQuery(workspaceOptions(period));
  const periodPreset = activePeriodPreset(fromDate, toDate, range.preset, {
    today: businessToday(),
    demo: isDemo,
    conversionDate: query.data?.conversionDate,
  });

  function newDraft(type: DocumentType = 'journal') {
    const key = `new:${type}`;
    if (!documentDrafts[key]) {
      const accounts: Record<DocumentType, [string, string]> = { receipt: ['1121', '131'], payment: ['642', '1121'], sale: ['131', '511'], purchase: ['156', '331'], journal: ['', ''] };
      const today = businessToday();
      const value: AccountingDocument = { datasetEpoch: isDemo ? undefined : apiScope().epoch, id: crypto.randomUUID(), reference: '', type, date: today >= fromDate && today <= toDate ? today : `${period}-01`, description: '', status: 'draft', version: 0, lines: accounts[type].map(accountId => ({ accountId, description: '', debit: '0', credit: '0' })) };
      setDocumentDrafts(current => ({ ...current, [key]: value }));
    }
    resumeDocumentDraft(key);
  }

  function editDraft(document: AccountingDocument) {
    const key = `document:${document.id}`;
    if (!documentDrafts[key]) setDocumentDrafts(current => ({ ...current, [key]: { ...structuredClone(document), datasetEpoch: isDemo ? undefined : apiScope().epoch } }));
    setActiveDraftKey(key);
    openDocument(null);
    setDraftOpen(true);
  }

  return <WorkspaceContext.Provider value={{ voucherDrafts, setVoucherDraft, period, fromDate, toDate, periodPreset, setPeriod, setDateRange, data: query.data, isPending: query.isPending, isFetching: query.isFetching, error: query.error, refresh: () => { void query.refetch(); }, documentDrafts, resumeDocumentDraft, draft, setDraft, draftOpen, setDraftOpen, newDraft, editDraft, detailId, openDocument }}>{children}</WorkspaceContext.Provider>;
}

export function useWorkspace() {
  const context = useContext(WorkspaceContext);
  if (!context) throw new Error('Thiếu WorkspaceProvider');
  return context;
}
