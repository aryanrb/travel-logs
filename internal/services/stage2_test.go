package services

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"travel-service/internal/models"
	"travel-service/internal/types"
)

// ─── Pure unit checks (no network) ───────────────────────────────────────────

func TestConvertMinorUnits(t *testing.T) {
	r := map[string]float64{"USD": 1, "NPR": 133}
	if v, ok := convertMinorUnits(100, r, "USD", "USD"); !ok || v != 100 {
		t.Fatalf("identity = %d,%v", v, ok)
	}
	if v, ok := convertMinorUnits(100, r, "USD", "NPR"); !ok || v != 13300 {
		t.Fatalf("USD→NPR = %d,%v, want 13300", v, ok)
	}
	if v, ok := convertMinorUnits(13300, r, "NPR", "USD"); !ok || v != 100 {
		t.Fatalf("NPR→USD = %d,%v, want 100", v, ok)
	}
	if _, ok := convertMinorUnits(100, r, "USD", "XYZ"); ok {
		t.Fatalf("unknown currency must be ok=false")
	}
}

func TestDecodeRates(t *testing.T) {
	if m, ok := decodeRates([]byte(`{"result":"success","rates":{"USD":1,"NPR":133}}`)); !ok || m["NPR"] != 133 {
		t.Fatalf("decode = %v,%v", m, ok)
	}
	if _, ok := decodeRates([]byte(`{"result":"error","rates":{}}`)); ok {
		t.Fatalf("error result must be ok=false")
	}
	if _, ok := decodeRates([]byte(`nonsense`)); ok {
		t.Fatalf("garbage must be ok=false")
	}
}

func seedRates() {
	rates.mu.Lock()
	rates.perUSD = map[string]float64{"USD": 1, "NPR": 133}
	rates.day = time.Now().UTC().Format("2006-01-02")
	rates.mu.Unlock()
}

func ids(res *types.PublicTripListResponse) []uuid.UUID {
	out := make([]uuid.UUID, len(res.Trips))
	for i, it := range res.Trips {
		out[i] = it.Trip.ID
	}
	return out
}

// ─── DB-integration checks (scoped to each test via a unique q nonce) ─────────

