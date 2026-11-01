package services

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	db "travel-service/internal/database"
	"travel-service/internal/models"
	"travel-service/internal/types"
	"travel-service/internal/utils/apperror"
)

type PoolService struct {
	q   *models.Queries
	db  *pgxpool.Pool
	log *zap.Logger
}

func NewPoolService() *PoolService {
	return &PoolService{q: models.New(db.Client), db: db.Client, log: zap.L().Named("PoolService")}
}

// poolPerPerson returns a trip pool's per_person figure, or nil when none (used by
// the join page's planned_cost_per_person).
func poolPerPerson(ctx context.Context, q *models.Queries, tripID uuid.UUID) *int64 {
	p, err := q.GetPool(ctx, tripID)
	if err != nil {
		return nil
	}
	v := p.PerPerson
	return &v
}

func (s *PoolService) Get(ctx context.Context, tripID, userID uuid.UUID) (*types.PoolResponse, error) {
	if _, _, err := requireViewer(ctx, s.q, tripID, userID); err != nil {
		return nil, err
	}
	pool, err := s.q.GetPool(ctx, tripID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperror.NotFound("this trip has no pool")
		}
		return nil, apperror.FromDB(err)
	}
	collected, err := s.q.PoolCollected(ctx, tripID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	spent, err := s.q.PoolSpent(ctx, tripID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	return &types.PoolResponse{
		Enabled: true, CollectorID: pool.CollectorID, PerPerson: pool.PerPerson,
		DueDate: types.DateFromPtr(pool.DueDate), Collected: collected, Spent: spent,
	}, nil
}

// Set creates or updates the pool (owner only). The collector must be a current
// member with an account.
func (s *PoolService) Set(ctx context.Context, tripID, userID uuid.UUID, in types.PoolInput) (*types.PoolResponse, error) {
	if _, err := requireOwner(ctx, s.q, tripID, userID); err != nil {
		return nil, err
	}
	if in.PerPerson <= 0 {
		return nil, apperror.UnprocessableEntity("per_person must be greater than 0 (in minor units)")
	}
	if err := s.requireMember(ctx, tripID, in.CollectorID); err != nil {
		return nil, err
	}
	if _, err := s.q.UpsertPool(ctx, models.UpsertPoolParams{
		TripID: tripID, CollectorID: in.CollectorID, PerPerson: in.PerPerson, DueDate: types.DatePtr(in.DueDate),
	}); err != nil {
		return nil, apperror.FromDB(err)
	}
	return s.Get(ctx, tripID, userID)
}

func (s *PoolService) Delete(ctx context.Context, tripID, userID uuid.UUID) error {
	if _, err := requireOwner(ctx, s.q, tripID, userID); err != nil {
		return err
	}
	// Contributions stay as settlements; pool expenses stay paid by the collector.
	return apperror.FromDBOrNil(s.q.DeletePool(ctx, tripID))
}

// requireMember confirms a user is the owner or an accepted participant with an
// account — the only valid collectors.
func (s *PoolService) requireMember(ctx context.Context, tripID, candidate uuid.UUID) error {
	trip, err := s.q.GetTrip(ctx, tripID)
	if err != nil {
		return apperror.FromDB(err)
	}
	if trip.OwnerID == candidate {
		return nil
	}
	uid := candidate
	p, err := s.q.GetParticipantByTripUser(ctx, models.GetParticipantByTripUserParams{TripID: tripID, UserID: &uid})
	if err != nil || p.Status != "accepted" {
		return apperror.UnprocessableEntity("collector_id must be a current trip member")
	}
	return nil
}
