package identity

import (
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"strings"
	"time"
	"unicode/utf8"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/platform/sqlite"
)

type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	Active   bool   `json:"active"`
}
type Setup struct {
	Username        string `json:"username"`
	Password        string `json:"password"`
	Name            string `json:"name"`
	CompanyName     string `json:"companyName"`
	Regime          string `json:"regime"`
	ConfirmAccounts bool   `json:"confirmAccounts"`
}
type Service struct{ Store *sqlite.Store }

func PasswordHash(password string) (string, error) {
	if utf8.RuneCountInString(password) < 12 || len(password) > 1024 {
		return "", accounting.Fail("VALIDATION", "Mật khẩu cần ít nhất 12 ký tự và không quá 1.024 byte.")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, 600000, 32)
	if err != nil {
		return "", err
	}
	return base64.RawStdEncoding.EncodeToString(salt) + ":" + base64.RawStdEncoding.EncodeToString(key), nil
}
func verify(password, encoded string) bool {
	parts := strings.Split(encoded, ":")
	if len(parts) != 2 {
		return false
	}
	salt, e := base64.RawStdEncoding.DecodeString(parts[0])
	if e != nil {
		return false
	}
	want, e := base64.RawStdEncoding.DecodeString(parts[1])
	if e != nil {
		return false
	}
	got, e := pbkdf2.Key(sha256.New, password, salt, 600000, 32)
	return e == nil && subtle.ConstantTimeCompare(got, want) == 1
}
func (s *Service) NeedsSetup(ctx context.Context) (bool, error) {
	var n int
	err := s.Store.Read.QueryRowContext(ctx, "SELECT count(*) FROM users").Scan(&n)
	return n == 0, err
}
func (s *Service) Setup(ctx context.Context, input Setup) error {
	needs, err := s.NeedsSetup(ctx)
	if err != nil {
		return err
	}
	if !needs {
		return accounting.Fail("ALREADY_INITIALIZED", "Không gian đã được khởi tạo. Hãy đăng nhập.")
	}
	input.Username = strings.ToLower(strings.TrimSpace(input.Username))
	if !accounting.ID(input.Username) || len(input.Username) > 64 || strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.CompanyName) == "" || strings.TrimSpace(input.Regime) == "" || !input.ConfirmAccounts {
		return accounting.Fail("VALIDATION", "Nhập đầy đủ thông tin và xác nhận danh mục tài khoản khởi tạo.")
	}
	hash, err := PasswordHash(input.Password)
	if err != nil {
		return err
	}
	tx, err := s.Store.Write.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM users").Scan(&n); err != nil {
		return err
	}
	if n != 0 {
		return accounting.Fail("ALREADY_INITIALIZED", "Không gian đã được khởi tạo. Hãy đăng nhập.")
	}
	id := sqlite.ID()
	if _, err = tx.ExecContext(ctx, "INSERT INTO users VALUES(?,?,?,?,?,1)", id, input.Username, input.Name, "owner", hash); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE meta SET company_name=?,regime=?,revision=revision+1 WHERE id=1", input.CompanyName, input.Regime); err != nil {
		return err
	}
	for _, a := range initialAccounts {
		if _, err = tx.ExecContext(ctx, "INSERT INTO accounts VALUES(?,?,?,?,?,1) ON CONFLICT(code) DO NOTHING", a[0], a[0], a[1], a[2], a[3] == "1"); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO audit(actor_id,action,entity_id,occurred_at,revision) SELECT ?,'setup',company_id,?,revision FROM meta", id, sqlite.Now()); err != nil {
		return err
	}
	return tx.Commit()
}

var initialAccounts = [][4]string{
	{"821", "Chi phí thuế thu nhập doanh nghiệp", "expense", "0"}, {"3334", "Thuế thu nhập doanh nghiệp", "tax", "0"},
	{"111", "Tiền mặt", "cash", "0"}, {"1121", "Tiền gửi ngân hàng", "cash", "0"}, {"131", "Phải thu của khách hàng", "receivable", "1"}, {"1331", "Thuế GTGT được khấu trừ", "tax", "0"}, {"156", "Hàng hóa", "inventory", "0"}, {"154", "Chi phí sản xuất kinh doanh dở dang", "wip", "0"}, {"211", "Tài sản cố định hữu hình", "asset", "0"}, {"214", "Hao mòn tài sản cố định", "asset", "0"}, {"242", "Chi phí trả trước", "asset", "0"}, {"331", "Phải trả cho người bán", "payable", "1"}, {"33311", "Thuế GTGT phải nộp", "tax", "0"}, {"3335", "Thuế thu nhập cá nhân", "tax", "0"}, {"334", "Phải trả người lao động", "liability", "0"}, {"338", "Phải trả bảo hiểm", "liability", "0"}, {"411", "Vốn đầu tư của chủ sở hữu", "capital", "0"}, {"421", "Lợi nhuận sau thuế chưa phân phối", "capital", "0"}, {"511", "Doanh thu bán hàng và cung cấp dịch vụ", "revenue", "0"}, {"632", "Giá vốn hàng bán", "expense", "0"}, {"642", "Chi phí quản lý doanh nghiệp", "expense", "0"}, {"911", "Xác định kết quả kinh doanh", "capital", "0"},
}

