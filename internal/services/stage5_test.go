package services

import (
	"testing"

	"github.com/google/uuid"

	"travel-service/internal/types"
)

func contribInput(from, to uuid.UUID, amount int64, status, kind string) types.SettlementInput {
	return types.SettlementInput{
		From: types.PartyRef{UserID: uptr(from)}, To: types.PartyRef{UserID: uptr(to)},
		Amount: amount, SettledDate: types.Date(d(2026, 3, 2)), Status: &status, Kind: &kind,
	}
}

// Pool create/read/delete, owner-only, collector must be a member.
func TestPoolLifecycle(t *testing.T) {
	e := newPubEnv(t)
	ps := NewPoolService()
	owner := e.user("owner")
	collector := e.user("collector")
	stranger := e.user("stranger")
	trip := e.trip(owner, "Pool", nil, nil)
	e.participant(trip, collector, "collaborator", "accepted")

	due := types.Date(d(2026, 3, 1))
	res, err := ps.Set(e.ctx, trip, owner, types.PoolInput{CollectorID: collector, PerPerson: 500000, DueDate: &due})
	if err != nil {
		t.Fatalf("set pool: %v", err)
	}
	if !res.Enabled || res.CollectorID != collector || res.PerPerson != 500000 || res.Collected != 0 || res.Spent != 0 {
		t.Fatalf("pool response wrong: %+v", res)
	}

	// Non-owner → 403.
	_, err = ps.Set(e.ctx, trip, collector, types.PoolInput{CollectorID: collector, PerPerson: 1000})
	wantStatus(t, err, 403)
	// Collector not a member → 422.
	_, err = ps.Set(e.ctx, trip, owner, types.PoolInput{CollectorID: stranger, PerPerson: 1000})
	wantStatus(t, err, 422)
	// per_person <= 0 → 422.
	_, err = ps.Set(e.ctx, trip, owner, types.PoolInput{CollectorID: collector, PerPerson: 0})
	wantStatus(t, err, 422)

	if err := ps.Delete(e.ctx, trip, owner); err != nil {
		t.Fatalf("delete pool: %v", err)
	}
	_, err = ps.Get(e.ctx, trip, owner)
	wantStatus(t, err, 404)
}

// A pending contribution notifies the collector and doesn't count until confirmed;
// confirm is limited to the receiver or the owner.
func TestPoolContributionConfirm(t *testing.T) {
	e := newPubEnv(t)
	ps := NewPoolService()
	ss := NewSettlementService()
	owner := e.user("owner")
	collector := e.user("collector")
	member := e.user("member")
	trip := e.trip(owner, "Contrib", nil, nil)
	e.participant(trip, collector, "collaborator", "accepted")
	e.participant(trip, member, "collaborator", "accepted")
	if _, err := ps.Set(e.ctx, trip, owner, types.PoolInput{CollectorID: collector, PerPerson: 500000}); err != nil {
		t.Fatalf("set pool: %v", err)
	}

	st, err := ss.Create(e.ctx, trip, member, contribInput(member, collector, 300000, "pending", "contribution"))
	if err != nil {
		t.Fatalf("contribute: %v", err)
	}
	if st.Kind != "contribution" || st.Status != "pending" {
		t.Fatalf("settlement kind/status wrong: %+v", st)
	}
	if pool, _ := ps.Get(e.ctx, trip, owner); pool.Collected != 0 {
		t.Fatalf("pending contribution must not count: collected=%d", pool.Collected)
	}
	if !hasType(e.notifTypes(collector), "pool_contribution") {
		t.Fatalf("collector should get a pool_contribution notification")
	}

	// A non-receiver, non-owner member can't confirm.
	wantStatus(t, ss.Confirm(e.ctx, trip, st.ID, member), 403)
	// The collector (receiver) can.
	if err := ss.Confirm(e.ctx, trip, st.ID, collector); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if pool, _ := ps.Get(e.ctx, trip, owner); pool.Collected != 300000 {
		t.Fatalf("confirmed contribution should count: collected=%d", pool.Collected)
	}
}

// Contributions must target the collector and need a pool.
func TestContributionValidation(t *testing.T) {
	e := newPubEnv(t)
	ps := NewPoolService()
	ss := NewSettlementService()
	owner := e.user("owner")
	collector := e.user("collector")
	member := e.user("member")
	trip := e.trip(owner, "CV", nil, nil)
	e.participant(trip, collector, "collaborator", "accepted")
	e.participant(trip, member, "collaborator", "accepted")

	// No pool yet → contribution 422.
	_, err := ss.Create(e.ctx, trip, member, contribInput(member, collector, 1000, "completed", "contribution"))
	wantStatus(t, err, 422)

	if _, err := ps.Set(e.ctx, trip, owner, types.PoolInput{CollectorID: collector, PerPerson: 500000}); err != nil {
		t.Fatalf("set pool: %v", err)
	}
	// Contribution to a non-collector → 422.
	_, err = ss.Create(e.ctx, trip, member, contribInput(member, owner, 1000, "completed", "contribution"))
	wantStatus(t, err, 422)
}

// from_pool expenses must be paid by the collector and need a pool; `spent` tracks them.
func TestPoolExpenseFromPool(t *testing.T) {
	e := newPubEnv(t)
	ps := NewPoolService()
	es := NewExpenseService()
	owner := e.user("owner")
	collector := e.user("collector")
	member := e.user("member")
	trip := e.trip(owner, "Spend", nil, nil)
	e.participant(trip, collector, "collaborator", "accepted")
	e.participant(trip, member, "collaborator", "accepted")
	if _, err := ps.Set(e.ctx, trip, owner, types.PoolInput{CollectorID: collector, PerPerson: 500000}); err != nil {
		t.Fatalf("set pool: %v", err)
	}

	poolExp := func(payer uuid.UUID) types.ExpenseInput {
		return types.ExpenseInput{
			Title: "Hotel", Amount: 80000, Date: types.Date(d(2026, 3, 2)),
			PaidBy: types.PartyRef{UserID: uptr(payer)}, SplitMethod: "equal",
			SharedAmong: []types.ShareInput{{UserID: uptr(collector)}, {UserID: uptr(member)}},
			FromPool:    true,
		}
	}

	res, err := es.Create(e.ctx, trip, collector, poolExp(collector))
	if err != nil {
		t.Fatalf("pool expense: %v", err)
	}
	if !res.Expense.FromPool {
		t.Fatalf("from_pool should be true on the created expense")
	}
	if pool, _ := ps.Get(e.ctx, trip, owner); pool.Spent != 80000 {
		t.Fatalf("spent should be 80000, got %d", pool.Spent)
	}

	// from_pool paid by a non-collector → 422.
	_, err = es.Create(e.ctx, trip, collector, poolExp(member))
	wantStatus(t, err, 422)

	// from_pool with no pool → 422.
	noPool := e.trip(owner, "NoPool", nil, nil)
	_, err = es.Create(e.ctx, noPool, owner, types.ExpenseInput{
		Title: "x", Amount: 1000, Date: types.Date(d(2026, 3, 2)),
		PaidBy: types.PartyRef{UserID: uptr(owner)}, SplitMethod: "equal",
		SharedAmong: []types.ShareInput{{UserID: uptr(owner)}}, FromPool: true,
	})
	wantStatus(t, err, 422)
}
