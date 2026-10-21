package ledger

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"io"
	"strings"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/platform/sqlite"
)

type ImportInput struct {
	CSV              string `json:"csv"`
	ExpectedRevision *int64 `json:"expectedRevision,omitempty"`
}
type ImportResult struct {
	Documents []Document `json:"documents"`
	Revision  int64      `json:"revision"`
}

var importHeader = []string{"sourceType", "sourceId", "type", "date", "description", "partnerId", "accountId", "linePartnerId", "lineDescription", "debit", "credit"}

func parseImport(text string) ([]Document, error) {
	reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(text, "\ufeff")))
	header, err := reader.Read()
	if err != nil {
		return nil, accounting.Fail("INVALID_IMPORT", "Không đọc được tiêu đề CSV.")
	}
	if strings.Join(header, ",") != strings.Join(importHeader, ",") {
		return nil, accounting.Fail("INVALID_IMPORT", "Tiêu đề CSV phải đúng mẫu nhập chứng từ.")
	}
	reader.FieldsPerRecord = len(importHeader)
	documents := []Document{}
	indices := map[string]int{}
	rows := 0
	for {
		row, e := reader.Read()
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, accounting.Fail("INVALID_IMPORT", "CSV có dòng sai cấu trúc hoặc dấu nháy.")
		}
		rows++
		if rows > 5000 {
			return nil, accounting.Fail("IMPORT_LIMIT", "Mỗi lần nhập tối đa 5.000 dòng.")
		}
		if reservedSource(row[0]) || !accounting.ID(row[0]) || !accounting.ID(row[1]) {
			return nil, accounting.Fail("INVALID_IMPORT", "Mỗi dòng cần mã nguồn và loại nguồn hợp lệ.")
		}
		source, e := manualSource(row[2], row[0])
		if e != nil {
			return nil, e
		}
		row[0] = source
		key := row[0] + "\x00" + row[1]
		index, exists := indices[key]
		if !exists {
			index = len(documents)
			indices[key] = index
			documents = append(documents, Document{ID: sqlite.ID(), SourceType: row[0], SourceID: row[1], Type: row[2], Date: row[3], Description: row[4], PartnerID: row[5], Lines: []Line{}})
		}
		d := &documents[index]
		if d.Type != row[2] || d.Date != row[3] || d.Description != row[4] || d.PartnerID != row[5] {
			return nil, accounting.Fail("INVALID_IMPORT", "Các dòng cùng nguồn phải có cùng loại, ngày, diễn giải và đối tượng chung.")
		}
		d.Lines = append(d.Lines, Line{AccountID: row[6], PartnerID: row[7], Description: row[8], Debit: row[9], Credit: row[10]})
	}
	if len(documents) == 0 || len(documents) > 500 {
		return nil, accounting.Fail("IMPORT_LIMIT", "Mỗi lần nhập từ 1–500 chứng từ nháp.")
	}
	return documents, nil
}
func (s *Service) PreviewImport(ctx context.Context, cmd Command, input ImportInput) (ImportResult, error) {
	r := ImportResult{}
	documents, err := parseImport(input.CSV)
	if err != nil {
		return r, err
	}
	tx, err := s.Store.Write.BeginTx(ctx, nil)
	if err != nil {
		return r, err
	}
	defer tx.Rollback()
	metadata, err := ReadContext(ctx, tx)
	if err != nil {
		return r, err
	}
	if metadata.Epoch != cmd.Epoch {
		return r, accounting.Fail("DATASET_CHANGED", "Phiên dữ liệu đã thay đổi.")
	}
	r.Revision = metadata.Revision
	r.Documents = []Document{}
	for _, doc := range documents {
		saved, e := s.SaveTx(ctx, tx, doc)
		if e != nil {
			return r, e
		}
		r.Documents = append(r.Documents, saved)
	}
	return r, nil
}
func (s *Service) Import(ctx context.Context, cmd Command, input ImportInput) (json.RawMessage, error) {
	documents, err := parseImport(input.CSV)
	if err != nil {
		return nil, err
	}
	return s.Write(ctx, cmd, "document.import", input, func(tx *sql.Tx) (any, error) {
		metadata, e := ReadContext(ctx, tx)
		if e != nil {
			return nil, e
		}
		if input.ExpectedRevision == nil || *input.ExpectedRevision != metadata.Revision {
			return nil, accounting.Fail("VERSION_CONFLICT", "Dữ liệu đã thay đổi hoặc chưa xem trước. Kiểm tra lại gói nhập.")
		}
		result := ImportResult{Revision: metadata.Revision, Documents: []Document{}}
		for _, doc := range documents {
			saved, e := s.SaveTx(ctx, tx, doc)
			if e != nil {
				return nil, e
			}
			result.Documents = append(result.Documents, saved)
		}
		return result, nil
	})
}
