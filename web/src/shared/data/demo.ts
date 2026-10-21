import type { AccountingDocument, DocumentType, Workspace } from './types.ts';

export const demoScope = { actor: 'demo-user', permissions: 'demo-editor', companyId: 'demo-company', epoch: 'demo-2026' } as const;

function entry(id: string, reference: string, date: string, type: DocumentType, description: string, amount: string, debit: string, credit: string, partnerId?: string, status: 'posted' | 'draft' = 'posted'): AccountingDocument {
  return { id, reference, date, type, description, partnerId, status, version: 1, lines: [
    { accountId: debit, partnerId, description, debit: amount, credit: '0' },
    { accountId: credit, partnerId, description, debit: '0', credit: amount },
  ] };
}

export const initialWorkspace: Workspace = {
  ...demoScope, revision: 1,
  accounts: [
    { id: '111', code: '111', name: 'Tiền mặt', group: 'cash' },
    { id: '1121', code: '1121', name: 'Tiền gửi ngân hàng', group: 'cash' },
    { id: '131', code: '131', name: 'Phải thu của khách hàng', group: 'receivable', requiresPartner: true },
    { id: '156', code: '156', name: 'Hàng hóa', group: 'inventory' },
    { id: '211', code: '211', name: 'Tài sản cố định hữu hình', group: 'asset' },
    { id: '331', code: '331', name: 'Phải trả cho người bán', group: 'payable', requiresPartner: true },
    { id: '411', code: '411', name: 'Vốn đầu tư của chủ sở hữu', group: 'capital' },
    { id: '511', code: '511', name: 'Doanh thu bán hàng và cung cấp dịch vụ', group: 'revenue' },
    { id: '632', code: '632', name: 'Giá vốn hàng bán', group: 'expense' },
    { id: '642', code: '642', name: 'Chi phí quản lý doanh nghiệp', group: 'expense' },
  ],
  partners: [
    { id: 'p1', code: 'KH001', name: 'Công ty TNHH Minh Anh', role: 'customer', email: 'minhanh@example.com', phone: '0901234001', active: true },
    { id: 'p2', code: 'KH002', name: 'Công ty CP Nội thất An Phát', role: 'customer', email: 'anphat@example.com', phone: '0901234002', active: true },
    { id: 'p3', code: 'NCC001', name: 'Công ty TNHH Thương mại Việt Hưng', role: 'supplier', email: 'viethung@example.com', phone: '0901234003', active: true },
    { id: 'p4', code: 'KH003', name: 'Công ty TNHH Green Home', role: 'both', email: 'greenhome@example.com', phone: '0901234004', active: true },
    { id: 'p5', code: 'NCC002', name: 'Công ty CP Công nghệ Sao Mai', role: 'supplier', email: 'saomai@example.com', phone: '0901234005', active: true },
    { id: 'p6', code: 'KH004', name: 'Công ty TNHH Dịch vụ Đại Việt', role: 'customer', email: 'daiviet@example.com', phone: '0901234006', active: true },
  ],
  documents: [
    entry('opening', 'NV-DAU', '2026-01-01', 'journal', 'Bộ số mẫu: góp vốn bằng tiền gửi', '980000000', '1121', '411'),
    entry('may-sales', 'BH-05001', '2026-05-20', 'sale', 'Dịch vụ tư vấn tháng 5', '285000000', '131', '511', 'p1'),
    entry('may-cost', 'PC-05001', '2026-05-21', 'payment', 'Chi phí dịch vụ tháng 5', '165000000', '642', '1121'),
    entry('jun-sales', 'BH-06001', '2026-06-20', 'sale', 'Dịch vụ tư vấn tháng 6', '342000000', '131', '511', 'p2'),
    entry('jun-cost', 'PC-06001', '2026-06-21', 'payment', 'Chi phí dịch vụ tháng 6', '192000000', '642', '1121'),
    entry('jul-sales', 'BH-07001', '2026-07-20', 'sale', 'Dịch vụ tư vấn tháng 7', '310000000', '131', '511', 'p4'),
    entry('jul-cost', 'PC-07001', '2026-07-21', 'payment', 'Chi phí dịch vụ tháng 7', '178000000', '642', '1121'),
    entry('aug-sales', 'BH-08001', '2026-08-20', 'sale', 'Dịch vụ tư vấn tháng 8', '425000000', '131', '511', 'p1'),
    entry('aug-cost', 'PC-08001', '2026-08-21', 'payment', 'Chi phí dịch vụ tháng 8', '235000000', '642', '1121'),
    entry('sep-sales', 'BH-09001', '2026-09-20', 'sale', 'Dịch vụ tư vấn tháng 9', '396000000', '131', '511', 'p2'),
    entry('sep-receipt', 'PT-09001', '2026-09-21', 'receipt', 'Khách hàng thanh toán công nợ', '1200000000', '1121', '131', 'p1'),
    entry('sep-cost', 'PC-09001', '2026-09-22', 'payment', 'Chi phí dịch vụ tháng 9', '218000000', '642', '1121'),
    entry('d1', 'BH-10008', '2026-10-21', 'sale', 'Dịch vụ thiết kế không gian văn phòng', '185000000', '131', '511', 'p1'),
    entry('d2', 'PT-10012', '2026-10-21', 'receipt', 'Thu tiền hợp đồng thiết kế nội thất', '120000000', '1121', '131', 'p2'),
    entry('d3', 'PC-10009', '2026-10-20', 'payment', 'Thanh toán chi phí thuê văn phòng tháng 10', '28500000', '642', '1121', 'p5'),
    entry('d4', 'MH-10006', '2026-10-20', 'purchase', 'Mua vật tư hoàn thiện dự án Green Home', '76500000', '156', '331', 'p3'),
    entry('d5', 'BH-10007', '2026-10-19', 'sale', 'Dịch vụ tư vấn và triển khai nhận diện', '248000000', '131', '511', 'p4'),
    entry('d6', 'PC-10008', '2026-10-19', 'payment', 'Chi phí triển khai dự án tháng 10', '157000000', '642', '1121', 'p5'),
    entry('d7', 'PT-10011', '2026-10-18', 'receipt', 'Thu tiền dịch vụ tư vấn đợt 2', '165000000', '1121', '131', 'p4'),
    entry('d8', 'MH-10005', '2026-10-17', 'purchase', 'Mua thiết bị phục vụ dự án An Phát', '42000000', '156', '331', 'p5'),
    entry('d9', 'BH-10006', '2026-10-16', 'sale', 'Dịch vụ tư vấn doanh nghiệp tháng 10', '95000000', '131', '511', 'p6'),
    entry('d10', 'PC-10007', '2026-10-15', 'payment', 'Chi phí phần mềm và dịch vụ văn phòng', '12500000', '642', '1121', 'p5'),
    entry('d11', 'NHAP-A', '2026-10-21', 'sale', 'Dự thảo dịch vụ tư vấn dự án An Phát', '68000000', '131', '511', 'p2', 'draft'),
    entry('d12', 'NHAP-B', '2026-10-20', 'payment', 'Dự thảo thanh toán vật tư Việt Hưng', '35000000', '331', '1121', 'p3', 'draft'),
    entry('d13', 'NHAP-C', '2026-10-19', 'journal', 'Dự thảo chi phí vận hành tháng 10', '8600000', '642', '111', undefined, 'draft'),
  ],
};
