package services

import "testing"

func TestPriceToCents(t *testing.T) {
	cases := map[float64]int{999: 99900, 19.99: 1999, 0.1 + 0.2: 30, 0: 0, 1234.565: 123457}
	for in, want := range cases {
		if got := priceToCents(in); got != want {
			t.Errorf("priceToCents(%v) = %d, want %d", in, got, want)
		}
	}
}
