package services

import (
	"testing"

	"github.com/google/uuid"

	"travel-service/internal/models"
)

func uptr(id uuid.UUID) *uuid.UUID { return &id }

func net(m map[partyKey]*balanceAgg, k partyKey) int64 {
	v := m[k]
	if v == nil {
		return 0
	}
	return v.paid - v.share + v.settledPaid - v.settledRecv
}

// Scenario C: external guide pays 2000, four travelers share 500 each.
func TestExternalPayerNotAutoShared(t *testing.T) {
	guide := uuid.New()
	u := []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New()}

	expenses := []models.Expense{{Amount: 2000, PaidByExternalID: uptr(guide)}}
	var shares []models.ListTripSharesRow
	for _, id := range u {
		shares = append(shares, models.ListTripSharesRow{UserID: uptr(id), Amount: 500})
	}

	m := aggregateBalances(expenses, shares, nil)
	if got := net(m, partyKey{false, guide}); got != 2000 {
		t.Fatalf("guide net = %d, want 2000 (payer, no auto share)", got)
	}
	for _, id := range u {
		if got := net(m, partyKey{true, id}); got != -500 {
			t.Fatalf("traveler net = %d, want -500", got)
		}
	}
}

// Scenario D: collaborator pays 1200, three people (incl. payer) share 400 each.
func TestPayerAlsoShares(t *testing.T) {
	payer := uuid.New()
	b, c := uuid.New(), uuid.New()
	expenses := []models.Expense{{Amount: 1200, PaidByUserID: uptr(payer)}}
	shares := []models.ListTripSharesRow{
		{UserID: uptr(payer), Amount: 400},
		{UserID: uptr(b), Amount: 400},
		{UserID: uptr(c), Amount: 400},
	}
	m := aggregateBalances(expenses, shares, nil)
	if got := net(m, partyKey{true, payer}); got != 800 {
		t.Fatalf("payer net = %d, want 800 (1200 paid - 400 share)", got)
	}
	if got := net(m, partyKey{true, b}); got != -400 {
		t.Fatalf("b net = %d, want -400", got)
	}
}

// Only completed settlements move balances; pending ones do not (rule 11).
func TestSettlementStatusAffectsBalance(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	expenses := []models.Expense{{Amount: 1000, PaidByUserID: uptr(a)}}
	shares := []models.ListTripSharesRow{
		{UserID: uptr(a), Amount: 500},
		{UserID: uptr(b), Amount: 500},
	}
	// b owes 500, a is owed 500.
	pending := []models.Settlement{{FromUserID: uptr(b), ToUserID: uptr(a), Amount: 500, Status: "pending"}}
	m := aggregateBalances(expenses, shares, pending)
	if got := net(m, partyKey{true, b}); got != -500 {
		t.Fatalf("b net with pending settlement = %d, want -500 (unchanged)", got)
	}

	completed := []models.Settlement{{FromUserID: uptr(b), ToUserID: uptr(a), Amount: 500, Status: "completed"}}
	m = aggregateBalances(expenses, shares, completed)
	if got := net(m, partyKey{true, b}); got != 0 {
		t.Fatalf("b net after completed settlement = %d, want 0", got)
	}
	if got := net(m, partyKey{true, a}); got != 0 {
		t.Fatalf("a net after completed settlement = %d, want 0", got)
	}
}
