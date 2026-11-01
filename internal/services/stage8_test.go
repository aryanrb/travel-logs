package services

import (
	"testing"

	"github.com/google/uuid"
)

// Notifications list newest-first with an unread count; mark-read clears them;
// the per-type payload (actor, trip) is populated.
func TestNotificationsListAndRead(t *testing.T) {
	e := newPubEnv(t)
	soc := NewSocialService()
	pub := NewPublicService()
	ns := NewNotificationService()
	owner := e.user("owner")
	follower := e.user("follower")
	liker := e.user("liker")

	trip := e.trip(owner, "Notif", nil, nil)
	e.publish(trip, false)

	if err := soc.Follow(e.ctx, follower, owner); err != nil { // → follow notif
		t.Fatalf("follow: %v", err)
	}
	if err := pub.Like(e.ctx, trip, liker); err != nil { // → like notif (newer)
		t.Fatalf("like: %v", err)
	}

	res, err := ns.List(e.ctx, owner, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(res.Notifications) != 2 || res.Unread != 2 {
		t.Fatalf("want 2 notifications & unread=2, got %d / %d", len(res.Notifications), res.Unread)
	}
	// Newest first: like then follow.
	if res.Notifications[0].Type != "like" || res.Notifications[1].Type != "follow" {
		t.Fatalf("not newest-first: %s then %s", res.Notifications[0].Type, res.Notifications[1].Type)
	}
	like := res.Notifications[0]
	if like.Actor == nil || like.Actor.ID != liker || like.Trip == nil || like.Trip.ID != trip {
		t.Fatalf("like notification payload wrong: %+v", like)
	}

	// Mark one read → unread drops to 1.
	if err := ns.MarkRead(e.ctx, owner, []uuid.UUID{like.ID}); err != nil {
		t.Fatalf("mark one: %v", err)
	}
	res, _ = ns.List(e.ctx, owner, "")
	if res.Unread != 1 {
		t.Fatalf("unread after one read = %d, want 1", res.Unread)
	}

	// Mark all (nil ids) → unread 0.
	if err := ns.MarkRead(e.ctx, owner, nil); err != nil {
		t.Fatalf("mark all: %v", err)
	}
	res, _ = ns.List(e.ctx, owner, "")
	if res.Unread != 0 {
		t.Fatalf("unread after mark-all = %d, want 0", res.Unread)
	}
}

// Notifications from people the viewer has since blocked are hidden from the list.
func TestNotificationsBlockHidden(t *testing.T) {
	e := newPubEnv(t)
	soc := NewSocialService()
	ns := NewNotificationService()
	owner := e.user("owner")
	a := e.user("a")

	if err := soc.Follow(e.ctx, a, owner); err != nil {
		t.Fatalf("a follow owner: %v", err)
	}
	if res, _ := ns.List(e.ctx, owner, ""); len(res.Notifications) != 1 {
		t.Fatalf("owner should have 1 notification before block, got %d", len(res.Notifications))
	}

	if err := soc.Block(e.ctx, owner, a); err != nil {
		t.Fatalf("owner blocks a: %v", err)
	}
	res, _ := ns.List(e.ctx, owner, "")
	for _, n := range res.Notifications {
		if n.Actor != nil && n.Actor.ID == a {
			t.Fatalf("notification from blocked actor should be hidden")
		}
	}
}
