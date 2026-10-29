package services

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	db "travel-service/internal/database"
	"travel-service/internal/models"
	"travel-service/internal/types"
	"travel-service/internal/utils/apperror"
)

// These are DB-integration tests. They run against POSTGRESQL_URL (loaded from
// the repo .env) and clean up everything they create. They skip if no DB.

type pubEnv struct {
	t     *testing.T
	ctx   context.Context
	q     *models.Queries
	pool  *pgxpool.Pool
	trips []uuid.UUID
	users []uuid.UUID
}

func newPubEnv(t *testing.T) *pubEnv {
	t.Helper()
	_ = godotenv.Load("../../.env")
	url := os.Getenv("POSTGRESQL_URL")
	if url == "" {
		t.Skip("POSTGRESQL_URL not set; skipping DB integration test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Skipf("cannot open db: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("cannot reach db: %v", err)
	}
	db.Client = pool
	e := &pubEnv{t: t, ctx: ctx, q: models.New(pool), pool: pool}
	t.Cleanup(e.cleanup)
	return e
}

func (e *pubEnv) cleanup() {
	for _, id := range e.trips {
		_, _ = e.pool.Exec(e.ctx, "DELETE FROM trips WHERE id=$1", id)
	}
	for _, id := range e.users {
		_, _ = e.pool.Exec(e.ctx, "DELETE FROM users WHERE id=$1", id)
	}
	e.pool.Close()
}

func sp(s string) *string       { return &s }
func i64p(i int64) *int64       { return &i }
func tp(t time.Time) *time.Time { return &t }

func d(y int, m time.Month, day int) time.Time {
	return time.Date(y, m, day, 0, 0, 0, 0, time.UTC)
}

func sameDate(got *time.Time, want time.Time) bool {
	if got == nil {
		return false
	}
	gy, gm, gd := got.Date()
	wy, wm, wd := want.Date()
	return gy == wy && gm == wm && gd == wd
}

func (e *pubEnv) user(name string) uuid.UUID {
	e.t.Helper()
	u, err := e.q.InsertUser(e.ctx, models.InsertUserParams{
		Email: name + "-" + uuid.NewString() + "@test.local", PasswordHash: "x", Name: name,
	})
	if err != nil {
		e.t.Fatalf("insert user: %v", err)
	}
	e.users = append(e.users, u.ID)
	return u.ID
}

func (e *pubEnv) trip(owner uuid.UUID, title string, start, end *time.Time) uuid.UUID {
	e.t.Helper()
	tr, err := e.q.InsertTrip(e.ctx, models.InsertTripParams{
		OwnerID: owner, Title: title, Currency: "USD", StartDate: start, EndDate: end,
	})
	if err != nil {
		e.t.Fatalf("insert trip: %v", err)
	}
	e.trips = append(e.trips, tr.ID)
	return tr.ID
}

func (e *pubEnv) publish(id uuid.UUID, show bool) {
	e.t.Helper()
	if err := e.q.SetTripPublic(e.ctx, models.SetTripPublicParams{IsPublic: true, ShowCostsPublic: show, ID: id}); err != nil {
		e.t.Fatalf("publish: %v", err)
	}
}

func (e *pubEnv) expense(trip, payer uuid.UUID, amount int64, category string, hidden bool) uuid.UUID {
	e.t.Helper()
	exp, err := e.q.InsertExpense(e.ctx, models.InsertExpenseParams{
		TripID: trip, Title: "x", Amount: amount, Currency: "USD", Category: sp(category),
		ExpenseDate: d(2026, 3, 2), PaidByUserID: uptr(payer), RecordedBy: payer,
		SplitMethod: "equal", HideFromPublic: hidden,
	})
	if err != nil {
		e.t.Fatalf("insert expense: %v", err)
	}
	return exp.ID
}

func (e *pubEnv) participant(trip, user uuid.UUID, role, status string) {
	e.t.Helper()
	tok := uuid.NewString()
	if _, err := e.q.InsertParticipant(e.ctx, models.InsertParticipantParams{
		TripID: trip, UserID: uptr(user), Role: role, Status: status, InviteToken: &tok,
	}); err != nil {
		e.t.Fatalf("insert participant: %v", err)
	}
}

func wantStatus(t *testing.T, err error, code int) {
	t.Helper()
	var ae *apperror.Error
	if !errors.As(err, &ae) {
		t.Fatalf("expected *apperror.Error, got %v", err)
	}
	if ae.Status() != code {
		t.Fatalf("expected status %d, got %d (%v)", code, ae.Status(), err)
	}
}

// 1. Private trips 404 on both public endpoints.
func TestPublicPrivateTrips404(t *testing.T) {
	e := newPubEnv(t)
	svc := NewPublicService()

	owner := e.user("owner")
	private := e.trip(owner, "Secret Trip", nil, nil) // never published

	_, err := svc.GetTrip(e.ctx, private)
	wantStatus(t, err, 404)

	res, err := svc.ListTrips(e.ctx, "", "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, it := range res.Trips {
		if it.Trip.ID == private {
			t.Fatalf("private trip leaked into public list")
		}
	}
}

// 2. budget is null when show_costs_public is false, and excludes hidden expenses when true.
func TestPublicBudgetVisibility(t *testing.T) {
	e := newPubEnv(t)
	svc := NewPublicService()

	owner := e.user("owner")
	trip := e.trip(owner, "Budget Trip", nil, nil)
	e.publish(trip, false)
	e.expense(trip, owner, 100000, "food", false)

	detail, err := svc.GetTrip(e.ctx, trip)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if detail.Budget != nil {
		t.Fatalf("budget must be nil when show_costs_public is false")
	}

	e.publish(trip, true)
	e.expense(trip, owner, 50000, "food", true)       // hidden — excluded
	e.expense(trip, owner, 30000, "transport", false) // visible

	detail, err = svc.GetTrip(e.ctx, trip)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if detail.Budget == nil {
		t.Fatalf("budget must be present when show_costs_public is true")
	}
	if detail.Budget.Total != 130000 {
		t.Fatalf("budget total = %d, want 130000 (hidden 50000 excluded)", detail.Budget.Total)
	}
	if detail.Budget.Travellers != 1 {
		t.Fatalf("travellers = %d, want 1 (owner only)", detail.Budget.Travellers)
	}
}

// 3. people excludes viewers and external payers; no emails or ids leak.
func TestPublicPeopleRedaction(t *testing.T) {
	e := newPubEnv(t)
	svc := NewPublicService()

	owner := e.user("Owner")
	collab := e.user("Collab")
	viewer := e.user("Viewer")
	trip := e.trip(owner, "People Trip", nil, nil)
	e.publish(trip, false)
	e.participant(trip, collab, "collaborator", "accepted")
	e.participant(trip, viewer, "viewer", "accepted")
	if _, err := e.q.UpsertExternalPayer(e.ctx, models.UpsertExternalPayerParams{TripID: trip, Name: "Local Guide"}); err != nil {
		t.Fatalf("external: %v", err)
	}

	detail, err := svc.GetTrip(e.ctx, trip)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(detail.People) != 2 {
		t.Fatalf("people = %d, want 2 (owner + collaborator)", len(detail.People))
	}
	for _, p := range detail.People {
		if p.Role == "viewer" {
			t.Fatalf("viewer leaked into public people")
		}
		if p.Name == "Local Guide" {
			t.Fatalf("external payer leaked into public people")
		}
	}
	// PublicPerson carries only Name + Role — ids and emails cannot leak by shape.
}

// 4. Copy shifts dates, creates a private trip for the caller, and copies nothing
// financial or personal.
func TestPublicCopy(t *testing.T) {
	e := newPubEnv(t)
	svc := NewPublicService()

	owner := e.user("owner")
	caller := e.user("caller")

	src := e.trip(owner, "Source", tp(d(2026, 3, 1)), tp(d(2026, 3, 10)))
	e.publish(src, true)

	if _, err := e.q.InsertDestination(e.ctx, models.InsertDestinationParams{
		TripID: src, Name: "Pokhara", ArrivalDate: tp(d(2026, 3, 3)), SortOrder: 1,
	}); err != nil {
		t.Fatalf("dest: %v", err)
	}
	entry, err := e.q.InsertEntry(e.ctx, models.InsertEntryParams{
		TripID: src, Title: "Day 2", EntryDate: d(2026, 3, 2), CreatedBy: owner,
	})
	if err != nil {
		t.Fatalf("entry: %v", err)
	}
	expID := e.expense(src, owner, 50000, "transport", false)
	depart := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	if _, err := e.q.InsertTransport(e.ctx, models.InsertTransportParams{
		TripID: src, EntryID: uptr(entry.ID), ExpenseID: uptr(expID), Type: "car",
		CostAmount: i64p(50000), DepartAt: tp(depart), CreatedBy: owner,
	}); err != nil {
		t.Fatalf("transport: %v", err)
	}
	// Things that must NOT be copied:
	if _, err := e.q.InsertPhoto(e.ctx, models.InsertPhotoParams{TripID: src, Url: "http://x/p.jpg", CreatedBy: owner}); err != nil {
		t.Fatalf("photo: %v", err)
	}
	e.participant(src, caller, "collaborator", "accepted")
	if _, err := e.q.UpsertExternalPayer(e.ctx, models.UpsertExternalPayerParams{TripID: src, Name: "Guide"}); err != nil {
		t.Fatalf("external: %v", err)
	}

	newStart := d(2026, 3, 15) // delta = +14 days
	nt, err := svc.Copy(e.ctx, src, caller, types.CopyTripInput{Title: "My Copy", StartDate: types.Date(newStart)})
	if err != nil {
		t.Fatalf("copy: %v", err)
	}
	e.trips = append(e.trips, nt.ID)

	if nt.OwnerID != caller {
		t.Fatalf("copy owner = %v, want caller %v", nt.OwnerID, caller)
	}
	if nt.IsPublic {
		t.Fatalf("copy must be private")
	}
	if nt.Title != "My Copy" {
		t.Fatalf("copy title = %q", nt.Title)
	}
	if !sameDate(nt.StartDate, d(2026, 3, 15)) {
		t.Fatalf("copy start = %v, want 2026-03-15", nt.StartDate)
	}
	if !sameDate(nt.EndDate, d(2026, 3, 24)) {
		t.Fatalf("copy end = %v, want 2026-03-24 (shifted +14)", nt.EndDate)
	}

	// Content copied with shifted dates.
	nd, _ := e.q.ListDestinations(e.ctx, nt.ID)
	if len(nd) != 1 || nd[0].Name != "Pokhara" || !sameDate(nd[0].ArrivalDate, d(2026, 3, 17)) {
		t.Fatalf("destinations not copied/shifted: %+v", nd)
	}
	ne, _ := e.q.ListEntries(e.ctx, nt.ID)
	if len(ne) != 1 || ne[0].CreatedBy != caller || !sameDate(&ne[0].EntryDate, d(2026, 3, 16)) {
		t.Fatalf("entries not copied/shifted/reattributed: %+v", ne)
	}
	ntr, _ := e.q.ListTransport(e.ctx, nt.ID)
	if len(ntr) != 1 {
		t.Fatalf("transport count = %d, want 1", len(ntr))
	}
	j := ntr[0]
	if j.ExpenseID != nil {
		t.Fatalf("copied journey must not keep expense_id")
	}
	if j.CostAmount != nil {
		t.Fatalf("copied journey must not keep cost_amount")
	}
	if j.EntryID == nil || *j.EntryID != ne[0].ID {
		t.Fatalf("journey entry_id must remap to the copied entry")
	}
	if j.CreatedBy != caller {
		t.Fatalf("journey created_by must be caller")
	}
	if j.DepartAt == nil || !j.DepartAt.Equal(depart.Add(14*24*time.Hour)) {
		t.Fatalf("journey depart_at not shifted: %v", j.DepartAt)
	}

	// Nothing financial or personal copied.
	if ex, _ := e.q.ListExpenses(e.ctx, nt.ID); len(ex) != 0 {
		t.Fatalf("expenses must not be copied: %d", len(ex))
	}
	if ph, _ := e.q.ListTripPhotos(e.ctx, nt.ID); len(ph) != 0 {
		t.Fatalf("photos must not be copied: %d", len(ph))
	}
	if pp, _ := e.q.ListParticipants(e.ctx, nt.ID); len(pp) != 0 {
		t.Fatalf("participants must not be copied: %d", len(pp))
	}
	if xp, _ := e.q.ListExternalPayers(e.ctx, nt.ID); len(xp) != 0 {
		t.Fatalf("external payers must not be copied: %d", len(xp))
	}
}
