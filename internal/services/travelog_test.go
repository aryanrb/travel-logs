package services

import (
	"testing"

	"travel-service/internal/models"
	"travel-service/internal/types"
)

// decodeNominatim parses lat/lng without the network.
func TestDecodeNominatim(t *testing.T) {
	c, ok := decodeNominatim([]byte(`[{"lat":"27.7172","lon":"85.3240"}]`))
	if !ok || c.lat < 27 || c.lat > 28 || c.lng < 85 || c.lng > 86 {
		t.Fatalf("decode = %+v ok=%v, want Kathmandu-ish", c, ok)
	}
	if _, ok := decodeNominatim([]byte(`[]`)); ok {
		t.Fatalf("empty result must be ok=false")
	}
	if _, ok := decodeNominatim([]byte(`not json`)); ok {
		t.Fatalf("garbage must be ok=false")
	}
}

// Deleting an entry keeps its expenses; the link becomes null (ON DELETE SET NULL).
func TestEntryDeleteKeepsExpenses(t *testing.T) {
	e := newPubEnv(t)
	tl := NewTimelineService()
	exp := NewExpenseService()

	owner := e.user("owner")
	trip := e.trip(owner, "Keep", nil, nil)

	entry, err := tl.CreateEntry(e.ctx, trip, owner, types.EntryInput{
		Title: "Day", EntryDate: types.Date(d(2026, 3, 2)),
		Expense: &types.ExpenseInput{
			Title: "Dinner", Amount: 1000, Date: types.Date(d(2026, 3, 2)),
			PaidBy: types.PartyRef{UserID: uptr(owner)}, SplitMethod: "equal",
			SharedAmong: []types.ShareInput{{UserID: uptr(owner)}},
		},
	})
	if err != nil {
		t.Fatalf("create entry+expense: %v", err)
	}
	if err := tl.DeleteEntry(e.ctx, trip, entry.ID, owner); err != nil {
		t.Fatalf("delete entry: %v", err)
	}
	list, err := exp.List(e.ctx, trip, owner)
	if err != nil {
		t.Fatalf("list expenses: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expense count = %d, want 1 (kept after entry delete)", len(list))
	}
	if list[0].EntryID != nil {
		t.Fatalf("entry_id must be null after entry delete, got %v", *list[0].EntryID)
	}
}

// An invalid nested expense rolls the whole operation back: no entry, no expense.
func TestInvalidNestedExpenseRollsBack(t *testing.T) {
	e := newPubEnv(t)
	tl := NewTimelineService()

	owner := e.user("owner")
	trip := e.trip(owner, "Rollback", nil, nil)

	_, err := tl.CreateEntry(e.ctx, trip, owner, types.EntryInput{
		Title: "Day", EntryDate: types.Date(d(2026, 3, 2)),
		Expense: &types.ExpenseInput{
			Title: "Bad", Amount: 1000, Date: types.Date(d(2026, 3, 2)),
			PaidBy: types.PartyRef{UserID: uptr(owner)}, SplitMethod: "equal",
			SharedAmong: nil, // no shares → invalid → rollback
		},
	})
	if err == nil {
		t.Fatalf("expected an error for the invalid nested expense")
	}
	if ents, _ := e.q.ListEntries(e.ctx, trip); len(ents) != 0 {
		t.Fatalf("entry must not be created when nested expense is invalid: %d", len(ents))
	}
	if exps, _ := e.q.ListExpenses(e.ctx, trip); len(exps) != 0 {
		t.Fatalf("expense must not be created: %d", len(exps))
	}
}

