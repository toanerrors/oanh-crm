package operations

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/modules/ledger"
	"oanh.local/accounting/internal/platform/sqlite"
)

func (s *Service) excelReferenceTx(ctx context.Context, tx *sql.Tx, kind, value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if kind == "account" || kind == "partner" {
		return s.Ledger.ResolveImportCodeTx(ctx, tx, kind, value)
	}
	query := ""
	switch kind {
	case "item":
		query = "SELECT id FROM items WHERE code=? COLLATE NOCASE AND active=1"
	case "warehouse":
		query = "SELECT id FROM warehouses WHERE code=? COLLATE NOCASE AND active=1"
	case "bank":
		query = "SELECT id FROM bank_accounts WHERE number=? AND active=1"
	default:
		return "", accounting.Fail("VALIDATION", "Loại mã tham chiếu không hợp lệ.")
	}
	var id string
	err := tx.QueryRowContext(ctx, query, value).Scan(&id)
	if err == sql.ErrNoRows {
		return "", accounting.Fail("INVALID_REFERENCE", fmt.Sprintf("Không tìm thấy mã %s trong danh mục %s đang sử dụng.", value, kind))
	}
	return id, err
}
func (s *Service) excelCatalogIDTx(ctx context.Context, tx *sql.Tx, kind, value string) (string, error) {
	if kind == "partners" || kind == "accounts" {
		return s.Ledger.ImportCatalogIDTx(ctx, tx, kind, value)
	}
	query := ""
	switch kind {
	case "items":
		query = "SELECT id FROM items WHERE code=? COLLATE NOCASE"
	case "warehouses":
		query = "SELECT id FROM warehouses WHERE code=? COLLATE NOCASE"
	case "employees":
		query = "SELECT id FROM employees WHERE code=? COLLATE NOCASE"
	case "banks":
		query = "SELECT id FROM bank_accounts WHERE number=?"
	default:
		return "", accounting.Fail("VALIDATION", "Loại danh mục không hợp lệ.")
	}
	var id string
	err := tx.QueryRowContext(ctx, query, value).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return id, err
}
func excelID(batchID, kind, key string) string {
	return "imp-" + sqlite.Hash([]byte(batchID + ":" + kind + ":" + key))[:32]
}
func (s *Service) applyExcelRecord(ctx context.Context, tx *sql.Tx, batch ExcelBatch, config ExcelConfiguration, record excelRecord, seen map[string]bool) (*ledger.Document, error) {
	values := record.Values[0]
	schema, _ := excelSchema(config.Kind)
	if schema.Group == "Danh mục" {
		code := values["code"]
		if config.Kind == "banks" {
			code = values["number"]
		}
		if seen[strings.ToUpper(code)] {
			return nil, accounting.Fail("DUPLICATE_CODE", "Mã bị lặp trong cùng tệp nhập.")
		}
		seen[strings.ToUpper(code)] = true
		id, err := s.excelCatalogIDTx(ctx, tx, config.Kind, code)
		if err != nil {
			return nil, err
		}
		if id != "" && config.Mode == "add" {
			return nil, accounting.Fail("DUPLICATE_CODE", "Mã đã tồn tại. Chọn chế độ cập nhật nếu muốn sửa bản ghi có sẵn.")
		}
		if id == "" && config.Mode == "update" {
			return nil, accounting.Fail("NOT_FOUND", "Chế độ cập nhật yêu cầu mã đã tồn tại trong danh mục.")
		}
		if id != "" && config.Mode == "update" {
			prior := map[string]string{}
			if config.Kind == "partners" || config.Kind == "accounts" {
				prior, err = s.Ledger.ImportCatalogOptionalTx(ctx, tx, config.Kind, id)
			} else {
				query := map[string]string{"items": "SELECT active FROM items WHERE id=?", "warehouses": "SELECT active FROM warehouses WHERE id=?", "banks": "SELECT active FROM bank_accounts WHERE id=?", "employees": "SELECT active FROM employees WHERE id=?"}[config.Kind]
				var active bool
				err = tx.QueryRowContext(ctx, query, id).Scan(&active)
				prior["active"] = strconv.FormatBool(active)
			}
			if err != nil {
				return nil, err
			}
			for key, value := range prior {
				if _, mapped := config.Mapping[key]; !mapped {
					values[key] = value
				}
			}
		}
		if id == "" {
			id = excelID(batch.ID, config.Kind, code)
		}
		active := values["active"] == "true"
		switch config.Kind {
		case "partners":
			_, err = s.Ledger.SavePartnersTx(ctx, tx, []ledger.Partner{{ID: id, Code: code, Name: values["name"], Role: values["role"], Email: values["email"], Phone: values["phone"], Active: active}})
		case "accounts":
			_, err = s.Ledger.SaveAccountTx(ctx, tx, ledger.Account{ID: id, Code: code, Name: values["name"], Group: values["group"], RequiresPartner: values["requiresPartner"] == "true", Active: active})
		case "items":
			_, err = s.saveItemTx(ctx, tx, Item{ID: id, Code: code, Name: values["name"], Unit: values["unit"], Tracked: values["tracked"] == "true", Active: active})
		case "warehouses":
			_, err = s.saveWarehouseTx(ctx, tx, Warehouse{ID: id, Code: code, Name: values["name"], Active: active})
		case "banks":
			account, e := s.excelReferenceTx(ctx, tx, "account", values["accountCode"])
			if e != nil {
				return nil, e
			}
			_, err = s.saveBankTx(ctx, tx, BankAccount{ID: id, Number: code, BankName: values["bankName"], AccountID: account, Active: active})
		case "employees":
			dependents, e := strconv.Atoi(values["dependents"])
			if e != nil {
				return nil, accounting.Fail("VALIDATION", "Số người phụ thuộc không hợp lệ.")
			}
			_, err = s.saveEmployeeTx(ctx, tx, Employee{ID: id, Code: code, Name: values["name"], BaseSalary: values["baseSalary"], InsuranceBase: values["insuranceBase"], Dependents: dependents, Active: active})
		}
		return nil, err
	}
	if config.Kind == "opening" {
		return s.excelOpeningTx(ctx, tx, batch, config, record)
	}
	lineFields := map[string]bool{"debitAccount": true, "debitPartner": true, "creditAccount": true, "creditPartner": true, "amount": true, "lineDescription": true, "accountCode": true, "linePartnerCode": true, "debit": true, "credit": true, "counterAccount": true, "itemCode": true, "warehouseCode": true, "quantity": true, "unitPrice": true, "vatRate": true, "discountRate": true, "discountAmount": true, "deductible": true, "invoiceSourceId": true, "allocationAmount": true, "costSourceId": true, "costSourceLine": true, "costAmount": true, "purchaseCostAmount": true, "costPercent": true}
	for _, row := range record.Values[1:] {
		for key, value := range values {
			if !lineFields[key] && row[key] != value {
				return nil, accounting.Fail("INCONSISTENT_HEADER", "Các dòng cùng mã chứng từ nguồn phải có cùng thông tin chung: "+key)
			}
		}
	}
	partner, err := s.excelReferenceTx(ctx, tx, "partner", values["partnerCode"])
	if err != nil {
		return nil, err
	}
	if config.Kind == "journal-pairs" || config.Kind == "journal-lines" {
		doc := ledger.Document{ID: excelID(batch.ID, config.Kind, values["sourceId"]), SourceID: values["sourceId"], SourceType: "journal", Type: "journal", Date: values["date"], Description: values["description"], PartnerID: partner, Lines: []ledger.Line{}}
		for _, row := range record.Values {
			if config.Kind == "journal-pairs" {
				debit, e := s.excelReferenceTx(ctx, tx, "account", row["debitAccount"])
				if e != nil {
					return nil, e
				}
				credit, e := s.excelReferenceTx(ctx, tx, "account", row["creditAccount"])
				if e != nil {
					return nil, e
				}
				dp, e := s.excelReferenceTx(ctx, tx, "partner", row["debitPartner"])
				if e != nil {
					return nil, e
				}
				cp, e := s.excelReferenceTx(ctx, tx, "partner", row["creditPartner"])
				if e != nil {
					return nil, e
				}
				doc.Lines = append(doc.Lines, ledger.Line{AccountID: debit, PartnerID: dp, Description: row["lineDescription"], Debit: row["amount"], Credit: "0"}, ledger.Line{AccountID: credit, PartnerID: cp, Description: row["lineDescription"], Debit: "0", Credit: row["amount"]})
			} else {
				account, e := s.excelReferenceTx(ctx, tx, "account", row["accountCode"])
				if e != nil {
					return nil, e
				}
				lp, e := s.excelReferenceTx(ctx, tx, "partner", row["linePartnerCode"])
				if e != nil {
					return nil, e
				}
				doc.Lines = append(doc.Lines, ledger.Line{AccountID: account, PartnerID: lp, Description: row["lineDescription"], Debit: row["debit"], Credit: row["credit"]})
			}
		}
		doc, err = s.Ledger.SaveTx(ctx, tx, doc)
		if err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, "SAVEPOINT excel_journal_validation"); err != nil {
			return nil, err
		}
		_, check := s.Ledger.PostTx(ctx, tx, ledger.Post{ID: doc.ID, Version: doc.Version})
		if _, err = tx.ExecContext(ctx, "ROLLBACK TO excel_journal_validation"); err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, "RELEASE excel_journal_validation"); err != nil {
			return nil, err
		}
		if check != nil {
			return nil, check
		}
		return &doc, nil
	}
	in := Input{Kind: config.Kind, SourceID: values["sourceId"], Date: values["date"], Description: values["description"], PartnerID: partner, PriceMode: values["priceMode"], TradeMode: values["tradeMode"], InvoiceNumber: values["invoiceNumber"], InvoiceSeries: values["invoiceSeries"], InvoiceDate: values["invoiceDate"], DueDate: values["dueDate"], IsPurchaseCost: values["isPurchaseCost"] == "true", Discount: Discount{Mode: values["discountMode"], Rate: values["invoiceDiscountRate"], Amount: values["invoiceDiscountAmount"], Allocation: values["discountAllocation"]}, Voucher: VoucherInfo{DocumentDate: values["documentDate"], Contact: values["contact"], Address: values["address"], TaxCode: values["taxCode"], Employee: values["employee"], Reference: values["reference"], ContractNumber: values["contractNumber"], Notes: values["notes"], OriginalCount: values["originalCount"], InvoiceLookupCode: values["invoiceLookupCode"], InvoiceLookupURL: values["invoiceLookupUrl"]}}
	in.CostAllocation = values["costAllocation"]
	in.MoneyAccount, err = s.excelReferenceTx(ctx, tx, "account", values["moneyAccount"])
	if err != nil {
		return nil, err
	}
	in.BankAccountID, err = s.excelReferenceTx(ctx, tx, "bank", values["bankNumber"])
	if err != nil {
		return nil, err
	}
	in.DestinationAccount, err = s.excelReferenceTx(ctx, tx, "account", values["destinationAccount"])
	if err != nil {
		return nil, err
	}
	in.DestinationBankAccountID, err = s.excelReferenceTx(ctx, tx, "bank", values["destinationBankNumber"])
	if err != nil {
		return nil, err
	}
	in.PaymentAccount, err = s.excelReferenceTx(ctx, tx, "account", values["paymentAccount"])
	if err != nil {
		return nil, err
	}
	if values["moneyAccount"] == "1121" && in.BankAccountID == "" || values["paymentAccount"] == "1121" && in.BankAccountID == "" || values["destinationAccount"] == "1121" && in.DestinationBankAccountID == "" {
		return nil, accounting.Fail("INVALID_BANK", "Chứng từ tiền gửi phải có số tài khoản ngân hàng đã khai báo.")
	}
	switch config.Kind {
	case "receipt", "payment":
		total := int64(0)
		hasAllocation := false
		for _, row := range record.Values {
			if row["invoiceSourceId"] != "" {
				hasAllocation = true
			}
		}
		for _, row := range record.Values {
			amount, _ := accounting.Money(row["amount"])
			total, err = accounting.Add(total, amount)
			if err != nil {
				return nil, err
			}
			if hasAllocation {
				if row["invoiceSourceId"] == "" || row["counterAccount"] != "" || row["allocationAmount"] == "" {
					return nil, accounting.Fail("VALIDATION", "Phiếu phân bổ công nợ cần nguồn công nợ, số tiền phân bổ trên từng dòng; để trống tài khoản đối ứng.")
				}
				sourceKind := "sale"
				if in.Kind == "payment" {
					sourceKind = "purchase"
				}
				sourceID, e := s.Ledger.ResolveImportDocumentTx(ctx, tx, sourceKind, row["invoiceSourceId"])
				if e != nil {
					return nil, e
				}
				in.Allocations = append(in.Allocations, Allocation{DocumentID: sourceID, Amount: row["allocationAmount"]})
				continue
			}
			account, e := s.excelReferenceTx(ctx, tx, "account", row["counterAccount"])
			if e != nil {
				return nil, e
			}
			lp, e := s.excelReferenceTx(ctx, tx, "partner", row["linePartnerCode"])
			if e != nil {
				return nil, e
			}
			in.TreasuryLines = append(in.TreasuryLines, TreasuryLine{CounterAccount: account, PartnerID: lp, Description: row["lineDescription"], Amount: row["amount"]})
		}
		in.Amount = accounting.Text(total)
	case "transfer":
		if len(record.Values) != 1 {
			return nil, accounting.Fail("VALIDATION", "Mỗi chứng từ chuyển tiền dùng một dòng dữ liệu.")
		}
		in.Amount = values["amount"]
	case "sale", "purchase":
		for _, row := range record.Values {
			item, e := s.excelReferenceTx(ctx, tx, "item", row["itemCode"])
			if e != nil {
				return nil, e
			}
			warehouse, e := s.excelReferenceTx(ctx, tx, "warehouse", row["warehouseCode"])
			if e != nil {
				return nil, e
			}
			account, e := s.excelReferenceTx(ctx, tx, "account", row["accountCode"])
			if e != nil {
				return nil, e
			}
			line := TradeLine{ItemID: item, WarehouseID: warehouse, AccountID: account, Description: row["lineDescription"], Quantity: row["quantity"], UnitPrice: row["unitPrice"], VATRate: row["vatRate"], DiscountRate: row["discountRate"], DiscountAmount: row["discountAmount"], PurchaseCost: row["purchaseCostAmount"], CostPercent: row["costPercent"]}
			if row["deductible"] != "" {
				value := row["deductible"] == "true"
				line.Deductible = &value
			}
			if row["costSourceId"] != "" {
				sourceID, e := s.Ledger.ResolveImportDocumentTx(ctx, tx, "purchase", row["costSourceId"])
				if e != nil {
					return nil, e
				}
				sourceLine, e := strconv.Atoi(row["costSourceLine"])
				if e != nil || sourceLine < 1 || row["costAmount"] == "" {
					return nil, accounting.Fail("VALIDATION", "Nguồn chi phí cần số dòng từ 1 và số tiền lấy từ nguồn.")
				}
				in.PurchaseCosts = append(in.PurchaseCosts, PurchaseCost{DocumentID: sourceID, Line: sourceLine - 1, Amount: row["costAmount"]})
			} else if row["costSourceLine"] != "" || row["costAmount"] != "" {
				return nil, accounting.Fail("VALIDATION", "Thiếu mã chứng từ nguồn chi phí mua hàng.")
			}
			in.Lines = append(in.Lines, line)
		}
	default:
		return nil, accounting.Fail("VALIDATION", "Loại nhập chứng từ không hợp lệ.")
	}
	value, err := s.saveVoucherTx(ctx, tx, in)
	if err != nil {
		return nil, err
	}
	voucher := value.(Voucher)
	return &voucher.Document, nil
}

