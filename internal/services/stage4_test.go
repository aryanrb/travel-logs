package services

import (
	"testing"

	"github.com/google/uuid"

	"travel-service/internal/models"
	"travel-service/internal/types"
)

// Follows are one-way and show up in the profile relationship; notify on new follow.
func TestFollowAndRelationship(t *testing.T) {
	e := newPubEnv(t)
	soc := NewSocialService()
	a := e.user("a")
	b := e.user("b")

	if err := soc.Follow(e.ctx, a, b); err != nil {
		t.Fatalf("a follow b: %v", err)
	}
	prof, err := soc.Profile(e.ctx, b, &a) // a views b
	if err != nil {
		t.Fatalf("profile: %v", err)
	}
	if prof.Relationship == nil || !prof.Relationship.Following || prof.Relationship.FollowsYou {
		t.Fatalf("relationship after a→b follow: %+v", prof.Relationship)
	}
	if prof.User.ID != b || prof.Stats.Followers != 1 {
		t.Fatalf("profile stats wrong: %+v", prof)
	}
	if !hasType(e.notifTypes(b), "follow") {
		t.Fatalf("b should get a follow notification")
	}

	// self → no relationship; follow self → 400.
	self, _ := soc.Profile(e.ctx, a, &a)
	if self.Relationship != nil {
		t.Fatalf("relationship must be nil when viewing yourself")
	}
	wantStatus(t, soc.Follow(e.ctx, a, a), 400)

	if err := soc.Unfollow(e.ctx, a, b); err != nil {
		t.Fatalf("unfollow: %v", err)
	}
	prof, _ = soc.Profile(e.ctx, b, &a)
	if prof.Relationship.Following {
		t.Fatalf("should not be following after unfollow")
	}
}

// Friend request → accept → friends; notifications fire; /me/friends reflects it.
func TestFriendRequestAccept(t *testing.T) {
	e := newPubEnv(t)
	soc := NewSocialService()
	a := e.user("a")
	b := e.user("b")

	if err := soc.Friend(e.ctx, a, b); err != nil {
		t.Fatalf("a friend b: %v", err)
	}
	// a requested b.
	if p, _ := soc.Profile(e.ctx, b, &a); p.Relationship.Friend != "requested" {
		t.Fatalf("a→b should be 'requested', got %q", p.Relationship.Friend)
	}
	if p, _ := soc.Profile(e.ctx, a, &b); p.Relationship.Friend != "incoming" {
		t.Fatalf("b sees a as 'incoming', got %q", p.Relationship.Friend)
	}
	if !hasType(e.notifTypes(b), "friend_request") {
		t.Fatalf("b should get a friend_request notification")
	}

	// b accepts by friending a back.
	if err := soc.Friend(e.ctx, b, a); err != nil {
		t.Fatalf("b friend a: %v", err)
	}
	if p, _ := soc.Profile(e.ctx, b, &a); p.Relationship.Friend != "friends" {
		t.Fatalf("should be 'friends' after accept, got %q", p.Relationship.Friend)
	}
	if !hasType(e.notifTypes(a), "friend_accepted") {
		t.Fatalf("a should get a friend_accepted notification")
	}
	fr, _ := soc.Friends(e.ctx, a)
	if len(fr.Friends) != 1 || fr.Friends[0].ID != b {
		t.Fatalf("a's friends should be [b]: %+v", fr.Friends)
	}

	// Unfriend → none.
	if err := soc.Unfriend(e.ctx, a, b); err != nil {
		t.Fatalf("unfriend: %v", err)
	}
	if p, _ := soc.Profile(e.ctx, b, &a); p.Relationship.Friend != "none" {
		t.Fatalf("should be 'none' after unfriend, got %q", p.Relationship.Friend)
	}
}

