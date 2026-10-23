package operations

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/modules/ledger"
	"oanh.local/accounting/internal/platform/sqlite"
)

func (s *Service) trade(ctx context.Context, tx *sql.Tx, in Input, p Policy, doc *ledger.Document, result *Result) error {
	if len(in.InvoiceNumber) > 100 || len(in.InvoiceSeries) > 100 || in.InvoiceNumber != strings.TrimSpace(in.InvoiceNumber) || in.InvoiceSeries != strings.TrimSpace(in.InvoiceSeries) || in.InvoiceDate != "" && !accounting.Date(in.InvoiceDate) || in.DueDate != "" && !accounting.Date(in.DueDate) || in.InvoiceNumber != "" && in.InvoiceDate == "" || in.InvoiceNumber == "" && (in.InvoiceDate != "" || in.InvoiceSeries != "") {
		return accounting.Fail("VALIDATION", "Thông tin hóa đơn không hợp lệ. Khi nhập số hóa đơn cần nhập cả ngày hóa đơn.")
	}
	if in.InvoiceDate != "" && in.DueDate != "" && in.DueDate < in.InvoiceDate {
		return accounting.Fail("VALIDATION", "Hạn thanh toán không được trước ngày hóa đơn.")
	}
	if in.InvoiceNumber != "" {
		var duplicate int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM operations WHERE kind=? AND json_extract(payload,'$.invoiceNumber')=? AND COALESCE(json_extract(payload,'$.invoiceSeries'),'')=? AND (?='sale' OR json_extract(payload,'$.partnerId')=?)`, in.Kind, in.InvoiceNumber, in.InvoiceSeries, in.Kind, in.PartnerID).Scan(&duplicate); err != nil {
			return err
		}
		if duplicate > 0 {
			return accounting.Fail("DUPLICATE_INVOICE", "Hóa đơn cùng bên phát hành, ký hiệu và số đã được ghi nhận. Kiểm tra chứng từ nguồn.")
		}
	}
	if in.TradeMode != "" && in.TradeMode != "goods" && in.TradeMode != "service" {
		return accounting.Fail("VALIDATION", "Loại hàng hóa/dịch vụ không hợp lệ.")
	}
	if in.PaymentAccount != "" && !moneyAccount(in.PaymentAccount) && in.BankAccountID == "" {
		return accounting.Fail("INVALID_ACCOUNT", "Chọn tài khoản tiền được hỗ trợ khi thanh toán ngay.")
	}
	if len(in.Lines) == 0 || len(in.Lines) > 100 || in.PriceMode != "exclusive" && in.PriceMode != "inclusive" {
		return accounting.Fail("VALIDATION", "Nhập từ 1–100 dòng hàng và chọn giá trước/sau thuế.")
	}
	var role string
	var active bool
	if err := tx.QueryRowContext(ctx, "SELECT role,active FROM partners WHERE id=?", in.PartnerID).Scan(&role, &active); err != nil || !active || in.Kind == "sale" && role == "supplier" || in.Kind == "purchase" && role == "customer" {
		return accounting.Fail("INVALID_PARTNER", "Chọn khách hàng/nhà cung cấp phù hợp và còn hiệu lực.")
	}
	if p.VATMethod == "direct" && in.PriceMode != "inclusive" {
		return accounting.Fail("VALIDATION", "VAT trực tiếp cần nhập giá thanh toán đã gồm thuế.")
	}
	discounts, err := tradeDiscounts(in, p)
	if err != nil {
		return err
	}
	costs, costLines, err := purchaseCosts(ctx, tx, in, p, discounts)
	if err != nil {
		return err
	}
	doc.Type = in.Kind
	var sumBase, sumTax, sumGross, sumCost, inputTax, sumDiscount, sumPurchaseCost int64
	for index, line := range in.Lines {
		var item Item
		err := tx.QueryRowContext(ctx, "SELECT id,name,tracked,active FROM items WHERE id=?", line.ItemID).Scan(&item.ID, &item.Name, &item.Tracked, &item.Active)
		if err != nil || !item.Active {
			return accounting.Fail("INVALID_ITEM", "Hàng hóa/dịch vụ không tồn tại hoặc đã ngừng sử dụng.")
		}
		if in.TradeMode == "service" && item.Tracked || in.TradeMode == "goods" && !item.Tracked {
			return accounting.Fail("INVALID_ITEM", "Chọn hàng hóa/dịch vụ đúng loại chứng từ.")
		}
		description := item.Name
		if line.Description != "" {
			description = line.Description
		}
		account := line.AccountID
		if account != "" {
			var group string
			var active bool
			if e := tx.QueryRowContext(ctx, "SELECT account_group,active FROM accounts WHERE id=?", account).Scan(&group, &active); e != nil || !active || in.Kind == "sale" && group != "revenue" || in.Kind == "purchase" && !(item.Tracked && account == "156" || group == "expense" || group == "wip") {
				return accounting.Fail("INVALID_ACCOUNT", "Tài khoản hàng tiền không phù hợp loại nghiệp vụ.")
			}
		}
		quantity, err := accounting.Quantity(line.Quantity, p.QuantityScale)
		if err != nil {
			return err
		}
		base, tax, gross, err := tradeLineAmounts(line, p, in.PriceMode, discounts[index])
		if err != nil {
			return err
		}
		if sumDiscount, err = accounting.Add(sumDiscount, discounts[index]); err != nil {
			return err
		}
		if sumPurchaseCost, err = accounting.Add(sumPurchaseCost, costs[index]); err != nil {
			return err
		}
		if sumBase, err = accounting.Add(sumBase, base); err != nil {
			return err
		}
		if sumTax, err = accounting.Add(sumTax, tax); err != nil {
			return err
		}
		if sumGross, err = accounting.Add(sumGross, gross); err != nil {
			return err
		}
		result.LineAmounts = append(result.LineAmounts, LineAmounts{Base: accounting.Text(base), Tax: accounting.Text(tax), Total: accounting.Text(gross), Discount: accounting.Text(discounts[index]), PurchaseCost: accounting.Text(costs[index])})
		if in.Kind == "sale" {
			if account == "" {
				account = "511"
			}
			addLine(doc, "131", in.PartnerID, description, gross, 0)
			addLine(doc, account, "", description, 0, base)
			addLine(doc, "33311", "", "Thuế GTGT đầu ra", 0, tax)
			if item.Tracked {
				cost, e := s.stock(ctx, tx, doc, item.ID, quantity, 0, p, index, false, line.WarehouseID)
				if e != nil {
					return e
				}
				if sumCost, e = accounting.Add(sumCost, cost); e != nil {
					return e
				}
				addLine(doc, "632", "", "Giá vốn: "+item.Name, cost, 0)
				addLine(doc, "156", "", "Xuất kho: "+item.Name, 0, cost)
			}
		} else {
			if p.VATMethod == "deduction" && line.Deductible == nil {
				return accounting.Fail("VAT_ELIGIBILITY_REQUIRED", "Xác nhận VAT đầu vào có đủ điều kiện khấu trừ trên từng dòng mua hàng.")
			}
			eligible := p.VATMethod == "deduction" && line.Deductible != nil && *line.Deductible
			purchaseCost := base
			if !eligible {
				purchaseCost = gross
			}
			result.LineAmounts[index].PurchaseValue = accounting.Text(purchaseCost)
			if account == "" {
				account = "642"
				if item.Tracked {
					account = "156"
				}
			}
			if in.IsPurchaseCost && item.Tracked {
				return accounting.Fail("INVALID_COST_SOURCE", "Chứng từ chi phí mua hàng chỉ gồm dịch vụ.")
			}
			if costs[index] > 0 && (!item.Tracked || account != "156") {
				return accounting.Fail("INVALID_COST_ALLOCATION", "Chi phí mua hàng chỉ phân bổ vào hàng nhập kho.")
			}
			if purchaseCost, err = accounting.Add(purchaseCost, costs[index]); err != nil {
				return err
			}
			if item.Tracked && account == "156" {
				if _, err = s.stock(ctx, tx, doc, item.ID, quantity, purchaseCost, p, index, true, line.WarehouseID); err != nil {
					return err
				}
			}
			addLine(doc, account, "", description, purchaseCost, 0)
			if eligible {
				addLine(doc, "1331", "", "Thuế GTGT đầu vào", tax, 0)
				inputTax, err = accounting.Add(inputTax, tax)
				if err != nil {
					return err
				}
			}
			addLine(doc, "331", in.PartnerID, description, 0, gross)
		}
	}
	if sumGross <= 0 {
		return accounting.Fail("INVALID_AMOUNT", "Tổng tiền thanh toán phải dương; hàng tặng cần nghiệp vụ riêng.")
	}
	doc.Lines = append(doc.Lines, costLines...)
	result.Amounts = map[string]string{"discount": accounting.Text(sumDiscount), "purchaseCost": accounting.Text(sumPurchaseCost), "base": accounting.Text(sumBase), "tax": accounting.Text(sumTax), "total": accounting.Text(sumGross), "cost": accounting.Text(sumCost), "inputTax": accounting.Text(inputTax)}
	result.Notes = append(result.Notes, "Làm tròn tiền theo từng dòng; tổng chứng từ bằng tổng các dòng đã chốt.")
	return nil
}

type lot struct {
	id              string
	quantity, value int64
	scale           int
}

func (s *Service) stock(ctx context.Context, tx *sql.Tx, doc *ledger.Document, item string, quantity, value int64, p Policy, index int, receipt bool, warehouse string) (int64, error) {
	if warehouse == "" {
		warehouse = "default"
	}
	var active bool
	if e := tx.QueryRowContext(ctx, "SELECT active FROM warehouses WHERE id=?", warehouse).Scan(&active); e != nil || !active {
		return 0, accounting.Fail("INVALID_WAREHOUSE", "Kho không tồn tại hoặc đã ngừng sử dụng.")
	}
	var last sql.NullString
	if err := tx.QueryRowContext(ctx, "SELECT MAX(m.date) FROM stock_movements m JOIN stock_movement_warehouses w ON w.movement_id=m.id WHERE m.item_id=? AND w.warehouse_id=?", item, warehouse).Scan(&last); err != nil {
		return 0, err
	}
	if last.Valid && doc.Date < last.String {
		return 0, accounting.Fail("RETROACTIVE_STOCK", "Chưa hỗ trợ biến động kho lùi trước ngày biến động cuối. Cần nghiệp vụ điều chỉnh riêng.")
	}
	rows, err := tx.QueryContext(ctx, "SELECT l.id,l.quantity,l.value,l.scale,COALESCE(p.data,'') FROM stock_lots l LEFT JOIN documents d ON d.id=l.document_id LEFT JOIN policies p ON p.id=d.policy_id JOIN stock_lot_warehouses w ON w.lot_id=l.id WHERE l.item_id=? AND w.warehouse_id=? AND quantity>0 ORDER BY l.date,CASE WHEN d.source_type IN ('opening','opening_adjustment') THEN 0 ELSE 1 END,COALESCE(d.posted_order,9223372036854775807),l.id", item, warehouse)
	if err != nil {
		return 0, err
	}
	lots := []lot{}
	var totalQty, totalValue int64
	for rows.Next() {
		var l lot
		var policyData string
		if err = rows.Scan(&l.id, &l.quantity, &l.value, &l.scale, &policyData); err != nil {
			rows.Close()
			return 0, err
		}
		if policyData != "" {
			var original Policy
			if err = json.Unmarshal([]byte(policyData), &original); err != nil {
				rows.Close()
				return 0, err
			}
			if original.InventoryMethod != p.InventoryMethod {
				rows.Close()
				return 0, accounting.Fail("STOCK_POLICY_LOCKED", "Phải xử lý hết tồn theo phương pháp cũ trước ngày hiệu lực phương pháp mới.")
			}
		}
		if l.scale != p.QuantityScale {
			rows.Close()
			return 0, accounting.Fail("STOCK_POLICY_LOCKED", "Scale số lượng khác lịch sử tồn kho.")
		}
		totalQty, err = accounting.Add(totalQty, l.quantity)
		if err != nil {
			rows.Close()
			return 0, err
		}
		totalValue, err = accounting.Add(totalValue, l.value)
		if err != nil {
			rows.Close()
			return 0, err
		}
		lots = append(lots, l)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	if receipt {
		if _, err = accounting.Add(totalQty, quantity); err != nil {
			return 0, err
		}
		if _, err = accounting.Add(totalValue, value); err != nil {
			return 0, err
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO stock_lots VALUES(?,?,?,?,?,?,?)", fmt.Sprintf("%s-%06d", doc.ID, index), item, doc.ID, doc.Date, quantity, value, p.QuantityScale)
		if err != nil {
			return 0, err
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO stock_lot_warehouses VALUES(?,?)", fmt.Sprintf("%s-%06d", doc.ID, index), warehouse); err != nil {
			return 0, err
		}
		err = insertStockMovement(ctx, tx, item, doc, quantity, value, p.QuantityScale, warehouse)
		return value, err
	}
	if totalQty < quantity {
		return 0, accounting.Fail("INSUFFICIENT_STOCK", "Không đủ tồn kho. Chính sách hiện tại không cho âm kho.")
	}
	var cost int64
	if p.InventoryMethod == "moving_average" {
		cost, err = accounting.Round(new(big.Rat).SetFrac(new(big.Int).Mul(big.NewInt(totalValue), big.NewInt(quantity)), big.NewInt(totalQty)), p.Rounding)
		if quantity == totalQty {
			cost = totalValue
		}
		if err != nil {
			return 0, err
		}
		if _, err = tx.ExecContext(ctx, "DELETE FROM stock_lots WHERE item_id=? AND id IN (SELECT lot_id FROM stock_lot_warehouses WHERE warehouse_id=?)", item, warehouse); err != nil {
			return 0, err
		}
		if totalQty > quantity {
			lotID := sqlite.ID()
			if _, err = tx.ExecContext(ctx, "INSERT INTO stock_lots VALUES(?,?,?,?,?,?,?)", lotID, item, doc.ID, doc.Date, totalQty-quantity, totalValue-cost, p.QuantityScale); err != nil {
				return 0, err
			}
			if _, err = tx.ExecContext(ctx, "INSERT INTO stock_lot_warehouses VALUES(?,?)", lotID, warehouse); err != nil {
				return 0, err
			}
		}
	} else {
		remaining := quantity
		for _, l := range lots {
			if remaining == 0 {
				break
			}
			take := min(l.quantity, remaining)
			piece := l.value
			if take != l.quantity {
				piece, err = accounting.Round(new(big.Rat).SetFrac(new(big.Int).Mul(big.NewInt(l.value), big.NewInt(take)), big.NewInt(l.quantity)), p.Rounding)
				if err != nil {
					return 0, err
				}
			}
			cost, err = accounting.Add(cost, piece)
			if err != nil {
				return 0, err
			}
			if _, err = tx.ExecContext(ctx, "UPDATE stock_lots SET quantity=quantity-?,value=value-? WHERE id=?", take, piece, l.id); err != nil {
				return 0, err
			}
			remaining -= take
		}
	}
	err = insertStockMovement(ctx, tx, item, doc, -quantity, -cost, p.QuantityScale, warehouse)
	return cost, err
}
