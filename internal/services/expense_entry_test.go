package services

import (
	"testing"

	"travel-service/internal/types"
)

// A nested expense on an entry is created linked to that entry, and a direct
// expense keeps the entry_id it was created with — both visible via list/get.
func TestExpenseEntryLink(t *testing.T) {
	e := newPubEnv(t)
	expSvc := NewExpenseService()
	tlSvc := NewTimelineService()

	owner := e.user("owner")
	trip := e.trip(owner, "Linky Trip", nil, nil)

	nested := types.ExpenseInput{
		Title:       "Dinner",
		Amount:      10000,
		Date:        types.Date(d(2026, 3, 2)),
		PaidBy:      types.PartyRef{UserID: uptr(owner)},
		SplitMethod: "equal",
		SharedAmong: []types.ShareInput{{UserID: uptr(owner)}},
	}

	// 1. Entry with a nested expense → expense links back to the entry.
	entry, err := tlSvc.CreateEntry(e.ctx, trip, owner, types.EntryInput{
		Title:     "Day 2",
		EntryDate: types.Date(d(2026, 3, 2)),
		Expense:   &nested,
	})
	if err != nil {
		t.Fatalf("create entry with nested expense: %v", err)
	}

	list, err := expSvc.List(e.ctx, trip, owner)
	if err != nil {
		t.Fatalf("list expenses: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expense count = %d, want 1", len(list))
	}
	if list[0].EntryID == nil || *list[0].EntryID != entry.ID {
		t.Fatalf("nested expense entry_id = %v, want %v", list[0].EntryID, entry.ID)
	}

	// 2. Direct expense created with entry_id → kept and returned on GET.
	direct := nested
	direct.Title = "Taxi"
	direct.EntryID = uptr(entry.ID)
	created, err := expSvc.Create(e.ctx, trip, owner, direct)
	if err != nil {
		t.Fatalf("create expense with entry_id: %v", err)
	}
	if created.Expense.EntryID == nil || *created.Expense.EntryID != entry.ID {
		t.Fatalf("create response entry_id = %v, want %v", created.Expense.EntryID, entry.ID)
	}
	got, err := expSvc.Get(e.ctx, trip, created.Expense.ID, owner)
	if err != nil {
		t.Fatalf("get expense: %v", err)
	}
	if got.Expense.EntryID == nil || *got.Expense.EntryID != entry.ID {
		t.Fatalf("get entry_id = %v, want %v", got.Expense.EntryID, entry.ID)
	}
}
