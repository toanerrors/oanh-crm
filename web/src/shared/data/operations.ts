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
export interface Item { id: string; code: string; name: string; unit: string; tracked: boolean; active: boolean; quantity?: string; value?: string; scale?: number }
export interface Employee { id: string; code: string; name: string; baseSalary: string; insuranceBase: string; dependents: number; active: boolean }
export interface Asset { id: string; code: string; name: string; kind: 'fixed' | 'prepaid'; cost: string; residual: string; months: number; startPeriod: string; allocated?: string }
export interface AutomationData { policies: Policy[]; items: Item[]; employees: Employee[]; assets: Asset[] }
export interface OperationInput {
  originalDocumentId?: string;
  moneyAccount?: string; counterAccount?: string; destinationAccount?: string;
  allocations?: { documentId: string; amount: string }[];
  invoiceNumber?: string; invoiceSeries?: string; invoiceDate?: string; dueDate?: string;
  expectedPolicyId?: string; expectedRevision?: number;
  kind: 'sale' | 'purchase' | 'receipt' | 'payment' | 'transfer' | 'treasury_reversal' | 'payroll' | 'asset' | 'depreciation' | 'costing' | 'closing' | 'corporate_tax';
  taxFrom?: string; nonDeductible?: string; exemptIncome?: string; carriedLoss?: string;
  sourceId: string; date: string; description: string; partnerId?: string; priceMode?: 'exclusive' | 'inclusive';
  lines?: { itemId: string; quantity: string; unitPrice: string; vatRate: string; deductible?: boolean }[];
  employeeId?: string; workedDays?: string; allowance?: string; nonTaxableAllowance?: string; overtimeHours?: string; overtimeMultiplier?: string;
  asset?: Asset; assetId?: string; amount?: string;
}
export interface TradeInvoice {
  documentId: string; reference: string; kind: 'sale' | 'purchase'; date: string; description: string;
  partnerId: string; partnerName: string; invoiceNumber: string; invoiceSeries: string; invoiceDate: string; dueDate: string;
  amount: string; paid: string; remaining: string; unallocated: boolean;
}
export interface InvoiceList { invoices: TradeInvoice[]; total: number; amount: string; paid: string; remaining: string; revision: number }
export interface Calculation { policyId: string; policyVersion: number; revision: number; document: AccountingDocument; amounts: Record<string, string>; notes: string[] }
export function automationOptions() { return queryOptions({ queryKey: [...workspaceKey(), 'automation'], queryFn: ({ signal }) => api<AutomationData>('/automation', { signal }), staleTime: 0, retry: false }); }
export const executeOperation = (input: OperationInput) => command<Calculation>('/operations', input);

export const amountLabels: Record<string, string> = { baseSalary: 'Lương cơ sở đã áp dụng', insuranceBase: 'Cơ sở bảo hiểm đã áp dụng', base: 'Giá trị trước thuế', tax: 'Thuế', total: 'Tổng thanh toán', cost: 'Giá vốn / nguyên giá', gross: 'Tổng thu nhập', net: 'Thực lĩnh', inputTax: 'VAT đầu vào được khấu trừ', taxable: 'Thu nhập tính thuế', employeeInsurance: 'BH người lao động', employerInsurance: 'BH doanh nghiệp', overtime: 'Làm thêm', amount: 'Giá trị ghi nhận', accumulated: 'Lũy kế', remaining: 'Còn lại', result: 'Kết quả kinh doanh', profitBeforeTax: 'Lợi nhuận trước thuế' };
