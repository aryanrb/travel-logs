package services

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	db "travel-service/internal/database"
	"travel-service/internal/models"
	"travel-service/internal/types"
	"travel-service/internal/utils/apperror"
)

type SummaryService struct {
	q   *models.Queries
	db  *pgxpool.Pool
	log *zap.Logger
}

func NewSummaryService() *SummaryService {
	return &SummaryService{q: models.New(db.Client), db: db.Client, log: zap.L().Named("SummaryService")}
}

// Get assembles the trip summary from saved data. Expense totals come from the
// expenses table only, so a cost linked to both a timeline/transport entry and
// the tracker is never double-counted (req 3.8).
func (s *SummaryService) Get(ctx context.Context, tripID, userID uuid.UUID) (*types.SummaryResponse, error) {
	_, trip, err := requireViewer(ctx, s.q, tripID, userID)
	if err != nil {
		return nil, err
	}

	dests, err := s.q.ListDestinations(ctx, tripID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	entries, err := s.q.ListEntries(ctx, tripID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	photos, err := s.q.CountTripPhotos(ctx, tripID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	total, err := s.q.TotalExpenses(ctx, tripID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	cats, err := s.q.CategoryBreakdown(ctx, tripID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	transport, err := s.q.TransportBreakdown(ctx, tripID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	balances, err := computeTripBalances(ctx, s.q, tripID)
	if err != nil {
		return nil, err
	}

	return &types.SummaryResponse{
		Currency:          trip.Currency,
		StartDate:         trip.StartDate,
		EndDate:           trip.EndDate,
		TravelDays:        travelDays(trip),
		Destinations:      dests,
		TimelineEntries:   len(entries),
		Photos:            photos,
		TotalExpenses:     total,
		CategoryBreakdown: cats,
		Transportation:    transport,
		Balances:          balances,
	}, nil
}

// travelDays is the inclusive day count when both dates are set, else 0.
func travelDays(t models.Trip) int {
	if t.StartDate == nil || t.EndDate == nil {
		return 0
	}
	d := int(t.EndDate.Sub(*t.StartDate).Hours()/24) + 1
	if d < 0 {
		return 0
	}
	return d
}
