package services

import (
	"strings"
	"testing"

	"travel-service/internal/types"
)

// Comments: create, one-level replies, ordering, validation, and notifications.
func TestCommentsCreateListNotify(t *testing.T) {
	e := newPubEnv(t)
	cs := NewCommentService()
	owner := e.user("owner")
	asker := e.user("asker")
	replier := e.user("replier")
	trip := e.trip(owner, "Comments", nil, nil)
	e.publish(trip, false)

	top, err := cs.Create(e.ctx, trip, asker, types.CommentInput{Body: "  How was the trek?  "})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if top.Body != "How was the trek?" { // trimmed
		t.Fatalf("body not trimmed: %q", top.Body)
	}
	if !hasType(e.notifTypes(owner), "comment") {
		t.Fatalf("owner should get a comment notification")
	}

	reply, err := cs.Create(e.ctx, trip, replier, types.CommentInput{Body: "Tough but worth it", ParentID: &top.ID})
	if err != nil {
		t.Fatalf("reply: %v", err)
	}
	if reply.ParentID == nil || *reply.ParentID != top.ID {
		t.Fatalf("reply parent_id wrong: %+v", reply.ParentID)
	}
	if !hasType(e.notifTypes(asker), "comment_reply") {
		t.Fatalf("parent author should get comment_reply")
	}

	// A reply to a reply is rejected (one level only).
	_, err = cs.Create(e.ctx, trip, owner, types.CommentInput{Body: "nested", ParentID: &reply.ID})
	wantStatus(t, err, 422)

	// Body length bounds.
	wantStatus(t, func() error { _, e2 := cs.Create(e.ctx, trip, asker, types.CommentInput{Body: "   "}); return e2 }(), 422)
	wantStatus(t, func() error {
		_, e2 := cs.Create(e.ctx, trip, asker, types.CommentInput{Body: strings.Repeat("x", 1001)})
		return e2
	}(), 422)

	list, err := cs.List(e.ctx, trip, "", nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list.Comments) != 2 || list.Comments[0].ID != top.ID || list.Comments[1].ID != reply.ID {
		t.Fatalf("list should be oldest-first [top, reply]: %+v", list.Comments)
	}

	// Comment on a private trip → 404.
	priv := e.trip(owner, "Private", nil, nil)
	_, err = cs.Create(e.ctx, priv, asker, types.CommentInput{Body: "hi"})
	wantStatus(t, err, 404)
}

// Edit is author-only (sets edited_at); delete is author or trip owner; deleting a
// top-level comment removes its replies.
func TestCommentPermissions(t *testing.T) {
	e := newPubEnv(t)
	cs := NewCommentService()
	owner := e.user("owner")
	author := e.user("author")
	other := e.user("other")
	trip := e.trip(owner, "Perms", nil, nil)
	e.publish(trip, false)

	c, _ := cs.Create(e.ctx, trip, author, types.CommentInput{Body: "original"})

	// Non-author edit → 403; author edit sets edited_at.
	_, err := cs.Update(e.ctx, c.ID, other, types.UpdateCommentInput{Body: "hacked"})
	wantStatus(t, err, 403)
	up, err := cs.Update(e.ctx, c.ID, author, types.UpdateCommentInput{Body: "edited"})
	if err != nil || up.EditedAt == nil || up.Body != "edited" {
		t.Fatalf("author edit failed: %+v %v", up, err)
	}

	// A reply, then owner deletes the top-level → reply also gone.
	reply, _ := cs.Create(e.ctx, trip, other, types.CommentInput{Body: "reply", ParentID: &c.ID})
	_ = reply
	// Random user can't delete.
	wantStatus(t, cs.Delete(e.ctx, c.ID, e.user("rando")), 403)
	// Trip owner can.
	if err := cs.Delete(e.ctx, c.ID, owner); err != nil {
		t.Fatalf("owner delete: %v", err)
	}
	list, _ := cs.List(e.ctx, trip, "", nil)
	if len(list.Comments) != 0 {
		t.Fatalf("deleting top-level should remove its replies, got %d", len(list.Comments))
	}
}

// Comments by blocked people are hidden both ways; blocked people can't comment.
func TestCommentBlockVisibility(t *testing.T) {
	e := newPubEnv(t)
	cs := NewCommentService()
	soc := NewSocialService()
	owner := e.user("owner")
	a := e.user("a")
	b := e.user("b")
	trip := e.trip(owner, "BlockComments", nil, nil)
	e.publish(trip, false)

	ca, _ := cs.Create(e.ctx, trip, a, types.CommentInput{Body: "from a"})
	cb, _ := cs.Create(e.ctx, trip, b, types.CommentInput{Body: "from b"})

	if err := soc.Block(e.ctx, b, a); err != nil {
		t.Fatalf("b blocks a: %v", err)
	}

	// b doesn't see a's comment.
	asB, _ := cs.List(e.ctx, trip, "", &b)
	for _, c := range asB.Comments {
		if c.ID == ca.ID {
			t.Fatalf("b should not see a's comment")
		}
	}
	// a doesn't see b's comment.
	asA, _ := cs.List(e.ctx, trip, "", &a)
	for _, c := range asA.Comments {
		if c.ID == cb.ID {
			t.Fatalf("a should not see b's comment")
		}
	}

	// Owner blocks X → X can't comment on owner's trip.
	x := e.user("x")
	if err := soc.Block(e.ctx, owner, x); err != nil {
		t.Fatalf("owner blocks x: %v", err)
	}
	_, err := cs.Create(e.ctx, trip, x, types.CommentInput{Body: "blocked"})
	wantStatus(t, err, 404)
}
