package services

import (
	"context"
	"encoding/base64"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	db "travel-service/internal/database"
	"travel-service/internal/models"
	"travel-service/internal/types"
	"travel-service/internal/utils/apperror"
)

const (
	defaultExploreLimit = 20
	maxExploreLimit     = 100
)

type PublicService struct {
	q   *models.Queries
	db  *pgxpool.Pool
	log *zap.Logger
}

func NewPublicService() *PublicService {
	return &PublicService{q: models.New(db.Client), db: db.Client, log: zap.L().Named("PublicService")}
}

// ─── Pure mappers (no DB — unit-testable) ────────────────────────────────────

func toPublicTripSafe(t models.Trip) types.PublicTripSafe {
	return types.PublicTripSafe{
		ID:            t.ID,
		OwnerID:       t.OwnerID,
		Title:         t.Title,
		Description:   t.Description,
		CoverImage:    t.CoverImage,
		StartDate:     types.DateFromPtr(t.StartDate),
		EndDate:       types.DateFromPtr(t.EndDate),
		StartLocation: t.StartLocation,
		TripType:      t.TripType,
		Currency:      t.Currency,
		IsPublic:      t.IsPublic,
		StartLat:      t.StartLat,
		StartLng:      t.StartLng,
	}
}

// toPublicTransport redacts a journey: cost only when costs are public, and the
// expense_id link is never exposed.
func toPublicTransport(t models.Transportation, showCosts bool) types.PublicTransport {
	pt := types.PublicTransport{
		ID:             t.ID,
		Type:           t.Type,
		StartPoint:     t.StartPoint,
		Destination:    t.Destination,
		DepartAt:       t.DepartAt,
		DistanceKm:     t.DistanceKm,
		DurationMin:    t.DurationMin,
		Notes:          t.Notes,
		EntryID:        t.EntryID,
		StartLat:       t.StartLat,
		StartLng:       t.StartLng,
		DestinationLat: t.DestinationLat,
		DestinationLng: t.DestinationLng,
	}
	if showCosts {
		pt.CostAmount = t.CostAmount
	}
	return pt
}

// buildPublicBudget returns nil unless costs are public (req: budget is null
// otherwise). Totals are expected to already exclude hidden expenses.
func buildPublicBudget(show bool, currency string, total int64, travellers int, cats []models.PublicCategoryBreakdownRow, planned *int64) *types.PublicBudget {
	if !show {
		return nil
	}
	if cats == nil {
		cats = []models.PublicCategoryBreakdownRow{}
	}
	return &types.PublicBudget{Currency: currency, Total: total, Travellers: travellers, ByCategory: cats, Planned: planned}
}

func shiftTime(t time.Time, d time.Duration) time.Time { return t.Add(d) }

func shiftPtr(t *time.Time, d time.Duration) *time.Time {
	if t == nil {
		return nil
	}
	nt := t.Add(d)
	return &nt
}

// ─── Endpoints ───────────────────────────────────────────────────────────────

