package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/platform/sqlite"
)

type Reopen struct {
	Period string `json:"period"`
	Reason string `json:"reason"`
}

func (s *Service) ReopenPeriod(ctx context.Context, cmd Command, in Reopen) (json.RawMessage, error) {
	return s.Write(ctx, cmd, "admin.period.reopen", in, func(tx *sql.Tx) (any, error) {
		if !accounting.Period(in.Period) || strings.TrimSpace(in.Reason) == "" || len(in.Reason) > 500 {
			return nil, accounting.Fail("VALIDATION", "Nhập kỳ và lý do mở lại kỳ.")
		}
		var closed bool
		if err := tx.QueryRowContext(ctx, "SELECT closed FROM periods WHERE period=?", in.Period).Scan(&closed); err == sql.ErrNoRows {
			return nil, accounting.Fail("NOT_FOUND", "Kỳ chưa được tạo.")
		} else if err != nil {
			return nil, err
		}
		if !closed {
			return nil, accounting.Fail("INVALID_PERIOD", "Kỳ đang mở.")
		}
		var later int
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM documents d WHERE d.source_type='closing' AND substr(d.date,1,7)>? AND NOT EXISTS (SELECT 1 FROM documents r WHERE r.reversal_of=d.id)", in.Period).Scan(&later); err != nil {
			return nil, err
		}
		if later > 0 {
			return nil, accounting.Fail("PERIOD_FINALIZED", "Mở lại các kỳ đã kết chuyển phía sau trước khi điều chỉnh kỳ này.")
		}
		rows, err := tx.QueryContext(ctx, "SELECT d.id FROM documents d WHERE d.source_type='closing' AND substr(d.date,1,7)=? AND NOT EXISTS (SELECT 1 FROM documents r WHERE r.reversal_of=d.id)", in.Period)
		if err != nil {
			return nil, err
		}
		ids := []string{}
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			ids = append(ids, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE periods SET closed=0 WHERE period=?", in.Period); err != nil {
			return nil, err
		}
		reversals := []Document{}
		for _, id := range ids {
			original, err := ReadDocument(ctx, tx, id)
			if err != nil {
				return nil, err
			}
			doc := Document{ID: sqlite.ID(), Type: "journal", Date: original.Date, Description: "Mở lại kỳ: " + in.Reason, SourceType: "closing_reversal", SourceID: id, ReversalOf: id, PolicyID: original.PolicyID}
			for _, line := range original.Lines {
				line.Debit, line.Credit = line.Credit, line.Debit
				doc.Lines = append(doc.Lines, line)
			}
			doc, err = s.SaveTx(ctx, tx, doc)
			if err != nil {
				return nil, err
			}
			doc, err = s.PostTx(ctx, tx, Post{ID: doc.ID, Version: doc.Version})
			if err != nil {
				return nil, err
			}
			reversals = append(reversals, doc)
		}
		return struct {
			Period    string     `json:"period"`
			Reason    string     `json:"reason"`
			Reversals []Document `json:"reversals"`
		}{in.Period, in.Reason, reversals}, nil
	})
}
