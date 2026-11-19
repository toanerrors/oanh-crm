package accounting

import (
	"math/big"
	"strings"
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

func TestCatalogCode(t *testing.T) {
	for _, valid := range []string{"A", "KH1", "OLAM-TN", "CHYANG SHENG", "GEN BRIGHT", "TTI CUCHI", "TTI DAUGIAY", "TTI Q9", "KH.01/A:B_C", "A" + strings.Repeat("B", 99)} {
		if !CatalogCode(valid) {
			t.Fatalf("rejected valid catalog code %q", valid)
		}
	}
	for _, invalid := range []string{"", " ", " TTI", "TTI ", "TTI  Q9", "TTI\tQ9", "TTI\nQ9", "-KH1", "KHÁCH 1", "KH@1", "A" + strings.Repeat("B", 100)} {
		if CatalogCode(invalid) {
			t.Fatalf("accepted invalid catalog code %q", invalid)
		}
	}
}
