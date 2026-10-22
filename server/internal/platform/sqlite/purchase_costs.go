package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"oanh.local/accounting/internal/accounting"
)

func verifyPurchaseCosts(ctx context.Context, db *sql.DB) error {
	type key struct {
		document string
		line     int
	}
	type source struct {
		date  string
		value *big.Int
	}
	sources := map[key]source{}
	used := map[key]*big.Int{}
	dates := map[key]string{}
	rows, e := db.QueryContext(ctx, "SELECT document_id,date,payload,calculation FROM operations WHERE kind='purchase' ORDER BY date")
	if e != nil {
		return e
	}
	defer rows.Close()
	for rows.Next() {
		var id, date, payload, calculation string
		if e = rows.Scan(&id, &date, &payload, &calculation); e != nil {
			return e
		}
		var input struct {
			CostAllocation string `json:"costAllocation"`
			IsPurchaseCost bool   `json:"isPurchaseCost"`
			TradeMode      string `json:"tradeMode"`
			Lines          []struct {
				PurchaseCost string `json:"purchaseCost"`
			} `json:"lines"`
			Costs []struct {
				DocumentID string `json:"documentId"`
				Line       int    `json:"line"`
				Amount     string `json:"amount"`
			} `json:"purchaseCosts"`
		}
		var result struct {
			Lines []struct {
				Value string `json:"purchaseValue"`
				Cost  string `json:"purchaseCost"`
			} `json:"lineAmounts"`
		}
		if e = json.Unmarshal([]byte(payload), &input); e != nil {
			return e
		}
		if e = json.Unmarshal([]byte(calculation), &result); e != nil {
			return e
		}
		if input.IsPurchaseCost {
			if input.TradeMode != "service" || len(input.Costs) > 0 || len(input.Lines) != len(result.Lines) {
				return fmt.Errorf("invalid purchase cost source")
			}
			for i, line := range result.Lines {
				n, err := accounting.Money(line.Value)
				if err != nil {
					return err
				}
				sources[key{id, i}] = source{date, big.NewInt(n)}
			}
		}
		selected, distributed := new(big.Int), new(big.Int)
		seen := map[key]bool{}
		for _, cost := range input.Costs {
			k := key{cost.DocumentID, cost.Line}
			if seen[k] || cost.Line < 0 {
				return fmt.Errorf("duplicate purchase cost source")
			}
			seen[k] = true
			n, err := accounting.Money(cost.Amount)
			if err != nil || n <= 0 {
				return fmt.Errorf("invalid allocated purchase cost")
			}
			if used[k] == nil {
				used[k] = new(big.Int)
				dates[k] = date
			}
			if date < dates[k] {
				dates[k] = date
			}
			used[k].Add(used[k], big.NewInt(n))
			selected.Add(selected, big.NewInt(n))
		}
		for _, line := range input.Lines {
			if line.PurchaseCost != "" {
				n, err := accounting.Money(line.PurchaseCost)
				if err != nil {
					return err
				}
				distributed.Add(distributed, big.NewInt(n))
			}
		}

		if input.CostAllocation != "" && input.CostAllocation != "manual" {
			distributed = new(big.Int)
			for _, line := range result.Lines {
				n, err := accounting.Money(line.Cost)
				if err != nil {
					return err
				}
				distributed.Add(distributed, big.NewInt(n))
			}
		}
		if selected.Cmp(distributed) != 0 || selected.Sign() > 0 && input.TradeMode != "goods" {
			return fmt.Errorf("purchase cost source and distribution differ")
		}
	}
	if e = rows.Err(); e != nil {
		return e
	}
	for k, n := range used {
		src, ok := sources[k]
		if !ok || n.Cmp(src.value) > 0 || src.date > dates[k] {
			return fmt.Errorf("purchase cost exceeds available source")
		}
	}
	return nil
}
