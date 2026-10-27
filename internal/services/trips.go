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

func (s *TripService) Create(ctx context.Context, userID uuid.UUID, in types.TripInput) (*models.Trip, error) {
	if strings.TrimSpace(in.Title) == "" {
		return nil, apperror.UnprocessableEntity("trip title is required")
	}
	trip, err := s.q.InsertTrip(ctx, models.InsertTripParams{
		OwnerID:       userID,
		Title:         in.Title,
		CoverImage:    in.CoverImage,
		Description:   in.Description,
		StartDate:     types.DatePtr(in.StartDate),
		EndDate:       types.DatePtr(in.EndDate),
		StartLocation: in.StartLocation,
		TripType:      in.TripType,
		Currency:      currencyOr(in.Currency),
	})
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	return &trip, nil
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
	return &types.TripsListResponse{Owned: owned, Shared: shared}, nil
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
	return &types.TripDetailResponse{Trip: trip, Destinations: dests, YourRole: role}, nil
}

func (s *TripService) Update(ctx context.Context, tripID, userID uuid.UUID, in types.TripInput) error {
	if _, err := requireOwner(ctx, s.q, tripID, userID); err != nil {
		return err
	}
	if strings.TrimSpace(in.Title) == "" {
		return apperror.UnprocessableEntity("trip title is required")
	}
	return apperror.FromDBOrNil(s.q.UpdateTrip(ctx, models.UpdateTripParams{
		Title:         in.Title,
		CoverImage:    in.CoverImage,
		Description:   in.Description,
		StartDate:     types.DatePtr(in.StartDate),
		EndDate:       types.DatePtr(in.EndDate),
		StartLocation: in.StartLocation,
		TripType:      in.TripType,
		Currency:      currencyOr(in.Currency),
		ID:            tripID,
	}))
}

func (s *TripService) SetPublic(ctx context.Context, tripID, userID uuid.UUID, isPublic bool) error {
	if _, err := requireOwner(ctx, s.q, tripID, userID); err != nil {
		return err
	}
	return apperror.FromDBOrNil(s.q.SetTripPublic(ctx, models.SetTripPublicParams{IsPublic: isPublic, ID: tripID}))
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
	d, err := s.q.InsertDestination(ctx, models.InsertDestinationParams{
		TripID: tripID, Name: in.Name, ArrivalDate: types.DatePtr(in.ArrivalDate), SortOrder: in.SortOrder,
	})
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	return &d, nil
}

func (s *TripService) DeleteDestination(ctx context.Context, tripID, destID, userID uuid.UUID) error {
	if _, err := requireOwner(ctx, s.q, tripID, userID); err != nil {
		return err
	}
	return apperror.FromDBOrNil(s.q.DeleteDestination(ctx, models.DeleteDestinationParams{ID: destID, TripID: tripID}))
}