// ListTrips powers explore. The SQL returns all public trips (filtered by q and
// trip_type) with aggregates; the rest — days, destination, cost+currency,
// following, sorting and paging — happens here so conversion and cost filters
// stay correct across pages.
func (s *PublicService) ListTrips(ctx context.Context, p types.ExploreParams, viewerID *uuid.UUID) (*types.PublicTripListResponse, error) {
	rows, err := s.q.ListPublicTripsForExplore(ctx, models.ListPublicTripsForExploreParams{
		Q:        strings.TrimSpace(p.Q),
		TripType: strings.TrimSpace(p.TripType),
	})
	if err != nil {
		return nil, apperror.FromDB(err)
	}

	costFilter := p.MinCostPerPerson != nil || p.MaxCostPerPerson != nil

	var followed, blocked map[uuid.UUID]bool
	if viewerID != nil {
		b, err := blockedSet(ctx, s.q, *viewerID)
		if err != nil {
			return nil, apperror.FromDB(err)
		}
		blocked = b
		if p.Following {
			ids, err := followedOwnerIDs(ctx, s.q, *viewerID)
			if err != nil {
				return nil, apperror.FromDB(err)
			}
			followed = make(map[uuid.UUID]bool, len(ids))
			for _, id := range ids {
				followed[id] = true
			}
		}
	}

	items := make([]types.PublicTripListItem, 0, len(rows))
	for _, r := range rows {
		t := r.Trip
		if blocked[t.OwnerID] {
			continue // hide trips by blocked people (either direction)
		}
		if p.Following && viewerID != nil && !followed[t.OwnerID] {
			continue
		}
		if p.MinDays != nil || p.MaxDays != nil {
			days, ok := modelTripDays(t)
			if !ok {
				continue // without both dates, excluded when a day filter is set
			}
			if p.MinDays != nil && days < *p.MinDays {
				continue
			}
			if p.MaxDays != nil && days > *p.MaxDays {
				continue
			}
		}
		if p.Destination != "" && !matchesDestination(t, r.DestinationNames, p.Destination) {
			continue
		}

		cpp := costPerPerson(t.ShowCostsPublic, r.TotalSpent, r.Travellers)

		if costFilter {
			if !t.ShowCostsPublic {
				continue // private costs excluded whenever a cost filter is set
			}
			cmp := *cpp
			if p.Currency != "" && !strings.EqualFold(p.Currency, t.Currency) {
				conv, ok := rates.convert(*cpp, t.Currency, p.Currency)
				if !ok {
					continue // can't convert → can't verify the filter → exclude
				}
				cmp = conv
			}
			if p.MinCostPerPerson != nil && cmp < *p.MinCostPerPerson {
				continue
			}
			if p.MaxCostPerPerson != nil && cmp > *p.MaxCostPerPerson {
				continue
			}
		}

		item := types.PublicTripListItem{
			Trip:          toPublicTripSafe(t),
			Destinations:  r.DestinationNames,
			OwnerName:     r.OwnerName,
			CostPerPerson: cpp,
			Likes:         int64(r.LikesCount),
			Saves:         int64(r.SavesCount),
		}
		if cpp != nil && p.Currency != "" && !strings.EqualFold(p.Currency, t.Currency) {
			if conv, ok := rates.convert(*cpp, t.Currency, p.Currency); ok {
				item.CostPerPersonConverted = &types.ConvertedCost{Amount: conv, Currency: strings.ToUpper(p.Currency)}
			}
		}
		items = append(items, item)
	}

	if err := decorateViewerFlags(ctx, s.q, items, viewerID); err != nil {
		return nil, err
	}
	sortExplore(items, p.Sort)
	return pageItems(items, p.Cursor, p.Limit), nil
}

// decorateViewerFlags sets liked_by_me/saved_by_me for a signed-in viewer across
// the given items in two queries (no-op when signed out).
func decorateViewerFlags(ctx context.Context, q *models.Queries, items []types.PublicTripListItem, viewerID *uuid.UUID) error {
	if viewerID == nil || len(items) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(items))
	for i := range items {
		ids[i] = items[i].Trip.ID
	}
	liked, err := q.LikedTripIDsFor(ctx, models.LikedTripIDsForParams{UserID: *viewerID, TripIds: ids})
	if err != nil {
		return apperror.FromDB(err)
	}
	saved, err := q.SavedTripIDsFor(ctx, models.SavedTripIDsForParams{UserID: *viewerID, TripIds: ids})
	if err != nil {
		return apperror.FromDB(err)
	}
	likedSet := make(map[uuid.UUID]bool, len(liked))
	for _, id := range liked {
		likedSet[id] = true
	}
	savedSet := make(map[uuid.UUID]bool, len(saved))
	for _, id := range saved {
		savedSet[id] = true
	}
	for i := range items {
		items[i].LikedByMe = likedSet[items[i].Trip.ID]
		items[i].SavedByMe = savedSet[items[i].Trip.ID]
	}
	return nil
}

// Like/Unlike toggle a like on a public trip (idempotent). Liking notifies the
// owner once per person (not for your own trip).
func (s *PublicService) Like(ctx context.Context, tripID, userID uuid.UUID) error {
	trip, err := s.q.GetPublicTrip(ctx, tripID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperror.NotFound("trip not found")
		}
		return apperror.FromDB(err)
	}
	already, _ := s.q.IsLiked(ctx, models.IsLikedParams{UserID: userID, TripID: tripID})
	if err := s.q.Like(ctx, models.LikeParams{UserID: userID, TripID: tripID}); err != nil {
		return apperror.FromDB(err)
	}
	if !already {
		notify(ctx, s.q, notifyParams{UserID: trip.Trip.OwnerID, Type: "like", ActorID: &userID, TripID: &tripID})
	}
	return nil
}

func (s *PublicService) Unlike(ctx context.Context, tripID, userID uuid.UUID) error {
	if err := s.ensurePublic(ctx, tripID); err != nil {
		return err
	}
	return apperror.FromDBOrNil(s.q.Unlike(ctx, models.UnlikeParams{UserID: userID, TripID: tripID}))
}

