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
	exp *ExpenseService // for nested expenses created alongside a journey
	log *zap.Logger
}

func NewTransportService() *TransportService {
	return &TransportService{q: models.New(db.Client), db: db.Client, exp: NewExpenseService(), log: zap.L().Named("TransportService")}
}

// verifyTransportLinks checks that an optional linked entry/expense belong to this
// trip, so a journey can't be stitched to another trip's records.
func verifyTransportLinks(ctx context.Context, q *models.Queries, tripID uuid.UUID, entryID, expenseID *uuid.UUID) error {
	if entryID != nil {
		e, err := q.GetEntry(ctx, *entryID)
		if err != nil || e.TripID != tripID {
			return apperror.BadRequest("entry_id does not belong to this trip")
		}
	}
	if expenseID != nil {
		e, err := q.GetExpense(ctx, *expenseID)
		if err != nil || e.TripID != tripID {
			return apperror.BadRequest("expense_id does not belong to this trip")
		}
	}
	return nil
}

// journeyTitle labels a journey for the activity feed: its route when both ends
// are known, else its type.
func journeyTitle(start, dest *string, typ string) string {
	if start != nil && dest != nil && *start != "" && *dest != "" {
		return *start + " → " + *dest
	}
	return typ
}

// nestedExpenseID creates the journey's optional nested expense and returns its id
// as the expense_id to use (overriding any expense_id in the body). With no nested
// expense it returns the body's expense_id unchanged.
func (s *TransportService) nestedExpenseID(ctx context.Context, qtx *models.Queries, tripID, userID uuid.UUID, trip models.Trip, in types.TransportInput) (*uuid.UUID, error) {
	if in.Expense == nil {
		return in.ExpenseID, nil
	}
	expIn := *in.Expense
	if err := validateExpenseInput(expIn); err != nil {
		return nil, err
	}
	exp, err := s.exp.insertWithinTx(ctx, qtx, tripID, userID, expIn, resolveCurrency(trip, expIn.Currency))
	if err != nil {
		return nil, err
	}
	return &exp.ID, nil
}

func (s *TransportService) Create(ctx context.Context, tripID, userID uuid.UUID, in types.TransportInput) (*models.Transportation, error) {
	_, trip, err := requireContributor(ctx, s.q, tripID, userID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Type) == "" {
		return nil, apperror.UnprocessableEntity("transportation type is required")
	}
	var created models.Transportation
	err = inTx(ctx, s.db, s.q, userID, func(qtx *models.Queries) (*activity, error) {
		expenseID, err := s.nestedExpenseID(ctx, qtx, tripID, userID, trip, in)
		if err != nil {
			return nil, err
		}
		if err := verifyTransportLinks(ctx, qtx, tripID, in.EntryID, expenseID); err != nil {
			return nil, err
		}
		t, err := qtx.InsertTransport(ctx, models.InsertTransportParams{
			TripID:      tripID,
			EntryID:     in.EntryID,
			ExpenseID:   expenseID,
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
		created = t
		return &activity{TripID: tripID, Action: "created", SubjectType: "journey", SubjectID: &t.ID,
			Data: eventData{Title: journeyTitle(t.StartPoint, t.Destination, t.Type)}}, nil
	})
	if err != nil {
		return nil, err
	}
	return &created, nil
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
	role, trip, err := requireContributor(ctx, s.q, tripID, userID)
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
	return inTx(ctx, s.db, s.q, userID, func(qtx *models.Queries) (*activity, error) {
		expenseID, err := s.nestedExpenseID(ctx, qtx, tripID, userID, trip, in)
		if err != nil {
			return nil, err
		}
		if err := verifyTransportLinks(ctx, qtx, tripID, in.EntryID, expenseID); err != nil {
			return nil, err
		}
		if err := qtx.UpdateTransport(ctx, models.UpdateTransportParams{
			EntryID:     in.EntryID,
			ExpenseID:   expenseID,
			Type:        in.Type,
			StartPoint:  in.StartPoint,
			Destination: in.Destination,
			DepartAt:    in.DepartAt,
			DistanceKm:  in.DistanceKm,
			DurationMin: in.DurationMin,
			CostAmount:  in.CostAmount,
			Notes:       in.Notes,
			ID:          id,
		}); err != nil {
			return nil, apperror.FromDB(err)
		}
		return &activity{TripID: tripID, Action: "updated", SubjectType: "journey", SubjectID: &id,
			Data: eventData{Title: journeyTitle(in.StartPoint, in.Destination, in.Type)}}, nil
	})
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
	return inTx(ctx, s.db, s.q, userID, func(qtx *models.Queries) (*activity, error) {
		if err := qtx.DeleteTransport(ctx, id); err != nil {
			return nil, apperror.FromDB(err)
		}
		return &activity{TripID: tripID, Action: "deleted", SubjectType: "journey", SubjectID: &t.ID,
			Data: eventData{Title: journeyTitle(t.StartPoint, t.Destination, t.Type)}}, nil
	})
}
