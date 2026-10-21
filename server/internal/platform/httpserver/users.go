package httpserver

import (
	"crypto/pbkdf2"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"net/http"
	"strings"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/modules/identity"
)

func (s *Server) users(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.user(w, r, true); !ok {
		return
	}
	rows, e := s.Store.Read.QueryContext(r.Context(), "SELECT id,username,name,role,active FROM users ORDER BY username")
	if e != nil {
		s.failure(w, r, e)
		return
	}
	defer rows.Close()
	users := []identity.User{}
	for rows.Next() {
		var u identity.User
		if e = rows.Scan(&u.ID, &u.Username, &u.Name, &u.Role, &u.Active); e != nil {
			s.failure(w, r, e)
			return
		}
		users = append(users, u)
	}
	if e = rows.Err(); e != nil {
		s.failure(w, r, e)
		return
	}
	s.respond(w, 200, users)
}
func (s *Server) saveUser(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.user(w, r, true)
	if !ok {
		return
	}
	var input struct {
		identity.User
		Password string `json:"password"`
	}
	if !s.decode(w, r, &input) {
		return
	}
	input.Username = strings.ToLower(strings.TrimSpace(input.Username))
	if !accounting.ID(input.ID) || !accounting.ID(input.Username) || len(input.Username) > 64 || strings.TrimSpace(input.Name) == "" || !strings.Contains("|owner|accountant|viewer|", "|"+input.Role+"|") {
		s.failure(w, r, accounting.Fail("VALIDATION", "Thông tin người dùng hoặc vai trò không hợp lệ."))
		return
	}
	hash := ""
	var e error
	if input.Password != "" {
		hash, e = identity.PasswordHash(input.Password)
		if e != nil {
			s.failure(w, r, e)
			return
		}
	}
	cmd := s.command(r, actor)
	fingerprint, e := pbkdf2.Key(sha256.New, input.Password, []byte(cmd.Actor+":"+cmd.Key), 600000, 32)
	if e != nil {
		s.failure(w, r, e)
		return
	}
	payload := struct {
		identity.User
		PasswordVerifier string `json:"passwordVerifier"`
	}{input.User, hex.EncodeToString(fingerprint)}
	b, e := s.Ledger.Write(r.Context(), cmd, "admin.user.save", payload, func(tx *sql.Tx) (any, error) {
		var oldRole, oldUsername string
		var active bool
		err := tx.QueryRowContext(r.Context(), "SELECT role,active,username FROM users WHERE id=?", input.ID).Scan(&oldRole, &active, &oldUsername)
		if err != nil && err != sql.ErrNoRows {
			return nil, err
		}
		if err == sql.ErrNoRows {
			if hash == "" {
				return nil, accounting.Fail("VALIDATION", "Nhập mật khẩu cho người dùng mới.")
			}
			_, err = tx.ExecContext(r.Context(), "INSERT INTO users VALUES(?,?,?,?,?,?)", input.ID, input.Username, input.Name, input.Role, hash, input.Active)
		} else {
			if oldUsername != input.Username {
				return nil, accounting.Fail("HISTORY_LOCKED", "Giữ nguyên tên đăng nhập đã tạo.")
			}
			if oldRole == "owner" && active && (!input.Active || input.Role != "owner") {
				var n int
				if err = tx.QueryRowContext(r.Context(), "SELECT count(*) FROM users WHERE role='owner' AND active=1").Scan(&n); err != nil {
					return nil, err
				}
				if n <= 1 {
					return nil, accounting.Fail("LAST_OWNER", "Phải giữ ít nhất một chủ sở hữu đang hoạt động.")
				}
			}
			_, err = tx.ExecContext(r.Context(), "UPDATE users SET name=?,role=?,active=? WHERE id=?", input.Name, input.Role, input.Active, input.ID)
			if err == nil && hash != "" {
				_, err = tx.ExecContext(r.Context(), "UPDATE users SET password_hash=? WHERE id=?", hash, input.ID)
			}
			if err == nil {
				_, err = tx.ExecContext(r.Context(), "DELETE FROM sessions WHERE user_id=?", input.ID)
			}
		}
		return input.User, err
	})
	s.result(w, r, b, e)
}
