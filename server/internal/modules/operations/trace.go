package operations

import (
	"context"
	"database/sql"
	"encoding/json"

	"oanh.local/accounting/internal/accounting"
)

type Trace struct {
	Input       Input  `json:"input"`
	Calculation Result `json:"calculation"`
	Policy      Policy `json:"policy"`
}

func (s *Service) Trace(ctx context.Context, documentID string) (Trace, error) {
	var result Trace
	tx, err := s.Ledger.Store.Read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	var input, calculation, policy string
	err = tx.QueryRowContext(ctx, "SELECT o.payload,o.calculation,p.data FROM operations o JOIN policies p ON p.id=o.policy_id WHERE o.document_id=?", documentID).Scan(&input, &calculation, &policy)
	if err == sql.ErrNoRows {
		return result, accounting.Fail("NOT_FOUND", "Chứng từ không có phép tính tự động.")
	}
	if err != nil {
		return result, err
	}
	for _, part := range []struct {
		data  string
		value any
	}{{input, &result.Input}, {calculation, &result.Calculation}, {policy, &result.Policy}} {
		if err = json.Unmarshal([]byte(part.data), part.value); err != nil {
			return result, err
		}
	}
	return result, tx.Commit()
}
