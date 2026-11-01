package services

import (
	"testing"

	"travel-service/internal/types"
)

func fp(f float64) *float64 { return &f }

// ─── Pure unit checks (no DB) ────────────────────────────────────────────────

func TestValidateCoords(t *testing.T) {
	ok := []struct{ lat, lng *float64 }{
		{nil, nil},
		{fp(27.7), fp(85.3)},
		{fp(-90), fp(-180)},
		{fp(90), fp(180)},
	}
	for _, c := range ok {
		if err := validateCoords(c.lat, c.lng); err != nil {
			t.Fatalf("validateCoords(%v,%v) = %v, want nil", c.lat, c.lng, err)
		}
	}
	bad := []struct{ lat, lng *float64 }{
		{fp(27.7), nil},   // only one
		{nil, fp(85.3)},   // only one
		{fp(91), fp(0)},   // lat too high
		{fp(-91), fp(0)},  // lat too low
		{fp(0), fp(181)},  // lng too high
		{fp(0), fp(-181)}, // lng too low
	}
	for _, c := range bad {
		if err := validateCoords(c.lat, c.lng); err == nil {
			t.Fatalf("validateCoords(%v,%v) = nil, want error", c.lat, c.lng)
		}
	}
}

func TestNormalizeHTTPSURL(t *testing.T) {
	// nil and "" both clear (→ nil).
	for _, in := range []*string{nil, sp(""), sp("   ")} {
		got, err := normalizeHTTPSURL(in)
		if err != nil || got != nil {
			t.Fatalf("normalizeHTTPSURL(%v) = %v,%v; want nil,nil", in, got, err)
		}
	}
	// https passes, trimmed.
	got, err := normalizeHTTPSURL(sp("  https://ik.io/a.jpg  "))
	if err != nil || got == nil || *got != "https://ik.io/a.jpg" {
		t.Fatalf("https normalize = %v,%v", got, err)
	}
	// non-https rejected.
	if _, err := normalizeHTTPSURL(sp("http://ik.io/a.jpg")); err == nil {
		t.Fatalf("http:// must be rejected")
	}
}

func TestNormalizeBio(t *testing.T) {
	if got, _ := normalizeBio(sp("")); got != nil {
		t.Fatalf(`"" bio must clear to nil`)
	}
	long := make([]rune, maxBioLen+1)
	for i := range long {
		long[i] = 'a'
	}
	if _, err := normalizeBio(sp(string(long))); err == nil {
		t.Fatalf("bio over %d chars must be rejected", maxBioLen)
	}
	if got, err := normalizeBio(sp("hi there")); err != nil || got == nil || *got != "hi there" {
		t.Fatalf("valid bio = %v,%v", got, err)
	}
}

// ─── DB-integration checks ───────────────────────────────────────────────────

