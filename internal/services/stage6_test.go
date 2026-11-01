package services

import (
	"testing"

	"github.com/google/uuid"

	"travel-service/internal/types"
)

// Likes are idempotent, public-only, notify the owner once (not for your own trip),
// and surface as likes/liked_by_me across explore + detail. popular sorts by them.
func TestLikesAndCounts(t *testing.T) {
	e := newPubEnv(t)
	pub := NewPublicService()
	owner := e.user("owner")
	liker := e.user("liker")
	nonce := "LIKE" + uuid.NewString()

	a := e.trip(owner, nonce+" a", nil, nil)
	b := e.trip(owner, nonce+" b", nil, nil)
	e.publish(a, false)
	e.publish(b, false)

	// liker likes a (twice → idempotent).
	if err := pub.Like(e.ctx, a, liker); err != nil {
		t.Fatalf("like: %v", err)
	}
	if err := pub.Like(e.ctx, a, liker); err != nil {
		t.Fatalf("like again: %v", err)
	}
	if !hasType(e.notifTypes(owner), "like") {
		t.Fatalf("owner should get a like notification")
	}

	// Detail shows likes + liked_by_me for the liker, false for a stranger.
	detail, _ := pub.GetTrip(e.ctx, a, &liker)
	if detail.Likes != 1 || !detail.LikedByMe {
		t.Fatalf("detail likes=%d liked_by_me=%v, want 1/true", detail.Likes, detail.LikedByMe)
	}
	other := e.user("other")
	detail, _ = pub.GetTrip(e.ctx, a, &other)
	if detail.LikedByMe {
		t.Fatalf("stranger should not have liked_by_me")
	}

	// Explore: a has 1 like, liked_by_me true for liker; popular puts a before b.
	res, _ := pub.ListTrips(e.ctx, types.ExploreParams{Q: nonce, Sort: "popular"}, &liker)
	if len(res.Trips) != 2 || res.Trips[0].Trip.ID != a {
		t.Fatalf("popular should rank liked trip first: %+v", ids(res))
	}
	for _, it := range res.Trips {
		if it.Trip.ID == a && (it.Likes != 1 || !it.LikedByMe) {
			t.Fatalf("explore item a likes/liked_by_me wrong: %+v", it)
		}
	}

	// Unlike (idempotent) drops the count.
	if err := pub.Unlike(e.ctx, a, liker); err != nil {
		t.Fatalf("unlike: %v", err)
	}
	if err := pub.Unlike(e.ctx, a, liker); err != nil {
		t.Fatalf("unlike again: %v", err)
	}
	detail, _ = pub.GetTrip(e.ctx, a, &liker)
	if detail.Likes != 0 || detail.LikedByMe {
		t.Fatalf("after unlike: likes=%d liked_by_me=%v", detail.Likes, detail.LikedByMe)
	}

	// Liking your own trip does not notify you.
	before := len(e.notifTypes(owner))
	if err := pub.Like(e.ctx, b, owner); err != nil {
		t.Fatalf("owner like own: %v", err)
	}
	if len(e.notifTypes(owner)) != before {
		t.Fatalf("liking your own trip must not notify you")
	}

	// Liking a private trip → 404.
	priv := e.trip(owner, nonce+" priv", nil, nil)
	wantStatus(t, pub.Like(e.ctx, priv, liker), 404)
}

// saved_by_me surfaces alongside saves on explore items.
func TestSavedByMeFlag(t *testing.T) {
	e := newPubEnv(t)
	pub := NewPublicService()
	owner := e.user("owner")
	saver := e.user("saver")
	nonce := "SAVE" + uuid.NewString()

	trip := e.trip(owner, nonce+" t", nil, nil)
	e.publish(trip, false)
	if err := pub.Save(e.ctx, trip, saver); err != nil {
		t.Fatalf("save: %v", err)
	}

	res, _ := pub.ListTrips(e.ctx, types.ExploreParams{Q: nonce}, &saver)
	if len(res.Trips) != 1 || res.Trips[0].Saves != 1 || !res.Trips[0].SavedByMe {
		t.Fatalf("saves/saved_by_me wrong: %+v", res.Trips)
	}
	// Signed out → saved_by_me false.
	out, _ := pub.ListTrips(e.ctx, types.ExploreParams{Q: nonce}, nil)
	if out.Trips[0].SavedByMe {
		t.Fatalf("saved_by_me must be false signed out")
	}
}
