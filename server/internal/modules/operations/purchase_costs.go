package operations

import (
	"context"
	"database/sql"
	"encoding/json"
	"math/big"
	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/modules/ledger"
)

type PurchaseCostSource struct {
	DocumentID  string `json:"documentId"`
	Reference   string `json:"reference"`
	Date        string `json:"date"`
	Line        int    `json:"line"`
	Description string `json:"description"`
	AccountID   string `json:"accountId"`
	Amount      string `json:"amount"`
	Remaining   string `json:"remaining"`
}
type costKey struct {
	document string
	line     int
}

func costSources(ctx context.Context, q ledger.Queryer, asOf string) ([]PurchaseCostSource, error) {
	rows, err := q.QueryContext(ctx, `SELECT o.document_id,d.reference,o.date,o.payload,o.calculation FROM operations o JOIN documents d ON d.id=o.document_id WHERE o.kind='purchase' AND o.date<=? AND d.status='posted' ORDER BY o.date,d.posted_order`, asOf)
	if err != nil {
		return nil, err
	}
	sources := []PurchaseCostSource{}
	used := map[costKey]*big.Int{}
	for rows.Next() {
		var id, reference, date, payload, calculation string
		if err = rows.Scan(&id, &reference, &date, &payload, &calculation); err != nil {
			rows.Close()
			return nil, err
		}
		var input Input
		var result Result
		if err = json.Unmarshal([]byte(payload), &input); err != nil {
			rows.Close()
			return nil, err
		}
		for _, cost := range input.PurchaseCosts {
			n, e := accounting.Money(cost.Amount)
			if e != nil {
				rows.Close()
				return nil, e
			}
			key := costKey{cost.DocumentID, cost.Line}
			if used[key] == nil {
				used[key] = new(big.Int)
			}
			used[key].Add(used[key], big.NewInt(n))
		}
		if !input.IsPurchaseCost {
			continue
		}
		if err = json.Unmarshal([]byte(calculation), &result); err != nil {
			rows.Close()
			return nil, err
		}
		if len(result.LineAmounts) != len(input.Lines) {
			rows.Close()
			return nil, accounting.Fail("INVALID_COST_SOURCE", "Thiếu kết quả tính của chứng từ chi phí.")
		}
		for i, line := range input.Lines {
			amount, e := accounting.Money(result.LineAmounts[i].PurchaseValue)
			if e != nil {
				rows.Close()
				return nil, e
			}
			account := line.AccountID
			if account == "" {
				account = "642"
			}
			sources = append(sources, PurchaseCostSource{DocumentID: id, Reference: reference, Date: date, Line: i, Description: line.Description, AccountID: account, Amount: accounting.Text(amount)})
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for i := range sources {
		source := &sources[i]
		available, _ := accounting.Money(source.Amount)
		balance := big.NewInt(available)
		if n := used[costKey{source.DocumentID, source.Line}]; n != nil {
			balance.Sub(balance, n)
		}
		if balance.Sign() < 0 {
			return nil, accounting.Fail("COST_OVERALLOCATED", "Chi phí mua hàng đã bị phân bổ vượt nguồn.")
		}
		source.Remaining = balance.String()
	}
	return sources, nil
}
func (s *Service) PurchaseCostSources(ctx context.Context, asOf string) ([]PurchaseCostSource, error) {
	if !accounting.Date(asOf) {
		return nil, accounting.Fail("VALIDATION", "Ngày tra cứu chi phí không hợp lệ.")
	}
	tx, e := s.Ledger.Store.Read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	return costSources(ctx, tx, asOf)
}
func purchaseCosts(ctx context.Context, tx *sql.Tx, in Input, policy Policy, discounts []int64) ([]int64, []ledger.Line, error) {
	amounts := make([]int64, len(in.Lines))
	lines := []ledger.Line{}
	if in.IsPurchaseCost && (in.Kind != "purchase" || in.TradeMode != "service" || len(in.PurchaseCosts) > 0) {
		return nil, nil, accounting.Fail("INVALID_COST_SOURCE", "Đánh dấu chi phí mua hàng trên chứng từ mua dịch vụ riêng.")
	}
	var distributed int64
	for i, line := range in.Lines {
		if line.PurchaseCost != "" {
			n, e := accounting.Money(line.PurchaseCost)
			if e != nil {
				return nil, nil, e
			}
			amounts[i] = n
			distributed, e = accounting.Add(distributed, n)
			if e != nil {
				return nil, nil, e
			}
		}
	}
	if len(in.PurchaseCosts) == 0 && distributed == 0 {
		return amounts, lines, nil
	}
	if in.Kind != "purchase" || in.TradeMode != "goods" || len(in.PurchaseCosts) > 100 {
		return nil, nil, accounting.Fail("INVALID_COST_ALLOCATION", "Chỉ phân bổ chi phí vào chứng từ mua hàng nhập kho.")
	}
	sources, err := costSources(ctx, tx, in.Date)
	if err != nil {
		return nil, nil, err
	}
	byKey := map[costKey]PurchaseCostSource{}
	for _, source := range sources {
		byKey[costKey{source.DocumentID, source.Line}] = source
	}
	seen := map[costKey]bool{}
	var selected int64
	for _, cost := range in.PurchaseCosts {
		key := costKey{cost.DocumentID, cost.Line}
		source, ok := byKey[key]
		if !ok || seen[key] {
			return nil, nil, accounting.Fail("INVALID_COST_SOURCE", "Nguồn chi phí không hợp lệ hoặc bị chọn trùng.")
		}
		seen[key] = true
		n, e := accounting.Money(cost.Amount)
		if e != nil {
			return nil, nil, e
		}
		remaining, _ := accounting.Money(source.Remaining)
		if n <= 0 || n > remaining {
			return nil, nil, accounting.Fail("COST_OVERALLOCATED", "Số phân bổ phải dương và không vượt chi phí còn lại.")
		}
		selected, e = accounting.Add(selected, n)
		if e != nil {
			return nil, nil, e
		}
		lines = append(lines, ledger.Line{AccountID: source.AccountID, Description: "Phân bổ chi phí " + source.Reference, Debit: "0", Credit: cost.Amount})
	}

	if in.CostAllocation != "" && in.CostAllocation != "manual" {
		if distributed != 0 {
			return nil, nil, accounting.Fail("INVALID_COST_ALLOCATION", "Không nhập tiền phân bổ từng dòng khi chọn phân bổ tự động.")
		}
		if in.CostAllocation != "quantity" && in.CostAllocation != "value" && in.CostAllocation != "percent" {
			return nil, nil, accounting.Fail("INVALID_COST_ALLOCATION", "Phương pháp phân bổ chi phí không hợp lệ.")
		}
		weights := make([]*big.Rat, len(in.Lines))
		percentTotal := new(big.Rat)
		for i, line := range in.Lines {
			qty, e := accounting.Decimal(line.Quantity)
			if e != nil {
				return nil, nil, e
			}
			weights[i] = qty
			if in.CostAllocation == "percent" {
				weights[i], e = accounting.Decimal(line.CostPercent)
				if e != nil {
					return nil, nil, e
				}
				percentTotal.Add(percentTotal, weights[i])
			}
			if in.CostAllocation == "value" {
				price, e := accounting.Decimal(line.UnitPrice)
				if e != nil {
					return nil, nil, e
				}
				value, e := accounting.Round(new(big.Rat).Mul(qty, price), policy.Rounding)
				if e != nil {
					return nil, nil, e
				}
				value -= discounts[i]
				if value < 0 {
					return nil, nil, accounting.Fail("DISCOUNT_EXCEEDS_VALUE", "Chiết khấu vượt giá trị dòng.")
				}
				if in.PriceMode == "inclusive" {
					rate, e := accounting.Decimal(line.VATRate)
					if e != nil {
						return nil, nil, e
					}
					if policy.VATMethod == "direct" {
						tax, e := accounting.Percent(accounting.Rat(value), line.VATRate, policy.Rounding)
						if e != nil {
							return nil, nil, e
						}
						value -= tax
					} else {
						rate.Quo(rate, accounting.Rat(100))
						value, e = accounting.Round(new(big.Rat).Quo(accounting.Rat(value), new(big.Rat).Add(accounting.Rat(1), rate)), policy.Rounding)
						if e != nil {
							return nil, nil, e
						}
					}
				}
				weights[i] = accounting.Rat(value)
			}
		}
		if in.CostAllocation == "percent" && percentTotal.Cmp(accounting.Rat(100)) != 0 {
			return nil, nil, accounting.Fail("ALLOCATION_MISMATCH", "Tổng tỷ lệ phân bổ chi phí phải bằng 100%.")
		}
		var e error
		amounts, e = allocateTradeAmount(selected, weights)
		if e != nil {
			return nil, nil, e
		}
		distributed = selected
	}
	if selected != distributed {
		return nil, nil, accounting.Fail("ALLOCATION_MISMATCH", "Tổng chi phí phân bổ từng dòng hàng phải bằng tổng chi phí chọn từ chứng từ nguồn.")
	}
	return amounts, lines, nil
}