// Pins are stored on trip/destination/entry/journey and surface on the public
// endpoint along with owner_id.
func TestStage1PinsAndOwnerIDOnPublic(t *testing.T) {
	e := newPubEnv(t)
	ts := NewTripService()
	tl := NewTimelineService()
	tr := NewTransportService()
	pub := NewPublicService()

	owner := e.user("owner")

	trip, err := ts.Create(e.ctx, owner, types.TripInput{
		Title: "Pinned", StartLocation: sp("Kathmandu"),
		StartLat: fp(27.7172), StartLng: fp(85.3240),
	})
	if err != nil {
		t.Fatalf("create trip: %v", err)
	}
	e.trips = append(e.trips, trip.ID)
	if trip.StartLat == nil || *trip.StartLat != 27.7172 {
		t.Fatalf("pin not stored on trip: %+v", trip.StartLat)
	}

	if _, err := ts.AddDestination(e.ctx, trip.ID, owner, types.DestinationInput{
		Name: "Panch Pokhari", Lat: fp(27.95), Lng: fp(85.75),
	}); err != nil {
		t.Fatalf("add destination: %v", err)
	}
	if _, err := tl.CreateEntry(e.ctx, trip.ID, owner, types.EntryInput{
		Title: "Day 1", EntryDate: types.Date(d(2026, 3, 2)), Lat: fp(27.7), Lng: fp(85.3),
	}); err != nil {
		t.Fatalf("create entry: %v", err)
	}
	if _, err := tr.Create(e.ctx, trip.ID, owner, types.TransportInput{
		Type: "bus", StartLat: fp(27.7), StartLng: fp(85.3),
		DestinationLat: fp(27.95), DestinationLng: fp(85.75),
	}); err != nil {
		t.Fatalf("create transport: %v", err)
	}

	e.publish(trip.ID, false)

	detail, err := pub.GetTrip(e.ctx, trip.ID, nil)
	if err != nil {
		t.Fatalf("public get: %v", err)
	}
	if detail.Trip.OwnerID != owner {
		t.Fatalf("public trip owner_id = %v, want %v", detail.Trip.OwnerID, owner)
	}
	if detail.Trip.StartLat == nil || *detail.Trip.StartLat != 27.7172 {
		t.Fatalf("public trip start_lat missing: %+v", detail.Trip.StartLat)
	}
	if len(detail.Destinations) != 1 || detail.Destinations[0].Lat == nil || *detail.Destinations[0].Lat != 27.95 {
		t.Fatalf("public destination coords missing: %+v", detail.Destinations)
	}
	if len(detail.Entries) != 1 || detail.Entries[0].Lat == nil || *detail.Entries[0].Lat != 27.7 {
		t.Fatalf("public entry coords missing: %+v", detail.Entries)
	}
	if len(detail.Transportation) != 1 {
		t.Fatalf("want 1 journey, got %d", len(detail.Transportation))
	}
	j := detail.Transportation[0]
	if j.StartLat == nil || *j.StartLat != 27.7 || j.DestinationLat == nil || *j.DestinationLat != 27.95 {
		t.Fatalf("public journey coords missing: %+v", j)
	}

	// List also carries owner_id.
	list, err := pub.ListTrips(e.ctx, types.ExploreParams{}, nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	found := false
	for _, it := range list.Trips {
		if it.Trip.ID == trip.ID {
			found = true
			if it.Trip.OwnerID != owner {
				t.Fatalf("list owner_id = %v, want %v", it.Trip.OwnerID, owner)
			}
		}
	}
	if !found {
		t.Fatalf("trip missing from public list")
	}
}

// Out-of-range coordinates are rejected with 422.
func TestStage1CoordRangeRejected(t *testing.T) {
	e := newPubEnv(t)
	ts := NewTripService()
	owner := e.user("owner")

	_, err := ts.Create(e.ctx, owner, types.TripInput{Title: "Bad", StartLat: fp(200), StartLng: fp(0)})
	wantStatus(t, err, 422)

	// Only one of the pair → 422.
	_, err = ts.Create(e.ctx, owner, types.TripInput{Title: "Half", StartLat: fp(27.7)})
	wantStatus(t, err, 422)
}

// cover_image: "" clears it; non-https is rejected. Same rules back GET/PUT /me.
func TestStage1CoverImageAndProfile(t *testing.T) {
	e := newPubEnv(t)
	ts := NewTripService()
	us := NewUserService()
	owner := e.user("owner")

	trip, err := ts.Create(e.ctx, owner, types.TripInput{Title: "Cover", CoverImage: sp("https://ik.io/c.jpg")})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	e.trips = append(e.trips, trip.ID)
	if trip.CoverImage == nil || *trip.CoverImage != "https://ik.io/c.jpg" {
		t.Fatalf("cover not set: %+v", trip.CoverImage)
	}

	// "" clears it.
	cleared, err := ts.Update(e.ctx, trip.ID, owner, types.TripInput{Title: "Cover", CoverImage: sp("")})
	if err != nil {
		t.Fatalf("update clear: %v", err)
	}
	if cleared.CoverImage != nil {
		t.Fatalf("cover_image should be cleared, got %v", *cleared.CoverImage)
	}

	// non-https rejected.
	_, err = ts.Update(e.ctx, trip.ID, owner, types.TripInput{Title: "Cover", CoverImage: sp("http://evil/x.jpg")})
	wantStatus(t, err, 422)

	// Profile bio round-trips via GET /me; "" clears; non-https photo rejected.
	if err := us.UpdateProfile(e.ctx, owner, types.UpdateProfileInput{Name: "Owner", Bio: sp("explorer"), PhotoURL: sp("https://ik.io/me.jpg")}); err != nil {
		t.Fatalf("update profile: %v", err)
	}
	me, err := us.Me(e.ctx, owner)
	if err != nil {
		t.Fatalf("me: %v", err)
	}
	if me.Bio == nil || *me.Bio != "explorer" {
		t.Fatalf("bio not returned by GET /me: %+v", me.Bio)
	}
	if me.PhotoURL == nil || *me.PhotoURL != "https://ik.io/me.jpg" {
		t.Fatalf("photo not set: %+v", me.PhotoURL)
	}

	if err := us.UpdateProfile(e.ctx, owner, types.UpdateProfileInput{Name: "Owner", Bio: sp("")}); err != nil {
		t.Fatalf("clear bio: %v", err)
	}
	me, _ = us.Me(e.ctx, owner)
	if me.Bio != nil {
		t.Fatalf("bio should be cleared, got %v", *me.Bio)
	}

	wantStatus(t, us.UpdateProfile(e.ctx, owner, types.UpdateProfileInput{Name: "Owner", PhotoURL: sp("http://x/y.jpg")}), 422)
}
