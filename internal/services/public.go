package services

import (
	"context"
	"errors"
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

// publicTripLimit caps discovery results.
const publicTripLimit = 50

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
		ID:          t.ID,
		Type:        t.Type,
		StartPoint:  t.StartPoint,
		Destination: t.Destination,
		DepartAt:    t.DepartAt,
		DistanceKm:  t.DistanceKm,
		DurationMin: t.DurationMin,
		Notes:       t.Notes,
		EntryID:     t.EntryID,
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

func (s *PublicService) ListTrips(ctx context.Context, q, tripType string) (*types.PublicTripListResponse, error) {
	rows, err := s.q.ListPublicTrips(ctx, models.ListPublicTripsParams{
		Q:        strings.TrimSpace(q),
		TripType: strings.TrimSpace(tripType),
		Lim:      publicTripLimit,
	})
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

func (s *PublicService) GetTrip(ctx context.Context, tripID uuid.UUID) (*types.PublicTripDetail, error) {
	row, err := s.q.GetPublicTrip(ctx, tripID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperror.NotFound("trip not found") // also covers private trips
		}
		return nil, apperror.FromDB(err)
	}
	trip := row.Trip

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

	return &types.PublicTripDetail{
		Trip:           toPublicTripSafe(trip),
		Destinations:   dests,
		People:         people,
		Entries:        entries,
		Transportation: trans,
		Photos:         photos,
		Budget:         budget,
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
	})
	if err != nil {
		return nil, apperror.FromDB(err)
	}

	for _, d := range dests {
		if _, err := qtx.InsertDestination(ctx, models.InsertDestinationParams{
			TripID: trip.ID, Name: d.Name, ArrivalDate: shiftPtr(d.ArrivalDate, delta), SortOrder: d.SortOrder,
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
			TripID:      trip.ID,
			EntryID:     entryID,
			ExpenseID:   nil, // never copy the expense link
			Type:        j.Type,
			StartPoint:  j.StartPoint,
			Destination: j.Destination,
			DepartAt:    shiftPtr(j.DepartAt, delta),
			DistanceKm:  j.DistanceKm,
			DurationMin: j.DurationMin,
			CostAmount:  nil, // never copy cost
			Notes:       j.Notes,
			CreatedBy:   callerID,
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