// Budget: set, replace and clear; 403 for non-owners; 422 for bad input.
func TestTripBudget(t *testing.T) {
	e := newPubEnv(t)
	ts := NewTripService()

	owner := e.user("owner")
	collab := e.user("collab")
	trip := e.trip(owner, "Budget", nil, nil)
	e.participant(trip, collab, "collaborator", "accepted")

	amt := int64(500000)
	tr, err := ts.SetBudget(e.ctx, trip, owner, types.BudgetInput{
		BudgetAmount: &amt,
		CategoryBudgets: []types.CategoryBudgetInput{
			{Category: "food", Amount: 200000}, {Category: "transport", Amount: 100000},
		},
	})
	if err != nil {
		t.Fatalf("set budget: %v", err)
	}
	if tr.BudgetAmount == nil || *tr.BudgetAmount != 500000 || len(tr.CategoryBudgets) != 2 {
		t.Fatalf("set result wrong: amount=%v cats=%d", tr.BudgetAmount, len(tr.CategoryBudgets))
	}

	// Replace entirely.
	amt2 := int64(600000)
	tr, err = ts.SetBudget(e.ctx, trip, owner, types.BudgetInput{
		BudgetAmount:    &amt2,
		CategoryBudgets: []types.CategoryBudgetInput{{Category: "food", Amount: 300000}},
	})
	if err != nil {
		t.Fatalf("replace budget: %v", err)
	}
	if *tr.BudgetAmount != 600000 || len(tr.CategoryBudgets) != 1 {
		t.Fatalf("replace result wrong: amount=%v cats=%d", *tr.BudgetAmount, len(tr.CategoryBudgets))
	}

	// Clear.
	tr, err = ts.SetBudget(e.ctx, trip, owner, types.BudgetInput{})
	if err != nil {
		t.Fatalf("clear budget: %v", err)
	}
	if tr.BudgetAmount != nil || len(tr.CategoryBudgets) != 0 {
		t.Fatalf("clear result wrong: amount=%v cats=%d", tr.BudgetAmount, len(tr.CategoryBudgets))
	}

	// 403 non-owner (collaborator).
	_, err = ts.SetBudget(e.ctx, trip, collab, types.BudgetInput{BudgetAmount: &amt})
	wantStatus(t, err, 403)

	// 422s: bad amount, duplicate category, unknown category.
	bad := int64(-5)
	_, err = ts.SetBudget(e.ctx, trip, owner, types.BudgetInput{BudgetAmount: &bad})
	wantStatus(t, err, 422)
	_, err = ts.SetBudget(e.ctx, trip, owner, types.BudgetInput{
		CategoryBudgets: []types.CategoryBudgetInput{{Category: "food", Amount: 1}, {Category: "food", Amount: 2}},
	})
	wantStatus(t, err, 422)
	_, err = ts.SetBudget(e.ctx, trip, owner, types.BudgetInput{
		CategoryBudgets: []types.CategoryBudgetInput{{Category: "spaceship", Amount: 1}},
	})
	wantStatus(t, err, 422)
}

// The trip's budget_amount surfaces as public budget.planned when costs are public.
func TestPublicBudgetPlanned(t *testing.T) {
	e := newPubEnv(t)
	ts := NewTripService()
	pub := NewPublicService()

	owner := e.user("owner")
	trip := e.trip(owner, "Planned", nil, nil)
	amt := int64(1000000)
	if _, err := ts.SetBudget(e.ctx, trip, owner, types.BudgetInput{BudgetAmount: &amt}); err != nil {
		t.Fatalf("set budget: %v", err)
	}
	e.publish(trip, true)

	detail, err := pub.GetTrip(e.ctx, trip)
	if err != nil {
		t.Fatalf("public get: %v", err)
	}
	if detail.Budget == nil || detail.Budget.Planned == nil || *detail.Budget.Planned != 1000000 {
		t.Fatalf("public budget.planned wrong: %+v", detail.Budget)
	}
}

// Saved trips are idempotent, public-only, and 404 when the trip isn't public.
func TestSavedTrips(t *testing.T) {
	e := newPubEnv(t)
	pub := NewPublicService()

	owner := e.user("owner")
	saver := e.user("saver")
	trip := e.trip(owner, "Pub", nil, nil)
	e.publish(trip, false)

	if err := pub.Save(e.ctx, trip, saver); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := pub.Save(e.ctx, trip, saver); err != nil {
		t.Fatalf("save again (idempotent): %v", err)
	}
	if list, _ := pub.ListSaved(e.ctx, saver); len(list.Trips) != 1 {
		t.Fatalf("saved count = %d, want 1 (idempotent)", len(list.Trips))
	}

	if err := pub.Unsave(e.ctx, trip, saver); err != nil {
		t.Fatalf("unsave: %v", err)
	}
	if err := pub.Unsave(e.ctx, trip, saver); err != nil {
		t.Fatalf("unsave again (idempotent): %v", err)
	}
	if list, _ := pub.ListSaved(e.ctx, saver); len(list.Trips) != 0 {
		t.Fatalf("saved count = %d, want 0 after unsave", len(list.Trips))
	}

	// Re-save, then make private: hidden from the list and save now 404s.
	_ = pub.Save(e.ctx, trip, saver)
	if err := e.q.SetTripPublic(e.ctx, models.SetTripPublicParams{IsPublic: false, ShowCostsPublic: false, ID: trip}); err != nil {
		t.Fatalf("unpublish: %v", err)
	}
	if list, _ := pub.ListSaved(e.ctx, saver); len(list.Trips) != 0 {
		t.Fatalf("no-longer-public trip must be hidden from saved: %d", len(list.Trips))
	}
	wantStatus(t, pub.Save(e.ctx, trip, saver), 404)
}

