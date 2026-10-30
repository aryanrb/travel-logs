package services

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	db "travel-service/internal/database"
	"travel-service/internal/models"
	"travel-service/internal/types"
	"travel-service/internal/utils/apperror"
)

// eventData is the activity-feed payload. amount/currency are set only for
// expenses and settlements; title names the subject.
type eventData struct {
	Title    string  `json:"title,omitempty"`
	Amount   *int64  `json:"amount,omitempty"`
	Currency *string `json:"currency,omitempty"`
}

// activity describes one feed event to record alongside a mutation.
type activity struct {
	TripID      uuid.UUID
	Action      string // created | updated | deleted
	SubjectType string // entry | journey | expense | photo | settlement | participant | trip
	SubjectID   *uuid.UUID
	Data        eventData
}

// recordActivity writes a trip_event inside the caller's transaction, denormalising
// the actor's name so the feed survives a later rename/removal (rule 9).
func recordActivity(ctx context.Context, qtx *models.Queries, actorID uuid.UUID, a activity) error {
	actorName := "unknown user"
	if u, err := qtx.GetUserByID(ctx, actorID); err == nil {
		actorName = u.Name
	}
	raw, err := json.Marshal(a.Data)
	if err != nil {
		return apperror.Internal("could not encode activity")
	}
	_, err = qtx.InsertEvent(ctx, models.InsertEventParams{
		TripID:      a.TripID,
		ActorID:     actorID,
		ActorName:   actorName,
		Action:      a.Action,
		SubjectType: a.SubjectType,
		SubjectID:   a.SubjectID,
		Data:        raw,
	})
	return apperror.FromDBOrNil(err)
}

// inTx runs fn in a transaction, records the activity it returns (nil to record
// nothing), and commits — so the mutation and its event land together or not at
// all. Operations with their own transaction call recordActivity directly instead.
func inTx(ctx context.Context, pool *pgxpool.Pool, base *models.Queries, actorID uuid.UUID, fn func(qtx *models.Queries) (*activity, error)) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return apperror.Internal("could not start transaction")
	}
	defer tx.Rollback(ctx)
	qtx := base.WithTx(tx)
	a, err := fn(qtx)
	if err != nil {
		return err
	}
	if a != nil {
		if err := recordActivity(ctx, qtx, actorID, *a); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return apperror.Internal("could not save changes")
	}
	return nil
}

// ─── Activity feed ────────────────────────────────────────────────────────────

type EventService struct {
	q   *models.Queries
	log *zap.Logger
}

func NewEventService() *EventService {
	return &EventService{q: models.New(db.Client), log: zap.L().Named("EventService")}
}

const activityPageSize = 50

// List returns the newest events for a trip, keyset-paged on the optional
// `before` cursor. Any trip member may read the feed.
func (s *EventService) List(ctx context.Context, tripID, userID uuid.UUID, before *time.Time) (*types.ActivityResponse, error) {
	if _, _, err := requireViewer(ctx, s.q, tripID, userID); err != nil {
		return nil, err
	}
	rows, err := s.q.ListEvents(ctx, models.ListEventsParams{TripID: tripID, Before: before, Lim: activityPageSize})
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	return &types.ActivityResponse{Events: rows}, nil
}
