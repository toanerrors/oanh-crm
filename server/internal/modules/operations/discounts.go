package operations

import (
	"math/big"
	"oanh.local/accounting/internal/accounting"
	"slices"
)

func tradeDiscounts(in Input, p Policy) ([]int64, error) {
	result := make([]int64, len(in.Lines))
	values := make([]int64, len(in.Lines))
	weights := make([]*big.Rat, len(in.Lines))
	if in.Discount.Mode != "" && in.Discount.Mode != "none" && in.Discount.Mode != "line" && in.Discount.Mode != "percent" && in.Discount.Mode != "total" {
		return nil, accounting.Fail("VALIDATION", "Loại chiết khấu không hợp lệ.")
	}
	for i, line := range in.Lines {
		qty, err := accounting.Decimal(line.Quantity)
		if err != nil {
			return nil, err
		}
		price, err := accounting.Decimal(line.UnitPrice)
		if err != nil {
			return nil, err
		}
		values[i], err = accounting.Round(new(big.Rat).Mul(qty, price), p.Rounding)
		if err != nil {
			return nil, err
		}
		weights[i] = accounting.Rat(values[i])
		if in.Discount.Allocation == "quantity" {
			weights[i] = qty
		}
		if line.DiscountRate != "" && line.DiscountAmount != "" {
			return nil, accounting.Fail("VALIDATION", "Mỗi dòng chỉ nhập tỷ lệ hoặc số tiền chiết khấu.")
		}
		if in.Discount.Mode == "line" || in.Discount.Mode == "total" && in.Discount.Allocation == "manual" {
			if line.DiscountRate != "" {
				result[i], err = accounting.Percent(accounting.Rat(values[i]), line.DiscountRate, p.Rounding)
			} else if line.DiscountAmount != "" {
				result[i], err = accounting.Money(line.DiscountAmount)
			}
			if err != nil {
				return nil, err
			}
		} else if line.DiscountRate != "" || line.DiscountAmount != "" {
			return nil, accounting.Fail("VALIDATION", "Chiết khấu dòng chỉ dùng khi chọn theo mặt hàng hoặc tự nhập phân bổ.")
		}
		if in.Discount.Mode == "percent" {
			result[i], err = accounting.Percent(accounting.Rat(values[i]), in.Discount.Rate, p.Rounding)
			if err != nil {
				return nil, err
			}
		}
	}
	if in.Discount.Mode == "total" {
		total, err := accounting.Money(in.Discount.Amount)
		if err != nil {
			return nil, err
		}
		if in.Discount.Allocation == "manual" {
			sum, err := accounting.Add(result...)
			if err != nil {
				return nil, err
			}
			if sum != total {
				return nil, accounting.Fail("ALLOCATION_MISMATCH", "Tổng chiết khấu các dòng phải bằng số tiền chiết khấu hóa đơn.")
			}
		} else {
			if in.Discount.Allocation != "value" && in.Discount.Allocation != "quantity" {
				return nil, accounting.Fail("VALIDATION", "Chọn phân bổ theo giá trị, số lượng hoặc tự nhập.")
			}
			result, err = allocateTradeAmount(total, weights)
			if err != nil {
				return nil, err
			}
		}
	}
	for i, amount := range result {
		if amount > values[i] {
			return nil, accounting.Fail("DISCOUNT_EXCEEDS_VALUE", "Chiết khấu vượt thành tiền dòng; chọn lại phương pháp hoặc tự nhập phân bổ.")
		}
	}
	return result, nil
}

// Whole-dong allocations use largest remainders, with input order breaking ties.
func allocateTradeAmount(total int64, weights []*big.Rat) ([]int64, error) {
	sum := new(big.Rat)
	for _, w := range weights {
		sum.Add(sum, w)
	}
	if sum.Sign() <= 0 {
		return nil, accounting.Fail("INVALID_ALLOCATION", "Tổng tiêu thức phân bổ phải dương.")
	}
	result := make([]int64, len(weights))
	remainder := make([]*big.Rat, len(weights))
	order := make([]int, len(weights))
	left := total
	for i, weight := range weights {
		ratio := new(big.Rat).Quo(new(big.Rat).Mul(accounting.Rat(total), weight), sum)
		value := new(big.Int).Quo(ratio.Num(), ratio.Denom())
		if !value.IsInt64() {
			return nil, accounting.Fail("AMOUNT_OVERFLOW", "Phân bổ vượt giới hạn số tiền.")
		}
		result[i] = value.Int64()
		left -= result[i]
		remainder[i] = new(big.Rat).Sub(ratio, accounting.Rat(result[i]))
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int { return -remainder[a].Cmp(remainder[b]) })
	for i := int64(0); i < left; i++ {
		result[order[i]]++
	}
	return result, nil
}
