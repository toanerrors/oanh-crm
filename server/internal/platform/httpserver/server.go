package httpserver

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/modules/identity"
	"oanh.local/accounting/internal/modules/ledger"
	"oanh.local/accounting/internal/modules/operations"
	"oanh.local/accounting/internal/platform/sqlite"
)

type Server struct {
	Store         *sqlite.Store
	Identity      *identity.Service
	Ledger        *ledger.Service
	Operations    *operations.Service
	Assets        string
	SecureCookies bool
	mux           *http.ServeMux
}

func New(store *sqlite.Store, assets string, secure bool) *Server {
	s := &Server{Store: store, Assets: assets, SecureCookies: secure}
	s.Identity = &identity.Service{Store: store}
	s.Ledger = &ledger.Service{Store: store}
	s.Operations = &operations.Service{Ledger: s.Ledger}
	m := http.NewServeMux()
	s.mux = m
	m.HandleFunc("GET /api/v1/session", s.session)
	m.HandleFunc("POST /api/v1/setup", s.setup)
	m.HandleFunc("POST /api/v1/auth/login", s.login)
	m.HandleFunc("POST /api/v1/auth/logout", s.logout)
	m.HandleFunc("GET /api/v1/workspace", s.workspace)
	m.HandleFunc("GET /api/v1/documents", s.documents)
	m.HandleFunc("GET /api/v1/documents/export", s.exportDocuments)
	m.HandleFunc("GET /api/v1/opening-balances", s.openingGet)
	m.HandleFunc("POST /api/v1/opening-balances", s.openingSave)
	m.HandleFunc("POST /api/v1/opening-balances/confirm", s.openingPost)
	m.HandleFunc("POST /api/v1/opening-balances/discard", s.openingDiscard)
	m.HandleFunc("GET /api/v1/financial-reports/print", s.financialPrint)
	m.HandleFunc("GET /api/v1/financial-reports/source", s.financialSource)
	m.HandleFunc("GET /api/v1/financial-reports/template", s.financialTemplate)
	m.HandleFunc("GET /api/v1/financial-reports", s.financialList)
	m.HandleFunc("POST /api/v1/financial-reports/preview", s.financialPreview)
	m.HandleFunc("POST /api/v1/financial-reports", s.financialSave)
	m.HandleFunc("GET /api/v1/reports", s.report)
	m.HandleFunc("GET /api/v1/reports/export", s.exportReport)
	m.HandleFunc("GET /api/v1/reports/print", s.printReport)
	m.HandleFunc("GET /api/v1/documents/{id}", s.document)
	m.HandleFunc("GET /api/v1/documents/{id}/calculation", s.calculation)
	m.HandleFunc("POST /api/v1/documents", s.saveDocument)
	m.HandleFunc("POST /api/v1/imports/preview", s.previewImport)
	m.HandleFunc("POST /api/v1/imports", s.importDocuments)
	m.HandleFunc("POST /api/v1/documents/{id}/post", s.postDocument)
	m.HandleFunc("POST /api/v1/documents/{id}/reverse", s.reverseDocument)
	m.HandleFunc("POST /api/v1/partners", s.partners)
	m.HandleFunc("POST /api/v1/accounts", s.accounts)
	m.HandleFunc("GET /api/v1/accounts/reference", s.referenceChart)
	m.HandleFunc("POST /api/v1/accounts/reference", s.applyReferenceChart)
	m.HandleFunc("POST /api/v1/periods", s.periods)
	m.HandleFunc("POST /api/v1/periods/reopen", s.reopenPeriod)
	m.HandleFunc("GET /api/v1/automation", s.automation)
	m.HandleFunc("GET /api/v1/trade-invoices", s.invoices)
	m.HandleFunc("GET /api/v1/warehouses", s.warehouses)
	m.HandleFunc("POST /api/v1/warehouses", s.saveWarehouse)
	m.HandleFunc("GET /api/v1/purchase-costs", s.purchaseCosts)
	m.HandleFunc("GET /api/v1/bank-accounts", s.banks)
	m.HandleFunc("POST /api/v1/bank-accounts", s.saveBank)
	m.HandleFunc("POST /api/v1/policies", s.policy)
	m.HandleFunc("POST /api/v1/items", s.item)
	m.HandleFunc("POST /api/v1/employees", s.employee)
	m.HandleFunc("POST /api/v1/operations/preview", s.preview)
	m.HandleFunc("POST /api/v1/operations", s.execute)
	m.HandleFunc("POST /api/v1/voucher-drafts", s.saveVoucher)
	m.HandleFunc("GET /api/v1/documents/{id}/voucher", s.voucher)
	m.HandleFunc("GET /api/v1/documents/{id}/print", s.printVoucher)
	m.HandleFunc("GET /api/v1/audit", s.audit)
	m.HandleFunc("GET /api/v1/users", s.users)
	m.HandleFunc("POST /api/v1/users", s.saveUser)
	m.HandleFunc("POST /api/v1/backup", s.backup)
	m.HandleFunc("POST /api/v1/restore/stage", s.stageRestore)
	m.HandleFunc("POST /api/v1/restore/activate", s.activateRestore)
	m.HandleFunc("GET /api/v1/documents/{id}/attachments", s.attachments)
	m.HandleFunc("POST /api/v1/documents/{id}/attachments", s.uploadAttachment)
	m.HandleFunc("GET /api/v1/attachments/{id}", s.downloadAttachment)
	m.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		s.failure(w, r, accounting.Fail("NOT_FOUND", "Endpoint không tồn tại."))
	})
	m.HandleFunc("/", s.static)
	return s
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host, _, err := net.SplitHostPort(r.Host)
	if err != nil {
		host = r.Host
	}
	if !s.SecureCookies && host != "localhost" && host != "127.0.0.1" && host != "::1" {
		http.Error(w, "Untrusted host", http.StatusForbidden)
		return
	}
	requestID := rand.Text()
	w.Header().Set("X-Request-ID", requestID)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "same-origin")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; object-src 'none'; base-uri 'self'")
	if strings.HasPrefix(r.URL.Path, "/api/") {
		w.Header().Set("Cache-Control", "no-store")
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.Error("request panic", "request_id", requestID)
			s.failure(w, r, errors.New("request failed"))
		}
	}()
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	if r.URL.Path == "/api/v1/restore/activate" {
		s.Store.Gate.Lock()
		defer s.Store.Gate.Unlock()
	} else {
		s.Store.Gate.RLock()
		defer s.Store.Gate.RUnlock()
	}
	http.NewCrossOriginProtection().Handler(s.mux).ServeHTTP(w, r)
}
func (s *Server) decode(w http.ResponseWriter, r *http.Request, value any) bool {
	if media, _, e := mime.ParseMediaType(r.Header.Get("Content-Type")); e != nil || media != "application/json" {
		s.failure(w, r, accounting.Fail("BAD_REQUEST", "Content-Type phải là application/json."))
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		s.failure(w, r, accounting.Fail("BAD_REQUEST", "JSON không hợp lệ hoặc có trường không được phép."))
		return false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		s.failure(w, r, accounting.Fail("BAD_REQUEST", "Request chỉ được chứa một đối tượng JSON."))
		return false
	}
	return true
}
func (s *Server) respond(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Error("response write failed")
	}
}
func (s *Server) failure(w http.ResponseWriter, r *http.Request, err error) {
	code := accounting.Code(err)
	status := 422
	switch code {
	case "INTERNAL_ERROR":
		status = 500
	case "BAD_REQUEST":
		status = 400
	case "UNAUTHENTICATED":
		status = 401
	case "FORBIDDEN":
		status = 403
	case "NOT_FOUND":
		status = 404
	case "RATE_LIMITED":
		status = 429
	case "OPENING_LOCKED", "OPENING_HISTORY_EXISTS", "VERSION_CONFLICT", "DATASET_CHANGED", "IDEMPOTENCY_CONFLICT", "DUPLICATE_SOURCE", "DUPLICATE_INVOICE", "DUPLICATE_CODE", "DOCUMENT_POSTED", "ALREADY_INITIALIZED", "HISTORY_LOCKED", "STOCK_POLICY_LOCKED":
		status = 409
	}
	message := err.Error()
	if status == 500 {
		message = "Không thể hoàn tất thao tác. Liên hệ quản trị với mã yêu cầu."
		slog.Error("request failed", "request_id", w.Header().Get("X-Request-ID"), "method", r.Method, "path", r.URL.Path, "error", err)
	}
	s.respond(w, status, map[string]string{"code": code, "message": message, "request_id": w.Header().Get("X-Request-ID")})
}
func (s *Server) user(w http.ResponseWriter, r *http.Request, owner bool) (identity.User, bool) {
	cookie, e := r.Cookie("oanh_session")
	if e != nil {
		s.failure(w, r, accounting.Fail("UNAUTHENTICATED", "Cần đăng nhập."))
		return identity.User{}, false
	}
	user, e := s.Identity.Authenticate(r.Context(), cookie.Value)
	if e != nil {
		s.failure(w, r, e)
		return user, false
	}
	if owner && user.Role != "owner" {
		s.failure(w, r, accounting.Fail("FORBIDDEN", "Chỉ chủ sở hữu có quyền thực hiện."))
		return user, false
	}
	return user, true
}
func (s *Server) command(r *http.Request, u identity.User) ledger.Command {
	return ledger.Command{Actor: u.ID, Epoch: r.Header.Get("X-Dataset-Epoch"), Key: r.Header.Get("Idempotency-Key")}
}
func (s *Server) result(w http.ResponseWriter, r *http.Request, b json.RawMessage, e error) {
	if e != nil {
		s.failure(w, r, e)
		return
	}
	s.respond(w, 200, b)
}
func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	needs, e := s.Identity.NeedsSetup(r.Context())
	if e != nil {
		s.failure(w, r, e)
		return
	}
	value := map[string]any{"needsSetup": needs}
	if cookie, e := r.Cookie("oanh_session"); e == nil {
		if user, e := s.Identity.Authenticate(r.Context(), cookie.Value); e == nil {
			value["user"] = user
			c, e := ledger.ReadContext(r.Context(), s.Store.Read)
			if e != nil {
				s.failure(w, r, e)
				return
			}
			value["context"] = c
		}
	}
	s.respond(w, 200, value)
}
func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	var input identity.Setup
	if !s.decode(w, r, &input) {
		return
	}
	if err := s.Identity.Setup(r.Context(), input); err != nil {
		s.failure(w, r, err)
		return
	}
	s.respond(w, 201, map[string]bool{"initialized": true})
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !s.decode(w, r, &input) {
		return
	}
	user, token, e := s.Identity.Login(r.Context(), input.Username, input.Password)
	if e != nil {
		s.failure(w, r, e)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "oanh_session", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: s.SecureCookies, MaxAge: 43200})
	s.respond(w, 200, user)
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, e := r.Cookie("oanh_session"); e == nil {
		if e = s.Identity.Logout(r.Context(), cookie.Value); e != nil {
			s.failure(w, r, e)
			return
		}
	}
	http.SetCookie(w, &http.Cookie{Name: "oanh_session", Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: s.SecureCookies, MaxAge: -1})
	s.respond(w, 200, map[string]bool{"loggedOut": true})
}
func (s *Server) workspace(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r, false)
	if !ok {
		return
	}
	period := r.URL.Query().Get("period")
	if !accounting.Period(period) {
		s.failure(w, r, accounting.Fail("BAD_REQUEST", "Kỳ phải có dạng YYYY-MM."))
		return
	}
	value, e := s.Ledger.Snapshot(r.Context(), u.ID, u.Role, period)
	if e != nil {
		s.failure(w, r, e)
		return
	}
	s.respond(w, 200, value)
}
func (s *Server) document(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.user(w, r, false); !ok {
		return
	}
	tx, e := s.Store.Read.BeginTx(r.Context(), &sql.TxOptions{ReadOnly: true})
	if e != nil {
		s.failure(w, r, e)
		return
	}
	defer tx.Rollback()
	d, e := ledger.ReadDocument(r.Context(), tx, r.PathValue("id"))
	if e != nil {
		s.failure(w, r, e)
		return
	}
	s.respond(w, 200, d)
}
func (s *Server) saveDocument(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r, false)
	if !ok {
		return
	}
	var d ledger.Document
	if !s.decode(w, r, &d) {
		return
	}
	b, e := s.Ledger.Save(r.Context(), s.command(r, u), d)
	s.result(w, r, b, e)
}
func (s *Server) postDocument(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r, false)
	if !ok {
		return
	}
	var input ledger.Post
	if !s.decode(w, r, &input) {
		return
	}
	input.ID = r.PathValue("id")
	b, e := s.Ledger.Post(r.Context(), s.command(r, u), input)
	s.result(w, r, b, e)
}
func (s *Server) reverseDocument(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r, false)
	if !ok {
		return
	}
	var input ledger.Reversal
	if !s.decode(w, r, &input) {
		return
	}
	input.ID = r.PathValue("id")
	b, e := s.Ledger.Reverse(r.Context(), s.command(r, u), input)
	s.result(w, r, b, e)
}
func (s *Server) partners(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r, false)
	if !ok {
		return
	}
	var input struct {
		Partners []ledger.Partner `json:"partners"`
	}
	if !s.decode(w, r, &input) {
		return
	}
	b, e := s.Ledger.SavePartners(r.Context(), s.command(r, u), input.Partners)
	s.result(w, r, b, e)
}
func (s *Server) accounts(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r, true)
	if !ok {
		return
	}
	var input ledger.Account
	if !s.decode(w, r, &input) {
		return
	}
	b, e := s.Ledger.SaveAccount(r.Context(), s.command(r, u), input)
	s.result(w, r, b, e)
}
func (s *Server) periods(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r, true)
	if !ok {
		return
	}
	var input ledger.Period
	if !s.decode(w, r, &input) {
		return
	}
	b, e := s.Ledger.SavePeriod(r.Context(), s.command(r, u), input)
	s.result(w, r, b, e)
}
func (s *Server) reopenPeriod(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r, true)
	if !ok {
		return
	}
	var input ledger.Reopen
	if !s.decode(w, r, &input) {
		return
	}
	b, e := s.Ledger.ReopenPeriod(r.Context(), s.command(r, u), input)
	s.result(w, r, b, e)
}
func (s *Server) automation(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r, false)
	if !ok {
		return
	}
	if u.Role == "viewer" {
		s.failure(w, r, accounting.Fail("FORBIDDEN", "Không có quyền xem dữ liệu nhân sự và cấu hình nghiệp vụ."))
		return
	}
	d, e := s.Operations.Data(r.Context())
	if e != nil {
		s.failure(w, r, e)
		return
	}
	s.respond(w, 200, d)
}
func (s *Server) calculation(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r, false)
	if !ok {
		return
	}
	if u.Role == "viewer" {
		s.failure(w, r, accounting.Fail("FORBIDDEN", "Không có quyền xem cơ sở tính nghiệp vụ và thông tin lương."))
		return
	}
	value, err := s.Operations.Trace(r.Context(), r.PathValue("id"))
	if err != nil {
		s.failure(w, r, err)
		return
	}
	s.respond(w, 200, value)
}
func (s *Server) policy(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r, true)
	if !ok {
		return
	}
	var input operations.Policy
	if !s.decode(w, r, &input) {
		return
	}
	b, e := s.Operations.SavePolicy(r.Context(), s.command(r, u), input)
	s.result(w, r, b, e)
}
func (s *Server) item(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r, false)
	if !ok {
		return
	}
	var input operations.Item
	if !s.decode(w, r, &input) {
		return
	}
	b, e := s.Operations.SaveItem(r.Context(), s.command(r, u), input)
	s.result(w, r, b, e)
}
func (s *Server) employee(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r, true)
	if !ok {
		return
	}
	var input operations.Employee
	if !s.decode(w, r, &input) {
		return
	}
	b, e := s.Operations.SaveEmployee(r.Context(), s.command(r, u), input)
	s.result(w, r, b, e)
}
func (s *Server) preview(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r, false)
	if !ok {
		return
	}
	var input operations.Input
	if !s.decode(w, r, &input) {
		return
	}
	v, e := s.Operations.Preview(r.Context(), u, r.Header.Get("X-Dataset-Epoch"), input)
	if e != nil {
		s.failure(w, r, e)
		return
	}
	s.respond(w, 200, v)
}
func (s *Server) execute(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r, false)
	if !ok {
		return
	}
	var input operations.Input
	if !s.decode(w, r, &input) {
		return
	}
	if input.ExpectedRevision == nil || input.ExpectedPolicyID == "" {
		s.failure(w, r, accounting.Fail("PREVIEW_REQUIRED", "Tính và kiểm tra bản xem trước trước khi ghi nhận."))
		return
	}
	if input.Kind == "closing" && u.Role != "owner" {
		s.failure(w, r, accounting.Fail("FORBIDDEN", "Chỉ chủ sở hữu được kết chuyển kết quả kinh doanh."))
		return
	}
	b, e := s.Operations.Execute(r.Context(), s.command(r, u), input)
	s.result(w, r, b, e)
}
func (s *Server) audit(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.user(w, r, true); !ok {
		return
	}
	rows, e := s.Store.Read.QueryContext(r.Context(), "SELECT a.id,u.name,a.action,a.entity_id,a.occurred_at,a.revision FROM audit a JOIN users u ON u.id=a.actor_id ORDER BY a.id DESC LIMIT 200")
	if e != nil {
		s.failure(w, r, e)
		return
	}
	defer rows.Close()
	values := []map[string]any{}
	for rows.Next() {
		var id, revision int64
		var user, action, entity, occurred string
		if e = rows.Scan(&id, &user, &action, &entity, &occurred, &revision); e != nil {
			s.failure(w, r, e)
			return
		}
		values = append(values, map[string]any{"id": id, "actor": user, "action": action, "entity": entity, "occurredAt": occurred, "revision": revision})
	}
	if e = rows.Err(); e != nil {
		s.failure(w, r, e)
		return
	}
	s.respond(w, 200, values)
}
func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	if s.Assets == "" {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(s.Assets, filepath.FromSlash(strings.TrimPrefix(r.URL.Path, "/")))
	if r.URL.Path == "/" {
		path = filepath.Join(s.Assets, "index.html")
	}
	info, e := os.Stat(path)
	if e == nil && !info.IsDir() {
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "public,max-age=31536000,immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		http.ServeFile(w, r, path)
		return
	}
	if filepath.Ext(r.URL.Path) != "" || !strings.Contains(r.Header.Get("Accept"), "text/html") {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, filepath.Join(s.Assets, "index.html"))
}
func (s *Server) backup(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.user(w, r, true); !ok {
		return
	}
	c, e := ledger.ReadContext(r.Context(), s.Store.Read)
	if e != nil {
		s.failure(w, r, e)
		return
	}
	if r.Header.Get("X-Dataset-Epoch") != c.Epoch {
		s.failure(w, r, accounting.Fail("DATASET_CHANGED", "Phiên dữ liệu đã thay đổi."))
		return
	}
	path, _, e := s.Store.Backup(r.Context())
	if e != nil {
		s.failure(w, r, e)
		return
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filepath.Base(path)}))
	w.Header().Set("Content-Type", "application/zip")
	http.ServeFile(w, r, path)
}
func (s *Server) stageRestore(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.user(w, r, true); !ok {
		return
	}
	dir := filepath.Join(s.Store.Root, "uploads")
	if e := os.MkdirAll(dir, 0700); e != nil {
		s.failure(w, r, e)
		return
	}
	f, e := os.CreateTemp(dir, "restore-")
	if e != nil {
		s.failure(w, r, e)
		return
	}
	defer os.Remove(f.Name())
	r.Body = http.MaxBytesReader(w, r.Body, 128<<20)
	_, e = io.Copy(f, r.Body)
	closeErr := f.Close()
	if e == nil {
		e = closeErr
	}
	if e != nil {
		s.failure(w, r, accounting.Fail("BAD_REQUEST", "Gói restore lỗi hoặc vượt 128 MB."))
		return
	}
	staged, e := s.Store.Stage(r.Context(), f.Name())
	if e != nil {
		s.failure(w, r, accounting.Fail("INVALID_BACKUP", "Gói backup không đạt kiểm tra checksum, schema hoặc đối soát."))
		return
	}
	s.respond(w, 200, staged)
}
func (s *Server) activateRestore(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r, true)
	if !ok {
		return
	}
	var input struct {
		StageID          string `json:"stageId"`
		ConfirmCompanyID string `json:"confirmCompanyId"`
	}
	if !s.decode(w, r, &input) {
		return
	}
	c, e := ledger.ReadContext(r.Context(), s.Store.Read)
	if e != nil {
		s.failure(w, r, e)
		return
	}
	if r.Header.Get("X-Dataset-Epoch") != c.Epoch || input.ConfirmCompanyID != c.CompanyID {
		s.failure(w, r, accounting.Fail("DATASET_CHANGED", "Xác nhận đúng doanh nghiệp và phiên dữ liệu đang dùng trước restore."))
		return
	}
	m, e := s.Store.Activate(r.Context(), input.StageID, u.ID)
	if e != nil {
		s.failure(w, r, e)
		return
	}
	s.respond(w, 200, map[string]any{"restored": true, "snapshot": m, "loginRequired": true})
}
func (s *Server) attachments(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.user(w, r, false); !ok {
		return
	}
	rows, e := s.Store.Read.QueryContext(r.Context(), "SELECT id,name,size FROM attachments WHERE document_id=? ORDER BY name", r.PathValue("id"))
	if e != nil {
		s.failure(w, r, e)
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id, name string
		var size int64
		if e = rows.Scan(&id, &name, &size); e != nil {
			s.failure(w, r, e)
			return
		}
		list = append(list, map[string]any{"id": id, "name": name, "size": size})
	}
	if e = rows.Err(); e != nil {
		s.failure(w, r, e)
		return
	}
	s.respond(w, 200, list)
}
func (s *Server) uploadAttachment(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r, false)
	if !ok {
		return
	}
	if u.Role == "viewer" {
		s.failure(w, r, accounting.Fail("FORBIDDEN", "Không có quyền thêm chứng từ đính kèm."))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 11<<20)
	if e := r.ParseMultipartForm(10 << 20); e != nil {
		s.failure(w, r, accounting.Fail("BAD_REQUEST", "File đính kèm tối đa 10 MB."))
		return
	}
	defer r.MultipartForm.RemoveAll()
	file, header, e := r.FormFile("file")
	if e != nil {
		s.failure(w, r, accounting.Fail("BAD_REQUEST", "Chọn file đính kèm."))
		return
	}
	defer file.Close()
	content, e := io.ReadAll(io.LimitReader(file, 10<<20+1))
	if e != nil || len(content) > 10<<20 {
		s.failure(w, r, accounting.Fail("BAD_REQUEST", "File đính kèm vượt giới hạn."))
		return
	}
	hash := sqlite.Hash(content)
	if e = sqlite.WriteFile(filepath.Join(s.Store.Dataset, "attachments", hash), content); e != nil {
		s.failure(w, r, e)
		return
	}
	name := filepath.Base(header.Filename)
	payload := map[string]any{"documentId": r.PathValue("id"), "hash": hash, "name": name, "size": len(content)}
	b, e := s.Ledger.Write(r.Context(), s.command(r, u), "attachment.add", payload, func(tx *sql.Tx) (any, error) {
		if _, e := ledger.ReadDocument(r.Context(), tx, r.PathValue("id")); e != nil {
			return nil, e
		}
		var existing string
		err := tx.QueryRowContext(r.Context(), "SELECT id FROM attachments WHERE document_id=? AND hash=? AND name=?", r.PathValue("id"), hash, name).Scan(&existing)
		if err == nil {
			return map[string]string{"id": existing, "name": name}, nil
		}
		if err != sql.ErrNoRows {
			return nil, err
		}
		id := sqlite.ID()
		_, e := tx.ExecContext(r.Context(), "INSERT INTO attachments VALUES(?,?,?,?,?)", id, r.PathValue("id"), hash, name, len(content))
		return map[string]string{"id": id, "name": name}, e
	})
	s.result(w, r, b, e)
}
func (s *Server) downloadAttachment(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.user(w, r, false); !ok {
		return
	}
	var hash, name string
	if e := s.Store.Read.QueryRowContext(r.Context(), "SELECT hash,name FROM attachments WHERE id=?", r.PathValue("id")).Scan(&hash, &name); e != nil {
		s.failure(w, r, accounting.Fail("NOT_FOUND", "Không tìm thấy file đính kèm."))
		return
	}
	if len(hash) != 64 || strings.ContainsAny(hash, "/\\.") {
		s.failure(w, r, fmt.Errorf("invalid blob hash"))
		return
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeFile(w, r, filepath.Join(s.Store.Dataset, "attachments", hash))
}
