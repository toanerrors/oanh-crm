package httpserver

import (
	"io"
	"net/http"
	"strconv"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/modules/identity"
	"oanh.local/accounting/internal/modules/operations"
	"oanh.local/accounting/internal/platform/spreadsheet"
)

func (s *Server) excelUser(w http.ResponseWriter, r *http.Request) (identity.User, bool) {
	u, ok := s.user(w, r, false)
	if !ok {
		return u, false
	}
	if u.Role == "viewer" {
		s.failure(w, r, accounting.Fail("FORBIDDEN", "Không có quyền nhập dữ liệu Excel."))
		return u, false
	}
	return u, true
}
func (s *Server) excelSchemas(w http.ResponseWriter, r *http.Request) {
	u, ok := s.excelUser(w, r)
	if !ok {
		return
	}
	s.respond(w, 200, operations.ExcelSchemas(u.Role))
}
func (s *Server) excelTemplate(w http.ResponseWriter, r *http.Request) {
	u, ok := s.excelUser(w, r)
	if !ok {
		return
	}
	allowed := false
	for _, schema := range operations.ExcelSchemas(u.Role) {
		if schema.Key == r.URL.Query().Get("kind") {
			allowed = true
		}
	}
	if !allowed {
		s.failure(w, r, accounting.Fail("FORBIDDEN", "Không có quyền dùng mẫu nhập này."))
		return
	}
	content, err := s.Operations.ExcelTemplate(r.URL.Query().Get("kind"), r.URL.Query().Get("advanced") == "true")
	if err != nil {
		s.failure(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="mau-nhap-excel.xlsx"`)
	w.Write(content)
}
func (s *Server) excelUpload(w http.ResponseWriter, r *http.Request) {
	u, ok := s.excelUser(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, spreadsheet.MaxFileSize+(1<<20))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		s.failure(w, r, accounting.Fail("IMPORT_LIMIT", "Tải tệp XLSX hoặc CSV tối đa 20 MB."))
		return
	}
	defer r.MultipartForm.RemoveAll()
	file, header, err := r.FormFile("file")
	if err != nil {
		s.failure(w, r, accounting.Fail("BAD_REQUEST", "Chọn một tệp dữ liệu."))
		return
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, spreadsheet.MaxFileSize+1))
	if err != nil {
		s.failure(w, r, accounting.Fail("BAD_REQUEST", "Không đọc hết được tệp tải lên."))
		return
	}
	result, err := s.Operations.StageExcel(r.Context(), s.command(r, u), header.Filename, raw)
	s.result(w, r, result, err)
}
func (s *Server) excelSource(w http.ResponseWriter, r *http.Request) {
	u, ok := s.excelUser(w, r)
	if !ok {
		return
	}
	header := 1
	if raw := r.URL.Query().Get("headerRow"); raw != "" {
		var err error
		header, err = strconv.Atoi(raw)
		if err != nil {
			s.failure(w, r, accounting.Fail("BAD_REQUEST", "Dòng tiêu đề phải là số nguyên."))
			return
		}
	}
	result, err := s.Operations.ExcelSource(r.Context(), s.command(r, u), r.PathValue("id"), r.URL.Query().Get("sheet"), header)
	if err != nil {
		s.failure(w, r, err)
		return
	}
	s.respond(w, 200, result)
}
func (s *Server) excelHistory(w http.ResponseWriter, r *http.Request) {
	u, ok := s.excelUser(w, r)
	if !ok {
		return
	}
	result, err := s.Operations.ExcelHistory(r.Context(), s.command(r, u))
	if err != nil {
		s.failure(w, r, err)
		return
	}
	s.respond(w, 200, result)
}
func (s *Server) excelPreview(w http.ResponseWriter, r *http.Request) {
	u, ok := s.excelUser(w, r)
	if !ok {
		return
	}
	var input operations.ExcelConfiguration
	if !s.decode(w, r, &input) {
		return
	}
	result, err := s.Operations.PreviewExcel(r.Context(), s.command(r, u), r.PathValue("id"), input)
	if err != nil {
		s.failure(w, r, err)
		return
	}
	s.respond(w, 200, result)
}
func (s *Server) excelCommit(w http.ResponseWriter, r *http.Request) {
	u, ok := s.excelUser(w, r)
	if !ok {
		return
	}
	var input operations.ExcelCommit
	if !s.decode(w, r, &input) {
		return
	}
	result, err := s.Operations.CommitExcel(r.Context(), s.command(r, u), r.PathValue("id"), input)
	s.result(w, r, result, err)
}
func (s *Server) excelDiscard(w http.ResponseWriter, r *http.Request) {
	u, ok := s.excelUser(w, r)
	if !ok {
		return
	}
	result, err := s.Operations.DiscardExcel(r.Context(), s.command(r, u), r.PathValue("id"))
	s.result(w, r, result, err)
}

func (s *Server) excelErrors(w http.ResponseWriter, r *http.Request) {
	u, ok := s.excelUser(w, r)
	if !ok {
		return
	}
	var input operations.ExcelConfiguration
	if !s.decode(w, r, &input) {
		return
	}
	preview, err := s.Operations.PreviewExcel(r.Context(), s.command(r, u), r.PathValue("id"), input)
	if err != nil {
		s.failure(w, r, err)
		return
	}
	rows := [][]string{}
	for _, issue := range preview.Issues {
		row := strconv.Itoa(issue.Row)
		if issue.Row == 0 {
			row = "Toàn tệp"
		}
		rows = append(rows, []string{row, issue.Column, issue.Code, issue.Message})
	}
	content, err := spreadsheet.ErrorReport(rows)
	if err != nil {
		s.failure(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="loi-nhap-excel.xlsx"`)
	w.Write(content)
}

func (s *Server) excelConfiguration(w http.ResponseWriter, r *http.Request) {
	u, ok := s.excelUser(w, r)
	if !ok {
		return
	}
	var input operations.ExcelConfiguration
	if !s.decode(w, r, &input) {
		return
	}
	result, err := s.Operations.SaveExcelConfiguration(r.Context(), s.command(r, u), r.PathValue("id"), input)
	s.result(w, r, result, err)
}