// costPerPerson is total spent ÷ travellers (travellers ≥ 1), or nil when the
// owner keeps costs private.
func costPerPerson(showCosts bool, total int64, travellers int32) *int64 {
	if !showCosts {
		return nil
	}
	denom := int64(travellers)
	if denom < 1 {
		denom = 1
	}
	v := total / denom
	return &v
}

// modelTripDays is the inclusive day count (start and end both counted): 12–15
// Nov = 4 days. ok=false when either date is missing.
func modelTripDays(t models.Trip) (int, bool) {
	if t.StartDate == nil || t.EndDate == nil {
		return 0, false
	}
	return int(t.EndDate.Sub(*t.StartDate).Hours()/24) + 1, true
}

func matchesDestination(t models.Trip, dests []string, needle string) bool {
	n := strings.ToLower(strings.TrimSpace(needle))
	if t.StartLocation != nil && strings.Contains(strings.ToLower(*t.StartLocation), n) {
		return true
	}
	for _, d := range dests {
		if strings.Contains(strings.ToLower(d), n) {
			return true
		}
	}
	return false
}

// itemDays mirrors modelTripDays for sorting, off the public item.
func itemDays(it types.PublicTripListItem) (int, bool) {
	sd, ed := it.Trip.StartDate, it.Trip.EndDate
	if sd == nil || ed == nil {
		return 0, false
	}
	return int(ed.Time().Sub(sd.Time()).Hours()/24) + 1, true
}

// sortExplore orders items in place. "" keeps the SQL order (created_at desc).
// Trips missing the dates a sort needs go last; ties break by id for stability.
func sortExplore(items []types.PublicTripListItem, mode string) {
	less := func(i, j int) bool { return false }
	switch mode {
	case "shortest", "longest":
		less = func(i, j int) bool {
			di, oi := itemDays(items[i])
			dj, oj := itemDays(items[j])
			if oi != oj {
				return oi // trips with dates first
			}
			if oi && di != dj {
				if mode == "shortest" {
					return di < dj
				}
				return di > dj
			}
			return items[i].Trip.ID.String() < items[j].Trip.ID.String()
		}
	case "soonest":
		less = func(i, j int) bool {
			si, sj := items[i].Trip.StartDate, items[j].Trip.StartDate
			if (si == nil) != (sj == nil) {
				return si != nil // trips with a start date first
			}
			if si != nil && !si.Time().Equal(sj.Time()) {
				return si.Time().Before(sj.Time())
			}
			return items[i].Trip.ID.String() < items[j].Trip.ID.String()
		}
	case "popular":
		less = func(i, j int) bool {
			if items[i].Likes != items[j].Likes {
				return items[i].Likes > items[j].Likes
			}
			if items[i].Saves != items[j].Saves {
				return items[i].Saves > items[j].Saves
			}
			return items[i].Trip.ID.String() < items[j].Trip.ID.String()
		}
	default:
		return
	}
	sort.SliceStable(items, less)
}

// pageItems applies the opaque offset cursor + limit and sets next_cursor.
func pageItems(items []types.PublicTripListItem, cursor string, limit int) *types.PublicTripListResponse {
	if limit <= 0 {
		limit = defaultExploreLimit
	}
	if limit > maxExploreLimit {
		limit = maxExploreLimit
	}
	start := decodeCursor(cursor)
	if start > len(items) {
		start = len(items)
	}
	end := start + limit
	var next *string
	if end < len(items) {
		c := encodeCursor(end)
		next = &c
	} else {
		end = len(items)
	}
	return &types.PublicTripListResponse{Trips: items[start:end], NextCursor: next}
}

func encodeCursor(n int) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(n)))
}