// Day filter is inclusive (start+end both counted); trips without both dates are
// excluded when a day filter is set.
func TestExploreDaysFilter(t *testing.T) {
	e := newPubEnv(t)
	svc := NewPublicService()
	owner := e.user("owner")
	nonce := "EXPL" + uuid.NewString()

	four := e.trip(owner, nonce+" four", tp(d(2026, 11, 12)), tp(d(2026, 11, 15))) // 4 days
	nodate := e.trip(owner, nonce+" nodate", nil, nil)
	e.publish(four, false)
	e.publish(nodate, false)

	min, max := 4, 4
	res, err := svc.ListTrips(e.ctx, types.ExploreParams{Q: nonce, MinDays: &min, MaxDays: &max}, nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	got := ids(res)
	if len(got) != 1 || got[0] != four {
		t.Fatalf("min=max=4 days → %v, want just the 4-day trip (no-date excluded)", got)
	}

	min5 := 5
	res, _ = svc.ListTrips(e.ctx, types.ExploreParams{Q: nonce, MinDays: &min5}, nil)
	if len(res.Trips) != 0 {
		t.Fatalf("min_days=5 should exclude the 4-day trip, got %d", len(res.Trips))
	}
}

// cost_per_person = total ÷ travellers; null when private; cost filter converts
// into the requested currency before comparing; private trips excluded under a
// cost filter.
func TestExploreCostAndConversion(t *testing.T) {
	e := newPubEnv(t)
	svc := NewPublicService()
	seedRates()

	owner := e.user("owner")
	collab := e.user("collab")
	nonce := "EXPL" + uuid.NewString()

	// travellers = 2 (owner + 1 collaborator), total 20000 → cpp 10000 (USD cents).
	pubTrip := e.trip(owner, nonce+" pub", nil, nil)
	e.participant(pubTrip, collab, "collaborator", "accepted")
	e.expense(pubTrip, owner, 15000, "food", false)
	e.expense(pubTrip, owner, 5000, "food", false)
	e.publish(pubTrip, true)

	// a private-costs trip, same spend.
	privTrip := e.trip(owner, nonce+" priv", nil, nil)
	e.expense(privTrip, owner, 20000, "food", false)
	e.publish(privTrip, false)

	res, err := svc.ListTrips(e.ctx, types.ExploreParams{Q: nonce}, nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var pub, priv *types.PublicTripListItem
	for i := range res.Trips {
		switch res.Trips[i].Trip.ID {
		case pubTrip:
			pub = &res.Trips[i]
		case privTrip:
			priv = &res.Trips[i]
		}
	}
	if pub == nil || pub.CostPerPerson == nil || *pub.CostPerPerson != 10000 {
		t.Fatalf("public cost_per_person = %v, want 10000", pub)
	}
	if priv == nil || priv.CostPerPerson != nil {
		t.Fatalf("private trip cost_per_person must be null")
	}
	if pub.CostPerPersonConverted != nil {
		t.Fatalf("no converted cost without a currency param")
	}

	// With currency=NPR: converted = 10000 * 133 = 1,330,000 paisa.
	res, _ = svc.ListTrips(e.ctx, types.ExploreParams{Q: nonce, Currency: "NPR"}, nil)
	for _, it := range res.Trips {
		if it.Trip.ID == pubTrip {
			if it.CostPerPersonConverted == nil || it.CostPerPersonConverted.Amount != 1330000 || it.CostPerPersonConverted.Currency != "NPR" {
				t.Fatalf("converted = %+v, want 1330000 NPR", it.CostPerPersonConverted)
			}
		}
	}

	// Cost filter in NPR: min 1,330,000 includes the public trip; 1,330,001 excludes.
	min := int64(1330000)
	res, _ = svc.ListTrips(e.ctx, types.ExploreParams{Q: nonce, Currency: "NPR", MinCostPerPerson: &min}, nil)
	if got := ids(res); len(got) != 1 || got[0] != pubTrip {
		t.Fatalf("cost filter min=1330000 NPR → %v, want just the public trip (private excluded under cost filter)", got)
	}
	tooHigh := int64(1330001)
	res, _ = svc.ListTrips(e.ctx, types.ExploreParams{Q: nonce, Currency: "NPR", MinCostPerPerson: &tooHigh}, nil)
	if len(res.Trips) != 0 {
		t.Fatalf("cost filter above the cost should exclude it, got %d", len(res.Trips))
	}
}

// Sorting by length and start date, over the same three trips.
func TestExploreSort(t *testing.T) {
	e := newPubEnv(t)
	svc := NewPublicService()
	owner := e.user("owner")
	nonce := "EXPL" + uuid.NewString()

	x := e.trip(owner, nonce+" x", tp(d(2026, 11, 10)), tp(d(2026, 11, 11))) // 2 days, starts Nov 10
	y := e.trip(owner, nonce+" y", tp(d(2026, 11, 1)), tp(d(2026, 11, 6)))   // 6 days, starts Nov 1
	z := e.trip(owner, nonce+" z", tp(d(2026, 11, 5)), tp(d(2026, 11, 8)))   // 4 days, starts Nov 5
	for _, id := range []uuid.UUID{x, y, z} {
		e.publish(id, false)
	}

	shortest, _ := svc.ListTrips(e.ctx, types.ExploreParams{Q: nonce, Sort: "shortest"}, nil)
	if got := ids(shortest); len(got) != 3 || got[0] != x || got[2] != y {
		t.Fatalf("shortest = %v, want [x z y]", got)
	}
	longest, _ := svc.ListTrips(e.ctx, types.ExploreParams{Q: nonce, Sort: "longest"}, nil)
	if got := ids(longest); got[0] != y || got[2] != x {
		t.Fatalf("longest = %v, want [y z x]", got)
	}
	soonest, _ := svc.ListTrips(e.ctx, types.ExploreParams{Q: nonce, Sort: "soonest"}, nil)
	if got := ids(soonest); got[0] != y || got[1] != z || got[2] != x {
		t.Fatalf("soonest = %v, want [y z x]", got)
	}
}

// Paging walks the whole set one item at a time via next_cursor with no dups.
func TestExplorePaging(t *testing.T) {
	e := newPubEnv(t)
	svc := NewPublicService()
	owner := e.user("owner")
	nonce := "EXPL" + uuid.NewString()

	want := map[uuid.UUID]bool{}
	for i := 0; i < 3; i++ {
		id := e.trip(owner, nonce+" p", nil, nil)
		e.publish(id, false)
		want[id] = true
	}

	seen := map[uuid.UUID]bool{}
	cursor := ""
	for pages := 0; pages < 10; pages++ {
		res, err := svc.ListTrips(e.ctx, types.ExploreParams{Q: nonce, Limit: 1, Cursor: cursor}, nil)
		if err != nil {
			t.Fatalf("page: %v", err)
		}
		if len(res.Trips) != 1 {
			t.Fatalf("page size = %d, want 1", len(res.Trips))
		}
		id := res.Trips[0].Trip.ID
		if seen[id] {
			t.Fatalf("duplicate trip across pages: %v", id)
		}
		seen[id] = true
		if res.NextCursor == nil {
			break
		}
		cursor = *res.NextCursor
	}
	if len(seen) != 3 {
		t.Fatalf("paged %d trips, want 3", len(seen))
	}
}

// destination matches start_location or any destination name; `following` is
// ignored signed-out and (until stage 4) returns nothing signed-in.
func TestExploreDestinationAndFollowing(t *testing.T) {
	e := newPubEnv(t)
	svc := NewPublicService()
	owner := e.user("owner")
	viewer := e.user("viewer")
	nonce := "EXPL" + uuid.NewString()

	trip := e.trip(owner, nonce+" trip", nil, nil)
	if _, err := e.q.InsertDestination(e.ctx, models.InsertDestinationParams{TripID: trip, Name: "Pokhara"}); err != nil {
		t.Fatalf("dest: %v", err)
	}
	e.publish(trip, false)

	hit, _ := svc.ListTrips(e.ctx, types.ExploreParams{Q: nonce, Destination: "pokh"}, nil)
	if len(hit.Trips) != 1 {
		t.Fatalf("destination match should find the trip, got %d", len(hit.Trips))
	}
	miss, _ := svc.ListTrips(e.ctx, types.ExploreParams{Q: nonce, Destination: "paris"}, nil)
	if len(miss.Trips) != 0 {
		t.Fatalf("non-matching destination should exclude, got %d", len(miss.Trips))
	}

	// following ignored when signed out.
	out, _ := svc.ListTrips(e.ctx, types.ExploreParams{Q: nonce, Following: true}, nil)
	if len(out.Trips) != 1 {
		t.Fatalf("following must be ignored signed-out, got %d", len(out.Trips))
	}
	// following signed-in follows nobody yet (stage 4) → empty.
	in, _ := svc.ListTrips(e.ctx, types.ExploreParams{Q: nonce, Following: true}, &viewer)
	if len(in.Trips) != 0 {
		t.Fatalf("following with no follows should be empty, got %d", len(in.Trips))
	}
}
