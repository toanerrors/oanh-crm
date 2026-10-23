package httpserver

import (
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/platform/sqlite"
)

func (s *Server) backups(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.user(w, r, true); !ok {
		return
	}
	page, size := 1, 20
	for key, target := range map[string]*int{"page": &page, "pageSize": &size} {
		if raw := r.URL.Query().Get(key); raw != "" {
			n, e := strconv.Atoi(raw)
			if e != nil || n < 1 || n > 1000000 || key == "pageSize" && n > 100 {
				s.failure(w, r, accounting.Fail("BAD_REQUEST", "Phân trang bản sao lưu không hợp lệ."))
				return
			}
			*target = n
		}
	}
	current, err := s.Store.RecoveryCurrent(r.Context())
	if err != nil {
		s.failure(w, r, err)
		return
	}
	items, unreadable, err := s.Store.ListBackups(r.Context(), current.CompanyID)
	if err != nil {
		s.failure(w, r, err)
		return
	}
	schedule, err := s.Store.ReadBackupSchedule(current.CompanyID)
	if err != nil {
		s.failure(w, r, err)
		return
	}
	job, err := s.Store.ReadBackupJob(current, time.Now())
	if err != nil {
		s.failure(w, r, err)
		return
	}
	count := len(items)
	var totalBytes int64
	for i := range items {
		totalBytes += items[i].Size
		items[i].Manifest.Files = nil
	}
	var latest any
	if len(items) > 0 {
		latest = items[0]
	}
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	if utf8.RuneCountInString(q) > 200 {
		s.failure(w, r, accounting.Fail("BAD_REQUEST", "Từ khóa quá dài."))
		return
	}
	if q != "" {
		filtered := []sqlite.BackupInfo{}
		for _, item := range items {
			if strings.Contains(strings.ToLower(item.Note+" "+item.ActorName+" "+item.Kind+" "+item.Manifest.CreatedAt), q) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	start := (page - 1) * size
	if start > len(items) {
		start = len(items)
	}
	end := min(start+size, len(items))
	s.respond(w, 200, map[string]any{"items": items[start:end], "total": len(items), "page": page, "pageSize": size, "count": count, "totalBytes": totalBytes, "unreadable": unreadable, "latest": latest, "current": current, "schedule": schedule, "job": job, "nextScheduledAt": sqlite.NextBackupAt(time.Now()), "recoveryEvents": s.Store.RecoveryEvents(current.CompanyID)})
}
func (s *Server) createBackup(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r, true)
	if !ok {
		return
	}
	var input struct {
		Note string `json:"note"`
	}
	if !s.decode(w, r, &input) {
		return
	}
	input.Note = strings.TrimSpace(input.Note)
	if utf8.RuneCountInString(input.Note) > 300 {
		s.failure(w, r, accounting.Fail("VALIDATION", "Ghi chú bản sao tối đa 300 ký tự."))
		return
	}
	current, err := s.Store.RecoveryCurrent(r.Context())
	if err != nil {
		s.failure(w, r, err)
		return
	}
	if r.Header.Get("X-Dataset-Epoch") != current.Epoch {
		s.failure(w, r, accounting.Fail("DATASET_CHANGED", "Phiên dữ liệu đã thay đổi."))
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if !accounting.ID(key) {
		s.failure(w, r, accounting.Fail("BAD_REQUEST", "Cần khóa chống tạo trùng bản sao."))
		return
	}
	id := sqlite.Hash([]byte("manual:" + current.CompanyID + ":" + current.Epoch + ":" + u.ID + ":" + key))
	path, _, err := s.Store.BackupWithOptions(r.Context(), sqlite.BackupOptions{ID: id, Kind: "manual", ActorID: u.ID, ActorName: u.Name, Note: input.Note, RequestHash: sqlite.Hash([]byte(input.Note))})
	if err != nil {
		s.failure(w, r, err)
		return
	}
	info, err := s.Store.BackupDetails(path)
	if err != nil {
		s.failure(w, r, err)
		return
	}
	s.respond(w, 200, info)
}
func (s *Server) downloadBackup(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.user(w, r, true); !ok {
		return
	}
	current, err := s.Store.RecoveryCurrent(r.Context())
	if err != nil {
		s.failure(w, r, err)
		return
	}
	path, info, err := s.Store.ReadBackup(r.Context(), r.PathValue("id"), current.CompanyID, false)
	if err != nil {
		s.failure(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": info.Filename}))
	w.Header().Set("X-Backup-SHA256", info.SHA256)
	http.ServeFile(w, r, path)
}
func (s *Server) verifyBackup(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.user(w, r, true); !ok {
		return
	}
	current, err := s.Store.RecoveryCurrent(r.Context())
	if err != nil {
		s.failure(w, r, err)
		return
	}
	if r.Header.Get("X-Dataset-Epoch") != current.Epoch {
		s.failure(w, r, accounting.Fail("DATASET_CHANGED", "Phiên dữ liệu đã thay đổi."))
		return
	}
	_, info, err := s.Store.ReadBackup(r.Context(), r.PathValue("id"), current.CompanyID, true)
	if err != nil {
		s.failure(w, r, err)
		return
	}
	s.respond(w, 200, info)
}
func (s *Server) previewStoredBackup(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r, true)
	if !ok {
		return
	}
	current, err := s.Store.RecoveryCurrent(r.Context())
	if err != nil {
		s.failure(w, r, err)
		return
	}
	if r.Header.Get("X-Dataset-Epoch") != current.Epoch {
		s.failure(w, r, accounting.Fail("DATASET_CHANGED", "Phiên dữ liệu đã thay đổi."))
		return
	}
	path, _, err := s.Store.ReadBackup(r.Context(), r.PathValue("id"), current.CompanyID, false)
	if err != nil {
		s.failure(w, r, err)
		return
	}
	preview, err := s.Store.PrepareRestore(r.Context(), path, u.ID)
	if err != nil {
		s.failure(w, r, err)
		return
	}
	s.respond(w, 200, preview)
}
func (s *Server) saveBackupSchedule(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r, true)
	if !ok {
		return
	}
	var input struct {
		Enabled bool  `json:"enabled"`
		Version int64 `json:"version"`
	}
	if !s.decode(w, r, &input) {
		return
	}
	current, err := s.Store.RecoveryCurrent(r.Context())
	if err != nil {
		s.failure(w, r, err)
		return
	}
	if r.Header.Get("X-Dataset-Epoch") != current.Epoch {
		s.failure(w, r, accounting.Fail("DATASET_CHANGED", "Phiên dữ liệu đã thay đổi."))
		return
	}
	schedule, err := s.Store.SaveBackupSchedule(current.CompanyID, u.Name, input.Enabled, input.Version)
	if err != nil {
		s.failure(w, r, err)
		return
	}
	s.respond(w, 200, schedule)
}