func decodeCursor(s string) int {
	if s == "" {
		return 0
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(string(b))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func (s *PublicService) GetTrip(ctx context.Context, tripID uuid.UUID, viewerID *uuid.UUID) (*types.PublicTripDetail, error) {
	row, err := s.q.GetPublicTrip(ctx, tripID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperror.NotFound("trip not found") // also covers private trips
		}
		return nil, apperror.FromDB(err)
	}
	trip := row.Trip
	if viewerID != nil {
		if blocked, _ := isBlockedBetween(ctx, s.q, *viewerID, trip.OwnerID); blocked {
			return nil, apperror.NotFound("trip not found")
		}
	}

	rawDests, err := s.q.ListDestinations(ctx, tripID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	dests := make([]types.PublicDestination, len(rawDests))
	for i, d := range rawDests {
		dests[i] = types.PublicDestination{ID: d.ID, Name: d.Name, ArrivalDate: types.DateFromPtr(d.ArrivalDate), SortOrder: d.SortOrder, Lat: d.Lat, Lng: d.Lng}
	}

	// People: owner first, then accepted collaborators. No viewers/externals/ids.
	collabs, err := s.q.ListPublicPeople(ctx, tripID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	people := make([]types.PublicPerson, 0, len(collabs)+1)
	people = append(people, types.PublicPerson{Name: row.OwnerName, Role: "owner"})
	for _, c := range collabs {
		people = append(people, types.PublicPerson{Name: c.Name, Role: c.Role})
	}
	travellers := len(people)

	rawEntries, err := s.q.ListEntries(ctx, tripID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	entries := make([]types.PublicEntry, len(rawEntries))
	for i, e := range rawEntries {
		entries[i] = types.PublicEntry{
			ID: e.ID, Title: e.Title, Description: e.Description,
			EntryDate: types.Date(e.EntryDate), EntryTime: e.EntryTime, Location: e.Location,
			Lat: e.Lat, Lng: e.Lng,
		}
	}

	rawTrans, err := s.q.ListTransport(ctx, tripID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	trans := make([]types.PublicTransport, len(rawTrans))
	for i, t := range rawTrans {
		trans[i] = toPublicTransport(t, trip.ShowCostsPublic)
	}

	rawPhotos, err := s.q.ListTripPhotos(ctx, tripID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	photos := make([]types.PublicPhoto, len(rawPhotos))
	for i, p := range rawPhotos {
		photos[i] = types.PublicPhoto{ID: p.ID, URL: p.Url, Caption: p.Caption, TakenDate: types.DateFromPtr(p.TakenDate), EntryID: p.EntryID}
	}

	var total int64
	var cats []models.PublicCategoryBreakdownRow
	if trip.ShowCostsPublic {
		if total, err = s.q.PublicTotalExpenses(ctx, tripID); err != nil {
			return nil, apperror.FromDB(err)
		}
		if cats, err = s.q.PublicCategoryBreakdown(ctx, tripID); err != nil {
			return nil, apperror.FromDB(err)
		}
	}
	budget := buildPublicBudget(trip.ShowCostsPublic, trip.Currency, total, travellers, cats, trip.BudgetAmount)

	likes, _ := s.q.CountLikes(ctx, tripID)
	saves, _ := s.q.CountSaves(ctx, tripID)
	var likedByMe, savedByMe bool
	if viewerID != nil {
		likedByMe, _ = s.q.IsLiked(ctx, models.IsLikedParams{UserID: *viewerID, TripID: tripID})
		savedByMe, _ = s.q.IsSaved(ctx, models.IsSavedParams{UserID: *viewerID, TripID: tripID})
	}

	return &types.PublicTripDetail{
		Trip:           toPublicTripSafe(trip),
		Destinations:   dests,
		People:         people,
		Entries:        entries,
		Transportation: trans,
		Photos:         photos,
		Budget:         budget,
		Likes:          int64(likes),
		LikedByMe:      likedByMe,
		Saves:          int64(saves),
		SavedByMe:      savedByMe,
	}, nil
}

// Copy clones a public trip into a new PRIVATE trip owned by the caller. Dates
// shift by (new start_date − source start_date). Nothing financial or personal
// is copied (no photos, expenses, settlements, participants, external payers;
// journeys lose expense_id and cost_amount). Runs in one transaction.
func (s *PublicService) Copy(ctx context.Context, sourceID, callerID uuid.UUID, in types.CopyTripInput) (*models.Trip, error) {
	if strings.TrimSpace(in.Title) == "" {
		return nil, apperror.UnprocessableEntity("title is required")
	}
	if in.StartDate.Time().IsZero() {
		return nil, apperror.UnprocessableEntity("start_date is required (YYYY-MM-DD)")
	}

	row, err := s.q.GetPublicTrip(ctx, sourceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperror.NotFound("trip not found")
		}
		return nil, apperror.FromDB(err)
	}
	source := row.Trip

	var delta time.Duration
	if source.StartDate != nil {
		delta = in.StartDate.Time().Sub(*source.StartDate)
	}

	dests, err := s.q.ListDestinations(ctx, sourceID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	entries, err := s.q.ListEntries(ctx, sourceID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	journeys, err := s.q.ListTransport(ctx, sourceID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, apperror.Internal("could not start transaction")
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)

	newStart := in.StartDate.Time()
	trip, err := qtx.InsertTrip(ctx, models.InsertTripParams{
		OwnerID:       callerID,
		Title:         in.Title,
		CoverImage:    source.CoverImage,
		Description:   source.Description,
		StartDate:     &newStart,
		EndDate:       shiftPtr(source.EndDate, delta),
		StartLocation: source.StartLocation,
		TripType:      source.TripType,
		Currency:      source.Currency,
		StartLat:      source.StartLat,
		StartLng:      source.StartLng,
	})
	if err != nil {
		return nil, apperror.FromDB(err)
	}

	for _, d := range dests {
		if _, err := qtx.InsertDestination(ctx, models.InsertDestinationParams{
			TripID: trip.ID, Name: d.Name, ArrivalDate: shiftPtr(d.ArrivalDate, delta), SortOrder: d.SortOrder,
			Lat: d.Lat, Lng: d.Lng,
		}); err != nil {
			return nil, apperror.FromDB(err)
		}
	}

	// Remap source entry ids → new entry ids so copied journeys point at copies.
	entryMap := make(map[uuid.UUID]uuid.UUID, len(entries))
	for _, e := range entries {
		ne, err := qtx.InsertEntry(ctx, models.InsertEntryParams{
			TripID: trip.ID, Title: e.Title, Description: e.Description,
			EntryDate: shiftTime(e.EntryDate, delta), EntryTime: e.EntryTime, Location: e.Location,
			Lat: e.Lat, Lng: e.Lng,
			CreatedBy: callerID,
		})
		if err != nil {
			return nil, apperror.FromDB(err)
		}
		entryMap[e.ID] = ne.ID
	}

	for _, j := range journeys {
		var entryID *uuid.UUID
		if j.EntryID != nil {
			if nid, ok := entryMap[*j.EntryID]; ok {
				entryID = &nid
			}
		}
		if _, err := qtx.InsertTransport(ctx, models.InsertTransportParams{
			TripID:         trip.ID,
			EntryID:        entryID,
			ExpenseID:      nil, // never copy the expense link
			Type:           j.Type,
			StartPoint:     j.StartPoint,
			Destination:    j.Destination,
			DepartAt:       shiftPtr(j.DepartAt, delta),
			DistanceKm:     j.DistanceKm,
			DurationMin:    j.DurationMin,
			CostAmount:     nil, // never copy cost
			Notes:          j.Notes,
			CreatedBy:      callerID,
			StartLat:       j.StartLat,
			StartLng:       j.StartLng,
			DestinationLat: j.DestinationLat,
			DestinationLng: j.DestinationLng,
		}); err != nil {
			return nil, apperror.FromDB(err)
		}
	}

	if err := recordActivity(ctx, qtx, callerID, activity{
		TripID: trip.ID, Action: "created", SubjectType: "trip", SubjectID: &trip.ID,
		Data: eventData{Title: trip.Title},
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, apperror.Internal("could not copy trip")
	}
	return &trip, nil
}

// ─── Saved trips ──────────────────────────────────────────────────────────────

// ensurePublic returns 404 unless the trip exists and is public — used so saving
// a private (or missing) trip never reveals its existence.
func (s *PublicService) ensurePublic(ctx context.Context, tripID uuid.UUID) error {
	if _, err := s.q.GetPublicTrip(ctx, tripID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperror.NotFound("trip not found")
		}
		return apperror.FromDB(err)
	}
	return nil
}

func (s *PublicService) Save(ctx context.Context, tripID, userID uuid.UUID) error {
	if err := s.ensurePublic(ctx, tripID); err != nil {
		return err
	}
	return apperror.FromDBOrNil(s.q.SaveTrip(ctx, models.SaveTripParams{UserID: userID, TripID: tripID}))
}

func (s *PublicService) Unsave(ctx context.Context, tripID, userID uuid.UUID) error {
	if err := s.ensurePublic(ctx, tripID); err != nil {
		return err
	}
	return apperror.FromDBOrNil(s.q.UnsaveTrip(ctx, models.UnsaveTripParams{UserID: userID, TripID: tripID}))
}

// ListSaved returns the caller's saved trips (public only, newest-saved first) in
// the same row shape as the discovery list.
func (s *PublicService) ListSaved(ctx context.Context, userID uuid.UUID) (*types.PublicTripListResponse, error) {
	rows, err := s.q.ListSavedTrips(ctx, userID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	items := make([]types.PublicTripListItem, len(rows))
	for i, r := range rows {
		items[i] = types.PublicTripListItem{
			Trip:         toPublicTripSafe(r.Trip),
			Destinations: r.DestinationNames,
			OwnerName:    r.OwnerName,
		}
	}
	return &types.PublicTripListResponse{Trips: items}, nil
}