func (s *Service) excelOpeningTx(ctx context.Context, tx *sql.Tx, batch ExcelBatch, config ExcelConfiguration, record excelRecord) (*ledger.Document, error) {
	state, err := openingAt(ctx, tx)
	if err != nil {
		return nil, err
	}
	if state.Document != nil && state.Document.Status == "posted" {
		return nil, accounting.Fail("OPENING_LOCKED", "Số dư đầu kỳ đã xác nhận, không nhập bổ sung trực tiếp.")
	}
	in := OpeningInput{ID: excelID(batch.ID, "opening", "initial"), Cutoff: config.Cutoff, Source: config.Source, Lines: []OpeningLine{}, Stocks: []OpeningStock{}, Assets: []Asset{}}
	if !accounting.Date(config.Cutoff) || config.Cutoff[8:] != "01" || strings.TrimSpace(config.Source) == "" {
		return nil, accounting.Fail("VALIDATION", "Nhập ngày đầu tháng chuyển đổi và nguồn đối chiếu số dư.")
	}
	if state.Input != nil {
		if state.Input.Cutoff != config.Cutoff {
			return nil, accounting.Fail("VERSION_CONFLICT", "Ngày chuyển đổi khác bản nháp số dư hiện có.")
		}
		in = *state.Input
	}
	seen := map[string]bool{}
	for _, line := range in.Lines {
		seen["account:"+line.AccountID+":"+line.PartnerID+":"+line.BankAccountID] = true
	}
	for _, stock := range in.Stocks {
		seen["stock:"+stock.ItemID+":"+stock.WarehouseID] = true
	}
	for _, asset := range in.Assets {
		seen["asset:"+strings.ToUpper(asset.Code)] = true
	}
	for _, row := range record.Values {
		kind := row["rowKind"]
		key := ""
		switch kind {
		case "account":
			account, e := s.excelReferenceTx(ctx, tx, "account", row["accountCode"])
			if e != nil {
				return nil, e
			}
			partner, e := s.excelReferenceTx(ctx, tx, "partner", row["partnerCode"])
			if e != nil {
				return nil, e
			}
			bank, e := s.excelReferenceTx(ctx, tx, "bank", row["bankNumber"])
			if e != nil {
				return nil, e
			}
			key = "account:" + account + ":" + partner + ":" + bank
			in.Lines = append(in.Lines, OpeningLine{Line: ledger.Line{AccountID: account, PartnerID: partner, Description: "Số dư đầu kỳ", Debit: row["debit"], Credit: row["credit"]}, BankAccountID: bank})
		case "stock":
			item, e := s.excelReferenceTx(ctx, tx, "item", row["itemCode"])
			if e != nil {
				return nil, e
			}
			warehouse, e := s.excelReferenceTx(ctx, tx, "warehouse", row["warehouseCode"])
			if e != nil {
				return nil, e
			}
			key = "stock:" + item + ":" + warehouse
			in.Stocks = append(in.Stocks, OpeningStock{ItemID: item, WarehouseID: warehouse, Quantity: row["quantity"], Value: row["value"]})
		case "fixed", "prepaid":
			months, e := strconv.Atoi(row["months"])
			if e != nil {
				return nil, accounting.Fail("VALIDATION", "Số tháng phân bổ không hợp lệ.")
			}
			key = "asset:" + strings.ToUpper(row["code"])
			in.Assets = append(in.Assets, Asset{ID: excelID(batch.ID, "opening-asset", row["code"]), Code: row["code"], Name: row["name"], Kind: kind, Cost: row["cost"], Allocated: row["allocated"], Residual: row["residual"], Months: months, StartPeriod: row["startPeriod"]})
		default:
			return nil, accounting.Fail("VALIDATION", "Loại dòng số dư không hợp lệ.")
		}
		if seen[key] {
			return nil, accounting.Fail("DUPLICATE_CODE", "Chi tiết số dư đã có trong nháp hoặc bị lặp trong tệp. Sửa tại biểu mẫu số dư để tránh cộng hai lần.")
		}
		seen[key] = true
	}
	result, err := s.saveOpeningTx(ctx, tx, in)
	if err != nil {
		return nil, err
	}
	opening := result.(OpeningState)
	return opening.Document, nil
}
