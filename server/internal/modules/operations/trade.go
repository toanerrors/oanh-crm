package operations

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"slices"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/modules/ledger"
	"oanh.local/accounting/internal/platform/sqlite"
)

func (s *Service) trade(ctx context.Context, tx *sql.Tx, in Input, p Policy, doc *ledger.Document, result *Result) error {
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
	doc.Type = in.Kind
	var sumBase, sumTax, sumGross, sumCost, inputTax int64
	for index, line := range in.Lines {
		var item Item
		err := tx.QueryRowContext(ctx, "SELECT id,name,tracked,active FROM items WHERE id=?", line.ItemID).Scan(&item.ID, &item.Name, &item.Tracked, &item.Active)
		if err != nil || !item.Active {
			return accounting.Fail("INVALID_ITEM", "Hàng hóa/dịch vụ không tồn tại hoặc đã ngừng sử dụng.")
		}
		quantity, err := accounting.Quantity(line.Quantity, p.QuantityScale)
		if err != nil {
			return err
		}
		qtyRat, _ := accounting.Decimal(line.Quantity)
		price, err := accounting.Decimal(line.UnitPrice)
		if err != nil {
			return err
		}
		if !slices.Contains(p.VATRates, line.VATRate) {
			return accounting.Fail("INVALID_RATE", "Thuế suất không thuộc chính sách đang có hiệu lực.")
		}
		rate, _ := accounting.Decimal(line.VATRate)
		rate.Quo(rate, accounting.Rat(100))
		value := new(big.Rat).Mul(qtyRat, price)
		var base, tax, gross int64
		if in.PriceMode == "inclusive" {
			gross, err = accounting.Round(value, p.Rounding)
			if err != nil {
				return err
			}
			if p.VATMethod == "direct" {
				tax, err = accounting.Percent(accounting.Rat(gross), line.VATRate, p.Rounding)
				base = gross - tax
			} else {
				base, err = accounting.Round(new(big.Rat).Quo(accounting.Rat(gross), new(big.Rat).Add(accounting.Rat(1), rate)), p.Rounding)
				tax = gross - base
			}
		} else {
			base, err = accounting.Round(value, p.Rounding)
			if err == nil {
				tax, err = accounting.Percent(accounting.Rat(base), line.VATRate, p.Rounding)
			}
			if err == nil {
				gross, err = accounting.Add(base, tax)
			}
		}
		if err != nil {
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
		if in.Kind == "sale" {
			addLine(doc, "131", in.PartnerID, item.Name, gross, 0)
			addLine(doc, "511", "", item.Name, 0, base)
			addLine(doc, "33311", "", "Thuế GTGT đầu ra", 0, tax)
			if item.Tracked {
				cost, e := s.stock(ctx, tx, doc, item.ID, quantity, 0, p, index, false)
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
			account := "642"
			if item.Tracked {
				account = "156"
				if _, err = s.stock(ctx, tx, doc, item.ID, quantity, purchaseCost, p, index, true); err != nil {
					return err
				}
			}
			addLine(doc, account, "", item.Name, purchaseCost, 0)
			if eligible {
				addLine(doc, "1331", "", "Thuế GTGT đầu vào", tax, 0)
				inputTax, err = accounting.Add(inputTax, tax)
				if err != nil {
					return err
				}
			}
			addLine(doc, "331", in.PartnerID, item.Name, 0, gross)
		}
	}
	result.Amounts = map[string]string{"base": accounting.Text(sumBase), "tax": accounting.Text(sumTax), "total": accounting.Text(sumGross), "cost": accounting.Text(sumCost), "inputTax": accounting.Text(inputTax)}
	result.Notes = append(result.Notes, "Làm tròn tiền theo từng dòng; tổng chứng từ bằng tổng các dòng đã chốt.")
	return nil
}

type lot struct {
	id              string
	quantity, value int64
	scale           int
}

func (s *Service) stock(ctx context.Context, tx *sql.Tx, doc *ledger.Document, item string, quantity, value int64, p Policy, index int, receipt bool) (int64, error) {
	var last sql.NullString
	if err := tx.QueryRowContext(ctx, "SELECT MAX(date) FROM stock_movements WHERE item_id=?", item).Scan(&last); err != nil {
		return 0, err
	}
	if last.Valid && doc.Date < last.String {
		return 0, accounting.Fail("RETROACTIVE_STOCK", "Chưa hỗ trợ biến động kho lùi trước ngày biến động cuối. Cần nghiệp vụ điều chỉnh riêng.")
	}
	rows, err := tx.QueryContext(ctx, "SELECT l.id,l.quantity,l.value,l.scale,COALESCE(p.data,'') FROM stock_lots l LEFT JOIN documents d ON d.id=l.document_id LEFT JOIN policies p ON p.id=d.policy_id WHERE item_id=? AND quantity>0 ORDER BY l.date,COALESCE(d.posted_order,9223372036854775807),l.id", item)
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
		_, err = tx.ExecContext(ctx, "INSERT INTO stock_movements(item_id,document_id,date,quantity,value,scale) VALUES(?,?,?,?,?,?)", item, doc.ID, doc.Date, quantity, value, p.QuantityScale)
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
		if _, err = tx.ExecContext(ctx, "DELETE FROM stock_lots WHERE item_id=?", item); err != nil {
			return 0, err
		}
		if totalQty > quantity {
			if _, err = tx.ExecContext(ctx, "INSERT INTO stock_lots VALUES(?,?,?,?,?,?,?)", sqlite.ID(), item, doc.ID, doc.Date, totalQty-quantity, totalValue-cost, p.QuantityScale); err != nil {
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
	_, err = tx.ExecContext(ctx, "INSERT INTO stock_movements(item_id,document_id,date,quantity,value,scale) VALUES(?,?,?,?,?,?)", item, doc.ID, doc.Date, -quantity, -cost, p.QuantityScale)
	return cost, err
}
