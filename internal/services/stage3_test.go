package services

import (
	"testing"

	"github.com/google/uuid"

	"travel-service/internal/models"
	"travel-service/internal/types"
)

func i32p(i int32) *int32 { return &i }

// link upserts an invite link for a trip and returns its token.
func (e *pubEnv) link(owner, trip uuid.UUID, in types.InviteLinkInput) string {
	e.t.Helper()
	res, err := NewRSVPService().UpsertLink(e.ctx, trip, owner, in)
	if err != nil {
		e.t.Fatalf("upsert link: %v", err)
	}
	return res.Token
}

func (e *pubEnv) participantRow(trip, user uuid.UUID) (models.TripParticipant, bool) {
	e.t.Helper()
	uid := user
	p, err := e.q.GetParticipantByTripUser(e.ctx, models.GetParticipantByTripUserParams{TripID: trip, UserID: &uid})
	if err != nil {
		return p, false
	}
	return p, true
}

func (e *pubEnv) notifTypes(user uuid.UUID) []string {
	e.t.Helper()
	rows, err := e.q.ListNotifications(e.ctx, models.ListNotificationsParams{UserID: user, Lim: 100})
	if err != nil {
		e.t.Fatalf("list notifications: %v", err)
	}
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Type
	}
	return out
}

func hasType(types []string, want string) bool {
	for _, t := range types {
		if t == want {
			return true
		}
	}
	return false
}

// Going grants accepted access with the link role; Maybe grants viewer access;
// changing away from Going revokes access.
func TestRSVPGoingMaybeDecline(t *testing.T) {
	e := newPubEnv(t)
	svc := NewRSVPService()
	owner := e.user("owner")
	a := e.user("a")
	b := e.user("b")
	trip := e.trip(owner, "RSVP", nil, nil)
	token := e.link(owner, trip, types.InviteLinkInput{Active: true, JoinMode: "anyone", Role: "collaborator"})

	ra, err := svc.Reply(e.ctx, token, a, "going")
	if err != nil || ra.Status != "going" {
		t.Fatalf("a going = %+v, %v", ra, err)
	}
	if p, ok := e.participantRow(trip, a); !ok || p.Role != "collaborator" || p.Status != "accepted" {
		t.Fatalf("a should be an accepted collaborator: %+v ok=%v", p, ok)
	}

	rb, err := svc.Reply(e.ctx, token, b, "maybe")
	if err != nil || rb.Status != "maybe" {
		t.Fatalf("b maybe = %+v, %v", rb, err)
	}
	if p, ok := e.participantRow(trip, b); !ok || p.Role != "viewer" || p.Status != "accepted" {
		t.Fatalf("b (maybe) should be an accepted viewer: %+v ok=%v", p, ok)
	}

	// a changes to declined → access revoked.
	if _, err := svc.Reply(e.ctx, token, a, "declined"); err != nil {
		t.Fatalf("a decline: %v", err)
	}
	if p, _ := e.participantRow(trip, a); p.Status != "revoked" {
		t.Fatalf("a access should be revoked after declining, got %q", p.Status)
	}

	// Owner received an 'rsvp' notification.
	if !hasType(e.notifTypes(owner), "rsvp") {
		t.Fatalf("owner should have an rsvp notification")
	}
}

// spots caps Going; the overflow waitlists; removing a Going promotes the earliest
// waitlisted person and notifies them.
func TestRSVPWaitlistAndPromotion(t *testing.T) {
	e := newPubEnv(t)
	svc := NewRSVPService()
	owner := e.user("owner")
	a := e.user("a")
	b := e.user("b")
	trip := e.trip(owner, "Waitlist", nil, nil)
	token := e.link(owner, trip, types.InviteLinkInput{Active: true, JoinMode: "anyone", Role: "viewer", Spots: i32p(1)})

	if r, _ := svc.Reply(e.ctx, token, a, "going"); r.Status != "going" {
		t.Fatalf("a should be going, got %q", r.Status)
	}
	rb, _ := svc.Reply(e.ctx, token, b, "going")
	if rb.Status != "waitlist" {
		t.Fatalf("b should be waitlisted (1 spot), got %q", rb.Status)
	}
	if _, ok := e.participantRow(trip, b); ok {
		if p, _ := e.participantRow(trip, b); p.Status == "accepted" {
			t.Fatalf("waitlisted b must not have accepted access")
		}
	}

	// Owner removes a → b is promoted.
	rsvps, _ := svc.ListRSVPs(e.ctx, trip, owner)
	var aRSVP uuid.UUID
	for _, r := range rsvps.RSVPs {
		if r.UserID == a {
			aRSVP = r.ID
		}
	}
	if err := svc.DeleteRSVP(e.ctx, trip, aRSVP, owner); err != nil {
		t.Fatalf("delete a: %v", err)
	}
	after, _ := svc.ListRSVPs(e.ctx, trip, owner)
	for _, r := range after.RSVPs {
		if r.UserID == b && r.Status != "going" {
			t.Fatalf("b should be promoted to going, got %q", r.Status)
		}
	}
	if p, ok := e.participantRow(trip, b); !ok || p.Status != "accepted" {
		t.Fatalf("promoted b should have accepted access: %+v", p)
	}
	if !hasType(e.notifTypes(b), "waitlist_promoted") {
		t.Fatalf("promoted b should get a waitlist_promoted notification")
	}
}

