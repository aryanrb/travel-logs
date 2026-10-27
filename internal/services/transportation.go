package services

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	db "travel-service/internal/database"
	"travel-service/internal/models"
	"travel-service/internal/types"
	"travel-service/internal/utils/apperror"
)

type TransportService struct {
	q   *models.Queries
	db  *pgxpool.Pool
	log *zap.Logger
}

func NewTransportService() *TransportService {
	return &TransportService{q: models.New(db.Client), db: db.Client, log: zap.L().Named("TransportService")}
}

// verifyLinks checks that an optional linked entry/expense belong to this trip,
// so transport can't be stitched to another trip's records.
func (s *TransportService) verifyLinks(ctx context.Context, tripID uuid.UUID, entryID, expenseID *uuid.UUID) error {
	if entryID != nil {
		e, err := s.q.GetEntry(ctx, *entryID)
		if err != nil || e.TripID != tripID {
			return apperror.BadRequest("entry_id does not belong to this trip")
		}
	}
	if expenseID != nil {
		e, err := s.q.GetExpense(ctx, *expenseID)
		if err != nil || e.TripID != tripID {
			return apperror.BadRequest("expense_id does not belong to this trip")
		}
	}
	return nil
}

func (s *TransportService) Create(ctx context.Context, tripID, userID uuid.UUID, in types.TransportInput) (*models.Transportation, error) {
	if _, _, err := requireContributor(ctx, s.q, tripID, userID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Type) == "" {
		return nil, apperror.UnprocessableEntity("transportation type is required")
	}
	if err := s.verifyLinks(ctx, tripID, in.EntryID, in.ExpenseID); err != nil {
		return nil, err
	}
	t, err := s.q.InsertTransport(ctx, models.InsertTransportParams{
		TripID:      tripID,
		EntryID:     in.EntryID,
		ExpenseID:   in.ExpenseID,
		Type:        in.Type,
		StartPoint:  in.StartPoint,
		Destination: in.Destination,
		DepartAt:    in.DepartAt,
		DistanceKm:  in.DistanceKm,
		DurationMin: in.DurationMin,
		CostAmount:  in.CostAmount,
		Notes:       in.Notes,
		CreatedBy:   userID,
	})
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	return &t, nil
}

func (s *TransportService) List(ctx context.Context, tripID, userID uuid.UUID) ([]models.Transportation, error) {
	if _, _, err := requireViewer(ctx, s.q, tripID, userID); err != nil {
		return nil, err
	}
	rows, err := s.q.ListTransport(ctx, tripID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	return rows, nil
}

func (s *TransportService) transportInTrip(ctx context.Context, id, tripID uuid.UUID) (models.Transportation, error) {
	t, err := s.q.GetTransport(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return t, apperror.NotFound("transportation not found")
		}
		return t, apperror.FromDB(err)
	}
	if t.TripID != tripID {
		return t, apperror.NotFound("transportation not found")
	}
	return t, nil
}

func (s *TransportService) Update(ctx context.Context, tripID, id, userID uuid.UUID, in types.TransportInput) error {
	role, _, err := requireContributor(ctx, s.q, tripID, userID)
	if err != nil {
		return err
	}
	t, err := s.transportInTrip(ctx, id, tripID)
	if err != nil {
		return err
	}
	if !canModify(role, t.CreatedBy, userID) {
		return apperror.Forbidden("you can only edit transportation you created")
	}
	if strings.TrimSpace(in.Type) == "" {
		return apperror.UnprocessableEntity("transportation type is required")
	}
	if err := s.verifyLinks(ctx, tripID, in.EntryID, in.ExpenseID); err != nil {
		return err
	}
	return apperror.FromDBOrNil(s.q.UpdateTransport(ctx, models.UpdateTransportParams{
		EntryID:     in.EntryID,
		ExpenseID:   in.ExpenseID,
		Type:        in.Type,
		StartPoint:  in.StartPoint,
		Destination: in.Destination,
		DepartAt:    in.DepartAt,
		DistanceKm:  in.DistanceKm,
		DurationMin: in.DurationMin,
		CostAmount:  in.CostAmount,
		Notes:       in.Notes,
		ID:          id,
	}))
}

func (s *TransportService) Delete(ctx context.Context, tripID, id, userID uuid.UUID) error {
	role, _, err := requireContributor(ctx, s.q, tripID, userID)
	if err != nil {
		return err
	}
	t, err := s.transportInTrip(ctx, id, tripID)
	if err != nil {
		return err
	}
	if !canModify(role, t.CreatedBy, userID) {
		return apperror.Forbidden("you can only delete transportation you created")
	}
	return apperror.FromDBOrNil(s.q.DeleteTransport(ctx, id))
}
