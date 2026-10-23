import { queryOptions } from '@tanstack/react-query';
import { api, command } from '../api/client';
import { workspaceKey } from './workspace';
import type { AccountingDocument } from './types';

export interface Policy {
  regime: string;
  corporateTaxRate: string;
  id?: string; version?: number; effectiveFrom: string; confirmed: boolean;
  rounding: 'half_up' | 'half_even' | 'down'; vatMethod: 'deduction' | 'direct'; vatRates: string[];
  inventoryMethod: 'fifo' | 'moving_average'; quantityScale: number; standardDays: string; hoursPerDay: string;
  personalDeduction: string; dependentDeduction: string;
  insurance: { name: string; employeeRate: string; employerRate: string }[];
  pitBrackets: { upTo: string; rate: string }[];
  depreciationMethod: 'straight_line'; residualMethod: 'last_period';
}
export interface Item { hasReferences?: boolean; id: string; code: string; name: string; unit: string; tracked: boolean; active: boolean; quantity?: string; value?: string; scale?: number }
export interface Employee { hasReferences?: boolean; id: string; code: string; name: string; baseSalary: string; insuranceBase: string; dependents: number; active: boolean }
export interface Asset { id: string; code: string; name: string; kind: 'fixed' | 'prepaid'; cost: string; residual: string; months: number; startPeriod: string; allocated?: string }
export interface AutomationData { policies: Policy[]; items: Item[]; employees: Employee[]; assets: Asset[] }
export interface VoucherInfo {
  documentDate?: string; contact?: string; address?: string; taxCode?: string; employee?: string;
  bankNumber?: string; bankName?: string; destinationBankNumber?: string; destinationBankName?: string;
  reference?: string; contractNumber?: string; notes?: string; invoiceLookupCode?: string; invoiceLookupUrl?: string; originalCount?: string;
}
export interface OperationInput {
  bankAccountId?: string; destinationBankAccountId?: string;
  discount?: { mode: 'none' | 'line' | 'percent' | 'total'; rate?: string; amount?: string; allocation?: 'value' | 'quantity' | 'manual' };
  costAllocation?: 'manual' | 'quantity' | 'value' | 'percent'; isPurchaseCost?: boolean; purchaseCosts?: { documentId: string; line: number; amount: string }[];
  draftId?: string; draftVersion?: number; voucher?: VoucherInfo;
  tradeMode?: 'goods' | 'service'; paymentAccount?: string;
  treasuryLines?: { counterAccount: string; partnerId?: string; description: string; amount: string }[];
  originalDocumentId?: string;
  replacementOf?: string;
  replacementReason?: string;
  moneyAccount?: string; counterAccount?: string; destinationAccount?: string;
  allocations?: { documentId: string; amount: string }[];
  invoiceNumber?: string; invoiceSeries?: string; invoiceDate?: string; dueDate?: string;
  expectedPolicyId?: string; expectedRevision?: number;
  kind: 'sale' | 'purchase' | 'receipt' | 'payment' | 'transfer' | 'treasury_reversal' | 'payroll' | 'asset' | 'depreciation' | 'costing' | 'closing' | 'corporate_tax';
  taxFrom?: string; nonDeductible?: string; exemptIncome?: string; carriedLoss?: string;
  sourceId: string; date: string; description: string; partnerId?: string; priceMode?: 'exclusive' | 'inclusive';
  lines?: { costPercent?: string; warehouseId?: string; discountRate?: string; discountAmount?: string; purchaseCost?: string; description?: string; accountId?: string; itemId: string; quantity: string; unitPrice: string; vatRate: string; deductible?: boolean }[];
  employeeId?: string; workedDays?: string; allowance?: string; nonTaxableAllowance?: string; overtimeHours?: string; overtimeMultiplier?: string;
  asset?: Asset; assetId?: string; amount?: string;
}
export interface TradeInvoice {
  documentId: string; reference: string; kind: 'sale' | 'purchase'; date: string; description: string;
  partnerId: string; partnerName: string; invoiceNumber: string; invoiceSeries: string; invoiceDate: string; dueDate: string;
  amount: string; paid: string; remaining: string; unallocated: boolean;
}
export interface InvoiceList { invoices: TradeInvoice[]; total: number; amount: string; paid: string; remaining: string; revision: number }
export interface Voucher { replacement?: AccountingDocument; reversal?: AccountingDocument; calculation?: Calculation; input: OperationInput; document: AccountingDocument }
export interface Calculation { lineAmounts?: { base: string; tax: string; total: string; discount?: string; purchaseCost?: string; purchaseValue?: string }[]; paymentDocument?: AccountingDocument; policyId: string; policyVersion: number; revision: number; document: AccountingDocument; amounts: Record<string, string>; notes: string[] }
export type TradeAmounts = Pick<Calculation, 'lineAmounts' | 'amounts' | 'policyId' | 'policyVersion' | 'revision'>;
export function automationOptions() { return queryOptions({ queryKey: [...workspaceKey(), 'automation'], queryFn: ({ signal }) => api<AutomationData>('/automation', { signal }), staleTime: 0, retry: false }); }
export const executeOperation = (input: OperationInput) => command<Calculation>('/operations', input);

export const amountLabels: Record<string, string> = { discount: 'Chiết khấu', purchaseCost: 'Chi phí mua hàng', baseSalary: 'Lương cơ sở đã áp dụng', insuranceBase: 'Cơ sở bảo hiểm đã áp dụng', base: 'Giá trị trước thuế', tax: 'Thuế', total: 'Tổng thanh toán', cost: 'Giá vốn / nguyên giá', gross: 'Tổng thu nhập', net: 'Thực lĩnh', inputTax: 'VAT đầu vào được khấu trừ', taxable: 'Thu nhập tính thuế', employeeInsurance: 'BH người lao động', employerInsurance: 'BH doanh nghiệp', overtime: 'Làm thêm', amount: 'Giá trị ghi nhận', accumulated: 'Lũy kế', remaining: 'Còn lại', result: 'Kết quả kinh doanh', profitBeforeTax: 'Lợi nhuận trước thuế' };

export interface BankAccount { hasReferences?: boolean; id: string; number: string; bankName: string; accountId: string; active: boolean; opening?: string; receipts?: string; payments?: string; closing?: string }
export interface BankList { accounts: BankAccount[]; revision: number; epoch: string }
export function banksOptions(period: string) { return queryOptions({ queryKey: [...workspaceKey(), 'bank-accounts', period], queryFn: ({ signal }) => api<BankList>(`/bank-accounts?period=${period}`, { signal }), staleTime: 0, retry: false }); }

export interface Warehouse { hasReferences?: boolean; id: string; code: string; name: string; active: boolean }
export interface WarehouseList { warehouses: Warehouse[]; stock: { warehouseId: string; itemId: string; quantity: string; value: string; scale: number }[]; revision: number; epoch: string }
export interface PurchaseCostSource { documentId: string; reference: string; date: string; line: number; description: string; accountId: string; amount: string; remaining: string }
export function warehousesOptions() { return queryOptions({ queryKey: [...workspaceKey(), 'warehouses'], queryFn: ({ signal }) => api<WarehouseList>('/warehouses', { signal }), staleTime: 0, retry: false }); }
