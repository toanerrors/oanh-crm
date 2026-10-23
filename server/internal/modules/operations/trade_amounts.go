package operations

import (
	"context"
	"math/big"
	"slices"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/modules/identity"
	"oanh.local/accounting/internal/modules/ledger"
)

type TradeAmounts struct {
	PolicyID      string            `json:"policyId"`
	PolicyVersion int               `json:"policyVersion"`
	Revision      int64             `json:"revision"`
	LineAmounts   []LineAmounts     `json:"lineAmounts"`
	Amounts       map[string]string `json:"amounts"`
}

func (s *Service) TradeAmounts(ctx context.Context, user identity.User, epoch string, in Input) (TradeAmounts, error) {
	var result TradeAmounts
	if user.Role == "viewer" {
		return result, accounting.Fail("FORBIDDEN", "Không có quyền lập nghiệp vụ tự động.")
	}
	if (in.Kind != "purchase" && in.Kind != "sale") || !accounting.Date(in.Date) || len(in.Lines) == 0 || len(in.Lines) > 100 || len(in.PurchaseCosts) > 100 || (in.PriceMode != "exclusive" && in.PriceMode != "inclusive") {
		return result, accounting.Fail("VALIDATION", "Chọn nghiệp vụ mua/bán, ngày, cách nhập giá và từ 1–100 dòng hàng.")
	}
	tx, err := s.Ledger.Store.Read.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	metadata, err := ledger.ReadContext(ctx, tx)
	if err != nil {
		return result, err
	}
	if metadata.Epoch != epoch {
		return result, accounting.Fail("DATASET_CHANGED", "Phiên dữ liệu đã thay đổi.")
	}
	p, err := PolicyAt(ctx, tx, in.Date)
	if err != nil {
		return result, err
	}
	if p.VATMethod == "direct" && in.PriceMode != "inclusive" {
		return result, accounting.Fail("VALIDATION", "VAT trực tiếp cần nhập giá thanh toán đã gồm thuế.")
	}
	discounts, err := tradeDiscounts(in, p)
	if err != nil {
		return result, err
	}
	result.PolicyID, result.PolicyVersion, result.Revision = p.ID, p.Version, metadata.Revision
	var sumBase, sumTax, sumGross, sumDiscount, sumPurchaseCost int64
	for i, line := range in.Lines {
		if _, err = accounting.Quantity(line.Quantity, p.QuantityScale); err != nil {
			return result, err
		}
		base, tax, gross, e := tradeLineAmounts(line, p, in.PriceMode, discounts[i])
		if e != nil {
			return result, e
		}
		result.LineAmounts = append(result.LineAmounts, LineAmounts{Base: accounting.Text(base), Tax: accounting.Text(tax), Total: accounting.Text(gross), Discount: accounting.Text(discounts[i])})
		if sumBase, err = accounting.Add(sumBase, base); err != nil {
			return result, err
		}
		if sumTax, err = accounting.Add(sumTax, tax); err != nil {
			return result, err
		}
		if sumGross, err = accounting.Add(sumGross, gross); err != nil {
			return result, err
		}
		if sumDiscount, err = accounting.Add(sumDiscount, discounts[i]); err != nil {
			return result, err
		}
	}
	for _, cost := range in.PurchaseCosts {
		amount, e := accounting.Money(cost.Amount)
		if e != nil {
			return result, e
		}
		if sumPurchaseCost, err = accounting.Add(sumPurchaseCost, amount); err != nil {
			return result, err
		}
	}
	result.Amounts = map[string]string{"base": accounting.Text(sumBase), "tax": accounting.Text(sumTax), "total": accounting.Text(sumGross), "discount": accounting.Text(sumDiscount), "purchaseCost": accounting.Text(sumPurchaseCost)}
	return result, tx.Commit()
}

func tradeLineAmounts(line TradeLine, p Policy, priceMode string, discount int64) (base, tax, gross int64, err error) {
	qty, err := accounting.Decimal(line.Quantity)
	if err != nil {
		return 0, 0, 0, err
	}
	price, err := accounting.Decimal(line.UnitPrice)
	if err != nil {
		return 0, 0, 0, err
	}
	if !slices.Contains(p.VATRates, line.VATRate) {
		return 0, 0, 0, accounting.Fail("INVALID_RATE", "Thuế suất không thuộc chính sách đang có hiệu lực.")
	}
	rate, _ := accounting.Decimal(line.VATRate)
	rate.Quo(rate, accounting.Rat(100))
	beforeDiscount, err := accounting.Round(new(big.Rat).Mul(qty, price), p.Rounding)
	if err != nil {
		return 0, 0, 0, err
	}
	if discount > beforeDiscount {
		return 0, 0, 0, accounting.Fail("DISCOUNT_EXCEEDS_VALUE", "Chiết khấu không được vượt thành tiền của dòng.")
	}
	value := accounting.Rat(beforeDiscount - discount)
	if priceMode == "inclusive" {
		gross, err = accounting.Round(value, p.Rounding)
		if err != nil {
			return 0, 0, 0, err
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
	return base, tax, gross, err
}
