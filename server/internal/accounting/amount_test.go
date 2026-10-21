package accounting

import (
	"math/big"
	"testing"
)

func TestExactAmounts(t *testing.T) {
	for _, v := range []string{"9007199254740993", "9223372036854775807"} {
		n, e := Money(v)
		if e != nil || Text(n) != v {
			t.Fatalf("%s: %v", v, e)
		}
	}
	for _, v := range []string{"9223372036854775808", "1.5", "01", "-1", "1e3"} {
		if _, e := Money(v); e == nil {
			t.Fatalf("accepted %s", v)
		}
	}
	for _, tc := range []struct {
		v, mode string
		want    int64
	}{{"1.5", "half_up", 2}, {"2.5", "half_even", 2}, {"3.5", "half_even", 4}, {"1.9", "down", 1}} {
		r, _ := new(big.Rat).SetString(tc.v)
		n, e := Round(r, tc.mode)
		if e != nil || n != tc.want {
			t.Fatalf("%+v got %d %v", tc, n, e)
		}
	}
	if _, e := Add(9223372036854775807, 1); Code(e) != "AMOUNT_OVERFLOW" {
		t.Fatal(e)
	}
	if _, e := Quantity("1.234", 2); e == nil {
		t.Fatal("scale accepted")
	}
}
