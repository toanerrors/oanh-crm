package accounting

import (
	"errors"
	"math/big"
	"regexp"
	"strconv"
	"time"
)

type Fault struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Fault) Error() string        { return e.Message }
func Fail(code, message string) error { return &Fault{code, message} }
func Code(err error) string {
	var fault *Fault
	if errors.As(err, &fault) {
		return fault.Code
	}
	return "INTERNAL_ERROR"
}

var integer = regexp.MustCompile(`^(0|[1-9][0-9]{0,18})$`)
var decimal = regexp.MustCompile(`^(0|[1-9][0-9]{0,14})(\.[0-9]{1,6})?$`)
var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:/-]{0,99}$`)

func Money(value string) (int64, error) {
	if !integer.MatchString(value) {
		return 0, Fail("INVALID_AMOUNT", "Số tiền phải là chuỗi số nguyên không âm.")
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, Fail("AMOUNT_OVERFLOW", "Số tiền vượt giới hạn int64.")
	}
	return n, nil
}
func Decimal(value string) (*big.Rat, error) {
	if !decimal.MatchString(value) {
		return nil, Fail("INVALID_DECIMAL", "Số thập phân phải dùng dấu chấm, tối đa 6 chữ số lẻ.")
	}
	n, ok := new(big.Rat).SetString(value)
	if !ok {
		return nil, Fail("INVALID_DECIMAL", "Số thập phân không hợp lệ.")
	}
	return n, nil
}
func Round(value *big.Rat, method string) (int64, error) {
	if value.Sign() < 0 {
		return 0, Fail("INVALID_AMOUNT", "Kết quả tiền không được âm.")
	}
	q, r := new(big.Int), new(big.Int)
	q.QuoRem(value.Num(), value.Denom(), r)
	switch method {
	case "down":
	case "half_up", "half_even":
		cmp := new(big.Int).Lsh(r, 1).Cmp(value.Denom())
		if cmp > 0 || (cmp == 0 && (method == "half_up" || q.Bit(0) == 1)) {
			q.Add(q, big.NewInt(1))
		}
	default:
		return 0, Fail("POLICY_REQUIRED", "Chọn phương pháp làm tròn trước khi tính.")
	}
	if !q.IsInt64() {
		return 0, Fail("AMOUNT_OVERFLOW", "Kết quả vượt giới hạn int64.")
	}
	return q.Int64(), nil
}
func Add(values ...int64) (int64, error) {
	n := new(big.Int)
	for _, v := range values {
		n.Add(n, big.NewInt(v))
	}
	if !n.IsInt64() {
		return 0, Fail("AMOUNT_OVERFLOW", "Tổng số tiền vượt giới hạn int64.")
	}
	return n.Int64(), nil
}
func Date(value string) bool {
	t, e := time.Parse("2006-01-02", value)
	return e == nil && t.Format("2006-01-02") == value
}
func Period(value string) bool {
	t, e := time.Parse("2006-01", value)
	return e == nil && t.Format("2006-01") == value
}
func ID(value string) bool { return identifier.MatchString(value) }
func Text(n int64) string  { return strconv.FormatInt(n, 10) }
func Rat(n int64) *big.Rat { return new(big.Rat).SetInt64(n) }

func Percent(base *big.Rat, rate, method string) (int64, error) {
	r, err := Decimal(rate)
	if err != nil {
		return 0, err
	}
	if r.Cmp(Rat(100)) > 0 {
		return 0, Fail("INVALID_RATE", "Tỷ lệ phải nằm trong khoảng 0–100%.")
	}
	return Round(new(big.Rat).Quo(new(big.Rat).Mul(base, r), Rat(100)), method)
}

func Quantity(value string, scale int) (int64, error) {
	r, err := Decimal(value)
	if err != nil {
		return 0, err
	}
	if scale < 0 || scale > 6 {
		return 0, Fail("POLICY_REQUIRED", "Scale số lượng phải từ 0 đến 6.")
	}
	multiplier := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil)
	r.Mul(r, new(big.Rat).SetInt(multiplier))
	if !r.IsInt() || !r.Num().IsInt64() || r.Sign() <= 0 {
		return 0, Fail("INVALID_QUANTITY", "Số lượng phải dương, đúng scale và trong giới hạn.")
	}
	return r.Num().Int64(), nil
}