func (s *Service) Login(ctx context.Context, username, password string) (User, string, error) {
	var user User
	username = strings.ToLower(strings.TrimSpace(username))
	if len(username) > 64 || len(password) > 1024 {
		return user, "", accounting.Fail("UNAUTHENTICATED", "Tên đăng nhập hoặc mật khẩu không đúng.")
	}
	var blocked string
	err := s.Store.Read.QueryRowContext(ctx, "SELECT blocked_until FROM login_limits WHERE username=?", username).Scan(&blocked)
	if err != nil && err != sql.ErrNoRows {
		return user, "", err
	}
	if blocked > time.Now().UTC().Format(time.RFC3339) {
		return user, "", accounting.Fail("RATE_LIMITED", "Đăng nhập sai nhiều lần. Thử lại sau 15 phút.")
	}
	var encoded string
	err = s.Store.Read.QueryRowContext(ctx, "SELECT id,username,name,role,active,password_hash FROM users WHERE username=?", username).Scan(&user.ID, &user.Username, &user.Name, &user.Role, &user.Active, &encoded)
	if err != nil && err != sql.ErrNoRows {
		return user, "", err
	}
	if err == sql.ErrNoRows {
		encoded = "AAAAAAAAAAAAAAAAAAAAAA:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	}
	valid := verify(password, encoded) && user.Active
	tx, err := s.Store.Write.BeginTx(ctx, nil)
	if err != nil {
		return user, "", err
	}
	defer tx.Rollback()
	if !valid {
		until := time.Now().UTC().Add(15 * time.Minute).Format(time.RFC3339)
		if _, err = tx.ExecContext(ctx, "INSERT INTO login_limits VALUES(?,1,'') ON CONFLICT(username) DO UPDATE SET failures=CASE WHEN blocked_until<>'' AND blocked_until<? THEN 1 ELSE failures+1 END,blocked_until=CASE WHEN blocked_until<>'' AND blocked_until<? THEN '' WHEN failures>=4 THEN ? ELSE '' END", username, sqlite.Now(), sqlite.Now(), until); err != nil {
			return user, "", err
		}
		if err = tx.Commit(); err != nil {
			return user, "", err
		}
		return User{}, "", accounting.Fail("UNAUTHENTICATED", "Tên đăng nhập hoặc mật khẩu không đúng.")
	}
	var active bool
	var currentHash string
	if err = tx.QueryRowContext(ctx, "SELECT active,password_hash FROM users WHERE id=?", user.ID).Scan(&active, &currentHash); err != nil {
		return user, "", err
	}
	if !active || encoded != currentHash {
		return User{}, "", accounting.Fail("UNAUTHENTICATED", "Thông tin đăng nhập đã thay đổi. Hãy thử lại.")
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM login_limits WHERE username=?", username); err != nil {
		return user, "", err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at<?", sqlite.Now()); err != nil {
		return user, "", err
	}
	token := rand.Text()
	if _, err = tx.ExecContext(ctx, "INSERT INTO sessions VALUES(?,?,?)", sqlite.Hash([]byte(token)), user.ID, time.Now().UTC().Add(12*time.Hour).Format(time.RFC3339)); err != nil {
		return user, "", err
	}
	if err = tx.Commit(); err != nil {
		return User{}, "", err
	}
	return user, token, nil
}
func (s *Service) Authenticate(ctx context.Context, token string) (User, error) {
	var user User
	if len(token) < 20 || len(token) > 200 {
		return user, accounting.Fail("UNAUTHENTICATED", "Cần đăng nhập.")
	}
	err := s.Store.Read.QueryRowContext(ctx, "SELECT u.id,u.username,u.name,u.role,u.active FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=? AND s.expires_at>? AND u.active=1", sqlite.Hash([]byte(token)), time.Now().UTC().Format(time.RFC3339)).Scan(&user.ID, &user.Username, &user.Name, &user.Role, &user.Active)
	if err == sql.ErrNoRows {
		return user, accounting.Fail("UNAUTHENTICATED", "Phiên đăng nhập đã hết hạn. Hãy đăng nhập lại.")
	}
	return user, err
}
func (s *Service) Logout(ctx context.Context, token string) error {
	_, err := s.Store.Write.ExecContext(ctx, "DELETE FROM sessions WHERE token_hash=?", sqlite.Hash([]byte(token)))
	return err
}
