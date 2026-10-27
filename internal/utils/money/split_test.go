package money

import "testing"

func TestEqualSplitReconciles(t *testing.T) {
	cases := []struct {
		total int64
		n     int
	}{
		{1500, 3}, // 500,500,500
		{2000, 4}, // 500 x4
		{1000, 3}, // 334,333,333
		{1, 3},    // 1,0,0
		{1201, 7}, // uneven
		{0, 4},    // all zero
	}
	for _, c := range cases {
		got := EqualSplit(c.total, c.n)
		if len(got) != c.n {
			t.Fatalf("EqualSplit(%d,%d) len=%d", c.total, c.n, len(got))
		}
		if s := Sum(got); s != c.total {
			t.Fatalf("EqualSplit(%d,%d) sum=%d, want %d", c.total, c.n, s, c.total)
		}
		// no two shares differ by more than 1 minor unit
		if got[0]-got[len(got)-1] > 1 {
			t.Fatalf("EqualSplit(%d,%d)=%v not balanced", c.total, c.n, got)
		}
	}
}

func TestPercentToAmountsReconciles(t *testing.T) {
	// 33.33/33.33/33.34 of 1000 -> must still sum to 1000
	got := PercentToAmounts(1000, []float64{33.33, 33.33, 33.34})
	if s := Sum(got); s != 1000 {
		t.Fatalf("PercentToAmounts sum=%d, want 1000 (%v)", s, got)
	}
	got2 := PercentToAmounts(999, []float64{50, 50})
	if s := Sum(got2); s != 999 {
		t.Fatalf("PercentToAmounts sum=%d, want 999 (%v)", s, got2)
	}
}
