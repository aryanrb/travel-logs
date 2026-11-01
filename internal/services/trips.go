package services

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	db "travel-service/internal/database"
	"travel-service/internal/models"
	"travel-service/internal/types"
	"travel-service/internal/utils/apperror"
)

type TripService struct {
	q   *models.Queries
	db  *pgxpool.Pool
	log *zap.Logger
}

func NewTripService() *TripService {
	return &TripService{q: models.New(db.Client), db: db.Client, log: zap.L().Named("TripService")}
}

func currencyOr(c *string) string {
	if c != nil && strings.TrimSpace(*c) != "" {
		return strings.ToUpper(strings.TrimSpace(*c))
	}
	return "USD"
}

// budgetCategories is the fixed set a category budget may target. Expense
// categories are free-form; budgets are constrained so a budget always maps to a
// recognisable category.
var budgetCategories = map[string]bool{
	"food": true, "transport": true, "accommodation": true, "activities": true,
	"shopping": true, "groceries": true, "entertainment": true, "health": true,
	"fees": true, "other": true,
}

func toCategoryBudgets(rows []models.TripCategoryBudget) []types.CategoryBudget {
	out := make([]types.CategoryBudget, len(rows))
	for i, r := range rows {
		out[i] = types.CategoryBudget{Category: r.Category, Amount: r.Amount}
	}
	return out
}