// Foreign-currency fields must be supplied all-three-or-none and are returned on
// the expense; rate > 0 and original currency must differ from the trip's.
func TestExpenseCurrencyFields(t *testing.T) {
	e := newPubEnv(t)
	exp := NewExpenseService()

	owner := e.user("owner")
	trip := e.trip(owner, "Cur", nil, nil) // trip currency USD

	base := func() types.ExpenseInput {
		return types.ExpenseInput{
			Title: "X", Amount: 1000, Date: types.Date(d(2026, 3, 2)),
			PaidBy: types.PartyRef{UserID: uptr(owner)}, SplitMethod: "equal",
			SharedAmong: []types.ShareInput{{UserID: uptr(owner)}},
		}
	}
	npr, amt, rate := "NPR", int64(133000), 0.0075

	// All three → stored and returned (also exercises numeric↔float64 round-trip).
	in := base()
	in.OriginalCurrency, in.OriginalAmount, in.ExchangeRate = &npr, &amt, &rate
	res, err := exp.Create(e.ctx, trip, owner, in)
	if err != nil {
		t.Fatalf("create with currency: %v", err)
	}
	if res.Expense.OriginalCurrency == nil || *res.Expense.OriginalCurrency != "NPR" ||
		res.Expense.OriginalAmount == nil || *res.Expense.OriginalAmount != 133000 ||
		res.Expense.ExchangeRate == nil || *res.Expense.ExchangeRate <= 0 {
		t.Fatalf("currency fields not returned: %+v", res.Expense)
	}

	// Only two of three → 422.
	in2 := base()
	in2.OriginalCurrency, in2.OriginalAmount = &npr, &amt
	_, err = exp.Create(e.ctx, trip, owner, in2)
	wantStatus(t, err, 422)

	// rate <= 0 → 422.
	zero := 0.0
	in3 := base()
	in3.OriginalCurrency, in3.OriginalAmount, in3.ExchangeRate = &npr, &amt, &zero
	_, err = exp.Create(e.ctx, trip, owner, in3)
	wantStatus(t, err, 422)

	// original currency same as trip's → 422.
	usd := "USD"
	in4 := base()
	in4.OriginalCurrency, in4.OriginalAmount, in4.ExchangeRate = &usd, &amt, &rate
	_, err = exp.Create(e.ctx, trip, owner, in4)
	wantStatus(t, err, 422)
}

// Events are written for each change and paged with `before`, newest first.
func TestActivityFeed(t *testing.T) {
	e := newPubEnv(t)
	tl := NewTimelineService()
	ev := NewEventService()

	owner := e.user("owner")
	trip := e.trip(owner, "Feed", nil, nil)

	for i := 0; i < 3; i++ {
		if _, err := tl.CreateEntry(e.ctx, trip, owner, types.EntryInput{Title: "E", EntryDate: types.Date(d(2026, 3, 2))}); err != nil {
			t.Fatalf("create entry: %v", err)
		}
	}

	res, err := ev.List(e.ctx, trip, owner, nil)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	if len(res.Events) < 3 {
		t.Fatalf("events = %d, want >= 3", len(res.Events))
	}
	newest := res.Events[0]
	if newest.SubjectType != "entry" || newest.Action != "created" || newest.ActorName != "owner" {
		t.Fatalf("newest event wrong: type=%s action=%s actor=%s", newest.SubjectType, newest.Action, newest.ActorName)
	}
	for i := 1; i < len(res.Events); i++ {
		if res.Events[i-1].CreatedAt.Before(res.Events[i].CreatedAt) {
			t.Fatalf("events are not newest-first")
		}
	}

	before := newest.CreatedAt
	older, err := ev.List(e.ctx, trip, owner, &before)
	if err != nil {
		t.Fatalf("list before: %v", err)
	}
	if len(older.Events) >= len(res.Events) {
		t.Fatalf("before cursor did not drop any events: %d vs %d", len(older.Events), len(res.Events))
	}
	for _, evt := range older.Events {
		if !evt.CreatedAt.Before(before) {
			t.Fatalf("before paging returned an event at/after the cursor")
		}
	}
}