// Approval mode holds replies as `requested` until the organiser approves.
func TestRSVPApprovalMode(t *testing.T) {
	e := newPubEnv(t)
	svc := NewRSVPService()
	owner := e.user("owner")
	a := e.user("a")
	trip := e.trip(owner, "Approval", nil, nil)
	token := e.link(owner, trip, types.InviteLinkInput{Active: true, JoinMode: "approval", Role: "collaborator"})

	r, _ := svc.Reply(e.ctx, token, a, "going")
	if r.Status != "requested" {
		t.Fatalf("approval-mode going should be requested, got %q", r.Status)
	}
	if _, ok := e.participantRow(trip, a); ok {
		if p, _ := e.participantRow(trip, a); p.Status == "accepted" {
			t.Fatalf("requested reply must not grant access yet")
		}
	}
	if !hasType(e.notifTypes(owner), "rsvp_request") {
		t.Fatalf("owner should get an rsvp_request notification")
	}

	// Approve → going + access.
	rsvps, _ := svc.ListRSVPs(e.ctx, trip, owner)
	if err := svc.Approve(e.ctx, trip, rsvps.RSVPs[0].ID, owner); err != nil {
		t.Fatalf("approve: %v", err)
	}
	after, _ := svc.ListRSVPs(e.ctx, trip, owner)
	if after.RSVPs[0].Status != "going" {
		t.Fatalf("approved reply should be going, got %q", after.RSVPs[0].Status)
	}
	if p, ok := e.participantRow(trip, a); !ok || p.Status != "accepted" || p.Role != "collaborator" {
		t.Fatalf("approved a should be an accepted collaborator: %+v", p)
	}
}

// Past reply_by closes the link to replies; reset rotates the token but keeps
// replies, and an inactive/old token 404s.
func TestRSVPClosedAndReset(t *testing.T) {
	e := newPubEnv(t)
	svc := NewRSVPService()
	owner := e.user("owner")
	a := e.user("a")
	b := e.user("b")

	// reply_by in the past → closed.
	past := types.Date(d(2026, 1, 1))
	closedTrip := e.trip(owner, "Closed", nil, nil)
	closedTok := e.link(owner, closedTrip, types.InviteLinkInput{Active: true, JoinMode: "anyone", Role: "viewer", ReplyBy: &past})
	_, err := svc.Reply(e.ctx, closedTok, a, "going")
	wantStatus(t, err, 403)

	// reset keeps replies; old token 404s.
	trip := e.trip(owner, "Reset", nil, nil)
	tok := e.link(owner, trip, types.InviteLinkInput{Active: true, JoinMode: "anyone", Role: "viewer"})
	if _, err := svc.Reply(e.ctx, tok, b, "going"); err != nil {
		t.Fatalf("b going: %v", err)
	}
	if _, err := svc.ResetLink(e.ctx, trip, owner); err != nil {
		t.Fatalf("reset: %v", err)
	}
	_, err = svc.Reply(e.ctx, tok, a, "going") // old token
	wantStatus(t, err, 404)
	rsvps, _ := svc.ListRSVPs(e.ctx, trip, owner)
	if len(rsvps.RSVPs) != 1 || rsvps.RSVPs[0].UserID != b {
		t.Fatalf("reset must keep replies: %+v", rsvps.RSVPs)
	}
}

// The join page reports counts, going faces, my_rsvp, and planned cost from budget;
// an inactive link 404s.
func TestJoinInfo(t *testing.T) {
	e := newPubEnv(t)
	svc := NewRSVPService()
	owner := e.user("owner")
	a := e.user("a")
	trip := e.trip(owner, "Join", nil, nil)
	budget := int64(1800000)
	if err := e.q.SetTripBudget(e.ctx, models.SetTripBudgetParams{BudgetAmount: &budget, ID: trip}); err != nil {
		t.Fatalf("budget: %v", err)
	}
	tok := e.link(owner, trip, types.InviteLinkInput{Active: true, JoinMode: "anyone", Role: "viewer"})
	if _, err := svc.Reply(e.ctx, tok, a, "going"); err != nil {
		t.Fatalf("a going: %v", err)
	}

	info, err := svc.JoinInfo(e.ctx, tok, &a)
	if err != nil {
		t.Fatalf("join info: %v", err)
	}
	if info.Counts.Going != 1 || len(info.GoingPeople) != 1 {
		t.Fatalf("counts/going_people wrong: %+v", info)
	}
	if info.MyRSVP == nil || *info.MyRSVP != "going" {
		t.Fatalf("my_rsvp should be going: %v", info.MyRSVP)
	}
	// 1 going → planned = budget / max(1, going) = 1,800,000.
	if info.PlannedCostPerPerson == nil || *info.PlannedCostPerPerson != 1800000 {
		t.Fatalf("planned_cost_per_person = %v, want 1800000", info.PlannedCostPerPerson)
	}

	// Inactive link → 404.
	if _, err := svc.UpsertLink(e.ctx, trip, owner, types.InviteLinkInput{Active: false, JoinMode: "anyone", Role: "viewer"}); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	_, err = svc.JoinInfo(e.ctx, tok, &a)
	wantStatus(t, err, 404)
}