// tripResponse attaches owner_name (no email) and category_budgets to a single
// trip. budget_amount and start coords are columns on the trip itself.
func (s *TripService) tripResponse(ctx context.Context, trip models.Trip) (*types.TripResponse, error) {
	name := ""
	if u, err := s.q.GetUserByID(ctx, trip.OwnerID); err == nil {
		name = u.Name
	}
	cats, err := s.q.ListCategoryBudgets(ctx, trip.ID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	return &types.TripResponse{Trip: trip, OwnerName: name, CategoryBudgets: toCategoryBudgets(cats)}, nil
}

func (s *TripService) Create(ctx context.Context, userID uuid.UUID, in types.TripInput) (*types.TripResponse, error) {
	if strings.TrimSpace(in.Title) == "" {
		return nil, apperror.UnprocessableEntity("trip title is required")
	}
	if err := validateCoords(in.StartLat, in.StartLng); err != nil {
		return nil, err
	}
	cover, err := normalizeHTTPSURL(in.CoverImage)
	if err != nil {
		return nil, err
	}
	var trip models.Trip
	err = inTx(ctx, s.db, s.q, userID, func(qtx *models.Queries) (*activity, error) {
		t, err := qtx.InsertTrip(ctx, models.InsertTripParams{
			OwnerID:       userID,
			Title:         in.Title,
			CoverImage:    cover,
			Description:   in.Description,
			StartDate:     types.DatePtr(in.StartDate),
			EndDate:       types.DatePtr(in.EndDate),
			StartLocation: in.StartLocation,
			TripType:      in.TripType,
			Currency:      currencyOr(in.Currency),
			StartLat:      in.StartLat,
			StartLng:      in.StartLng,
		})
		if err != nil {
			return nil, apperror.FromDB(err)
		}
		trip = t
		return &activity{TripID: t.ID, Action: "created", SubjectType: "trip", SubjectID: &t.ID,
			Data: eventData{Title: t.Title}}, nil
	})
	if err != nil {
		return nil, err
	}
	// A pin wins over a name lookup: only geocode when no pin was set.
	if in.StartLat == nil && trip.StartLocation != nil {
		go geocodeTripStart(trip.ID, *trip.StartLocation)
	}
	return s.tripResponse(ctx, trip)
}

func (s *TripService) List(ctx context.Context, userID uuid.UUID) (*types.TripsListResponse, error) {
	owned, err := s.q.ListOwnedTrips(ctx, userID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	uid := userID
	shared, err := s.q.ListSharedTrips(ctx, &uid)
	if err != nil {
		return nil, apperror.FromDB(err)
	}

	all := make([]models.Trip, 0, len(owned)+len(shared))
	all = append(all, owned...)
	for _, sh := range shared {
		all = append(all, sh.Trip)
	}
	names, err := s.ownerNames(ctx, all)
	if err != nil {
		return nil, err
	}
	budgets, err := s.categoryBudgetsFor(ctx, all)
	if err != nil {
		return nil, err
	}

	ownedResp := make([]types.TripResponse, len(owned))
	for i, t := range owned {
		ownedResp[i] = types.TripResponse{Trip: t, OwnerName: names[t.OwnerID], CategoryBudgets: budgets[t.ID]}
	}
	sharedResp := make([]types.SharedTripResponse, len(shared))
	for i, sh := range shared {
		sharedResp[i] = types.SharedTripResponse{
			Trip: types.TripResponse{Trip: sh.Trip, OwnerName: names[sh.Trip.OwnerID], CategoryBudgets: budgets[sh.Trip.ID]},
			Role: sh.Role,
		}
	}
	return &types.TripsListResponse{Owned: ownedResp, Shared: sharedResp}, nil
}

// ownerNames resolves owner ids → names in one query (no emails exposed).
func (s *TripService) ownerNames(ctx context.Context, trips []models.Trip) (map[uuid.UUID]string, error) {
	seen := map[uuid.UUID]bool{}
	var ids []uuid.UUID
	for _, t := range trips {
		if !seen[t.OwnerID] {
			seen[t.OwnerID] = true
			ids = append(ids, t.OwnerID)
		}
	}
	names := map[uuid.UUID]string{}
	if len(ids) == 0 {
		return names, nil
	}
	users, err := s.q.GetUsersByIDs(ctx, ids)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	for _, u := range users {
		names[u.ID] = u.Name
	}
	return names, nil
}

// categoryBudgetsFor fetches category budgets for many trips in one query, so the
// list endpoint stays a fixed number of queries.
func (s *TripService) categoryBudgetsFor(ctx context.Context, trips []models.Trip) (map[uuid.UUID][]types.CategoryBudget, error) {
	var ids []uuid.UUID
	for _, t := range trips {
		ids = append(ids, t.ID)
	}
	out := map[uuid.UUID][]types.CategoryBudget{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.q.ListCategoryBudgetsForTrips(ctx, ids)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	for _, r := range rows {
		out[r.TripID] = append(out[r.TripID], types.CategoryBudget{Category: r.Category, Amount: r.Amount})
	}
	return out, nil
}

func (s *TripService) Get(ctx context.Context, tripID, userID uuid.UUID) (*types.TripDetailResponse, error) {
	role, trip, err := requireViewer(ctx, s.q, tripID, userID)
	if err != nil {
		return nil, err
	}
	dests, err := s.q.ListDestinations(ctx, tripID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	tr, err := s.tripResponse(ctx, trip)
	if err != nil {
		return nil, err
	}
	return &types.TripDetailResponse{Trip: *tr, Destinations: dests, YourRole: role}, nil
}

func (s *TripService) Update(ctx context.Context, tripID, userID uuid.UUID, in types.TripInput) (*types.TripResponse, error) {
	if _, err := requireOwner(ctx, s.q, tripID, userID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Title) == "" {
		return nil, apperror.UnprocessableEntity("trip title is required")
	}
	if err := validateCoords(in.StartLat, in.StartLng); err != nil {
		return nil, err
	}
	cover, err := normalizeHTTPSURL(in.CoverImage)
	if err != nil {
		return nil, err
	}
	err = inTx(ctx, s.db, s.q, userID, func(qtx *models.Queries) (*activity, error) {
		if err := qtx.UpdateTrip(ctx, models.UpdateTripParams{
			Title:         in.Title,
			CoverImage:    cover,
			Description:   in.Description,
			StartDate:     types.DatePtr(in.StartDate),
			EndDate:       types.DatePtr(in.EndDate),
			StartLocation: in.StartLocation,
			TripType:      in.TripType,
			Currency:      currencyOr(in.Currency),
			ID:            tripID,
		}); err != nil {
			return nil, apperror.FromDB(err)
		}
		return &activity{TripID: tripID, Action: "updated", SubjectType: "trip", SubjectID: &tripID,
			Data: eventData{Title: in.Title}}, nil
	})
	if err != nil {
		return nil, err
	}
	// A pin wins: persist it and skip the lookup. Without a pin, geocode by name
	// as before (UpdateTrip does not touch coords, so existing ones are kept).
	if in.StartLat != nil {
		if err := apperror.FromDBOrNil(s.q.UpdateTripStartCoords(ctx, models.UpdateTripStartCoordsParams{
			StartLat: in.StartLat, StartLng: in.StartLng, ID: tripID,
		})); err != nil {
			return nil, err
		}
	} else if in.StartLocation != nil {
		go geocodeTripStart(tripID, *in.StartLocation)
	}
	trip, err := s.q.GetTrip(ctx, tripID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	return s.tripResponse(ctx, trip)
}

// SetBudget replaces a trip's whole budget — the overall amount and the
// per-category budgets — in one transaction (owner only). Returns the trip.
func (s *TripService) SetBudget(ctx context.Context, tripID, userID uuid.UUID, in types.BudgetInput) (*types.TripResponse, error) {
	if _, err := requireOwner(ctx, s.q, tripID, userID); err != nil {
		return nil, err
	}
	if err := validateBudgetInput(in); err != nil {
		return nil, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, apperror.Internal("could not start transaction")
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)

	if err := qtx.SetTripBudget(ctx, models.SetTripBudgetParams{BudgetAmount: in.BudgetAmount, ID: tripID}); err != nil {
		return nil, apperror.FromDB(err)
	}
	if err := qtx.DeleteCategoryBudgets(ctx, tripID); err != nil {
		return nil, apperror.FromDB(err)
	}
	for _, cb := range in.CategoryBudgets {
		if err := qtx.InsertCategoryBudget(ctx, models.InsertCategoryBudgetParams{
			TripID:   tripID,
			Category: strings.ToLower(strings.TrimSpace(cb.Category)),
			Amount:   cb.Amount,
		}); err != nil {
			return nil, apperror.FromDB(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, apperror.Internal("could not save budget")
	}
	trip, err := s.q.GetTrip(ctx, tripID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	return s.tripResponse(ctx, trip)
}

// validateBudgetInput rejects non-positive amounts, duplicate categories and
// categories outside budgetCategories.
func validateBudgetInput(in types.BudgetInput) error {
	if in.BudgetAmount != nil && *in.BudgetAmount <= 0 {
		return apperror.UnprocessableEntity("budget_amount must be greater than 0 (in minor units)")
	}
	seen := map[string]bool{}
	for _, cb := range in.CategoryBudgets {
		cat := strings.ToLower(strings.TrimSpace(cb.Category))
		if cat == "" {
			return apperror.UnprocessableEntity("category is required")
		}
		if !budgetCategories[cat] {
			return apperror.UnprocessableEntity("unknown category: " + cb.Category)
		}
		if seen[cat] {
			return apperror.UnprocessableEntity("duplicate category: " + cb.Category)
		}
		seen[cat] = true
		if cb.Amount <= 0 {
			return apperror.UnprocessableEntity("category amount must be greater than 0 (in minor units)")
		}
	}
	return nil
}

func (s *TripService) SetPublic(ctx context.Context, tripID, userID uuid.UUID, isPublic, showCostsPublic bool) error {
	if _, err := requireOwner(ctx, s.q, tripID, userID); err != nil {
		return err
	}
	return apperror.FromDBOrNil(s.q.SetTripPublic(ctx, models.SetTripPublicParams{
		IsPublic: isPublic, ShowCostsPublic: showCostsPublic, ID: tripID,
	}))
}

func (s *TripService) Archive(ctx context.Context, tripID, userID uuid.UUID) error {
	if _, err := requireOwner(ctx, s.q, tripID, userID); err != nil {
		return err
	}
	return apperror.FromDBOrNil(s.q.ArchiveTrip(ctx, tripID))
}

// Delete hard-deletes the trip and all its content (cascade). The frontend is
// responsible for the confirmation step (req 3.2).
func (s *TripService) Delete(ctx context.Context, tripID, userID uuid.UUID) error {
	if _, err := requireOwner(ctx, s.q, tripID, userID); err != nil {
		return err
	}
	return apperror.FromDBOrNil(s.q.DeleteTrip(ctx, tripID))
}

func (s *TripService) AddDestination(ctx context.Context, tripID, userID uuid.UUID, in types.DestinationInput) (*models.TripDestination, error) {
	if _, err := requireOwner(ctx, s.q, tripID, userID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Name) == "" {
		return nil, apperror.UnprocessableEntity("destination name is required")
	}
	if err := validateCoords(in.Lat, in.Lng); err != nil {
		return nil, err
	}
	d, err := s.q.InsertDestination(ctx, models.InsertDestinationParams{
		TripID: tripID, Name: in.Name, ArrivalDate: types.DatePtr(in.ArrivalDate), SortOrder: in.SortOrder,
		Lat: in.Lat, Lng: in.Lng,
	})
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	// A pin wins over a name lookup.
	if in.Lat == nil {
		go geocodeDestination(d.ID, d.Name)
	}
	return &d, nil
}

func (s *TripService) DeleteDestination(ctx context.Context, tripID, destID, userID uuid.UUID) error {
	if _, err := requireOwner(ctx, s.q, tripID, userID); err != nil {
		return err
	}
	return apperror.FromDBOrNil(s.q.DeleteDestination(ctx, models.DeleteDestinationParams{ID: destID, TripID: tripID}))
}
