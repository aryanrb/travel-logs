// Package money handles split math in integer minor units (e.g. cents, paisa).
// Working in integers keeps "allocated shares must equal the total" exact —
// no float rounding drift. See core business rule 10.
package money

import (
	"math"
	"sort"
)

// EqualSplit divides total across n parties. Any indivisible remainder (in
// minor units) is spread one unit at a time across the first `remainder`
// parties, so the sum of the result always equals total exactly.
func EqualSplit(total int64, n int) []int64 {
	out := make([]int64, n)
	if n <= 0 {
		return out
	}
	base := total / int64(n)
	rem := total % int64(n)
	for i := range out {
		out[i] = base
		if int64(i) < rem {
			out[i]++
		}
	}
	return out
}

// PercentToAmounts converts percentages into minor-unit amounts summing to
// total, using the largest-remainder method so the result reconciles exactly.
// Caller must validate that percents sum to ~100 first.
func PercentToAmounts(total int64, percents []float64) []int64 {
	amounts := make([]int64, len(percents))
	type frac struct {
		idx int
		f   float64
	}
	fracs := make([]frac, len(percents))
	var allocated int64
	for i, p := range percents {
		exact := float64(total) * p / 100.0
		fl := int64(math.Floor(exact))
		amounts[i] = fl
		allocated += fl
		fracs[i] = frac{i, exact - float64(fl)}
	}
	leftover := total - allocated
	sort.SliceStable(fracs, func(a, b int) bool { return fracs[a].f > fracs[b].f })
	for k := int64(0); k < leftover && int(k) < len(fracs); k++ {
		amounts[fracs[k].idx]++
	}
	return amounts
}

// Sum adds up minor-unit amounts.
func Sum(xs []int64) int64 {
	var t int64
	for _, x := range xs {
		t += x
	}
	return t
}