// Blocking hides profiles/trips both ways, ends relationships, and blocks follows.
func TestBlockVisibility(t *testing.T) {
	e := newPubEnv(t)
	soc := NewSocialService()
	pub := NewPublicService()
	a := e.user("a")
	b := e.user("b")
	nonce := "BLK" + uuid.NewString()

	aTrip := e.trip(a, nonce+" a", nil, nil)
	bTrip := e.trip(b, nonce+" b", nil, nil)
	e.publish(aTrip, false)
	e.publish(bTrip, false)

	// Pre-existing relationships that the block must sever.
	_ = soc.Follow(e.ctx, a, b)
	_ = soc.Follow(e.ctx, b, a)
	_ = soc.Friend(e.ctx, a, b)
	_ = soc.Friend(e.ctx, b, a) // accepted

	if err := soc.Block(e.ctx, a, b); err != nil {
		t.Fatalf("a block b: %v", err)
	}

	// Relationships severed both ways.
	if f, _ := e.q.IsFollowing(e.ctx, models.IsFollowingParams{FollowerID: a, FolloweeID: b}); f {
		t.Fatalf("a→b follow should be gone after block")
	}
	if f, _ := e.q.IsFollowing(e.ctx, models.IsFollowingParams{FollowerID: b, FolloweeID: a}); f {
		t.Fatalf("b→a follow should be gone after block")
	}
	if ok, _ := areFriends(e.ctx, e.q, a, b); ok {
		t.Fatalf("friendship should be gone after block")
	}

	// Target blocked viewer → 404 (b viewing a).
	_, err := soc.Profile(e.ctx, a, &b)
	wantStatus(t, err, 404)
	// Viewer blocked target → profile with empty trips + blocked=true (a viewing b).
	prof, err := soc.Profile(e.ctx, b, &a)
	if err != nil {
		t.Fatalf("a viewing b: %v", err)
	}
	if !prof.Relationship.Blocked || len(prof.Trips) != 0 {
		t.Fatalf("a-blocked-b profile should have blocked=true and empty trips: %+v", prof.Relationship)
	}

	// Explore hides each other's trips.
	aView, _ := pub.ListTrips(e.ctx, types.ExploreParams{Q: nonce}, &a)
	for _, it := range aView.Trips {
		if it.Trip.ID == bTrip {
			t.Fatalf("a should not see b's trip in explore")
		}
	}
	bView, _ := pub.ListTrips(e.ctx, types.ExploreParams{Q: nonce}, &b)
	for _, it := range bView.Trips {
		if it.Trip.ID == aTrip {
			t.Fatalf("b should not see a's trip in explore")
		}
	}

	// Blocked person can't follow back.
	wantStatus(t, soc.Follow(e.ctx, b, a), 404)
}

// Friends-mode invite links admit the organiser's friends only (areFriends wired).
func TestRSVPFriendsMode(t *testing.T) {
	e := newPubEnv(t)
	soc := NewSocialService()
	rsvp := NewRSVPService()
	owner := e.user("owner")
	stranger := e.user("stranger")
	pal := e.user("pal")
	trip := e.trip(owner, "Friends only", nil, nil)
	token := e.link(owner, trip, types.InviteLinkInput{Active: true, JoinMode: "friends", Role: "viewer"})

	// Non-friend rejected.
	_, err := rsvp.Reply(e.ctx, token, stranger, "going")
	wantStatus(t, err, 403)

	// Become friends with the owner, then it works.
	_ = soc.Friend(e.ctx, pal, owner)
	_ = soc.Friend(e.ctx, owner, pal) // accept
	r, err := rsvp.Reply(e.ctx, token, pal, "going")
	if err != nil || r.Status != "going" {
		t.Fatalf("friend should be able to go: %+v %v", r, err)
	}
}

// Report validation: reason=other needs details; bad reason/subject rejected.
func TestReportValidation(t *testing.T) {
	e := newPubEnv(t)
	soc := NewSocialService()
	reporter := e.user("reporter")
	target := e.user("target")

	wantStatus(t, soc.Report(e.ctx, reporter, types.ReportInput{SubjectType: "user", SubjectID: target, Reason: "other"}), 422)
	wantStatus(t, soc.Report(e.ctx, reporter, types.ReportInput{SubjectType: "nonsense", SubjectID: target, Reason: "spam"}), 422)

	if err := soc.Report(e.ctx, reporter, types.ReportInput{SubjectType: "user", SubjectID: target, Reason: "spam"}); err != nil {
		t.Fatalf("valid report: %v", err)
	}
	details := "they posted ads"
	if err := soc.Report(e.ctx, reporter, types.ReportInput{SubjectType: "trip", SubjectID: target, Reason: "other", Details: &details}); err != nil {
		t.Fatalf("other-with-details report: %v", err)
	}
}
