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

// notifyParams describes one notification to create.
type notifyParams struct {
	UserID     uuid.UUID // recipient
	Type       string
	ActorID    *uuid.UUID
	TripID     *uuid.UUID
	CommentID  *uuid.UUID
	RSVPStatus *string
}

// notify records a notification unless it would notify someone about their own
// action, or cross a block in either direction. Best-effort: a failure is logged,
// never surfaced — a missed notification must not fail the action that caused it.
func notify(ctx context.Context, q *models.Queries, p notifyParams) {
	if p.ActorID != nil && *p.ActorID == p.UserID {
		return // never notify people about their own actions
	}
	if p.ActorID != nil {
		if blocked, _ := isBlockedBetween(ctx, q, p.UserID, *p.ActorID); blocked {
			return
		}
	}
	if _, err := q.InsertNotification(ctx, models.InsertNotificationParams{
		UserID:     p.UserID,
		Type:       p.Type,
		ActorID:    p.ActorID,
		TripID:     p.TripID,
		CommentID:  p.CommentID,
		RsvpStatus: p.RSVPStatus,
	}); err != nil {
		zap.L().Named("notify").Warn("insert notification", zap.Error(err))
	}
}

const notificationPageSize = 30

type NotificationService struct {
	q   *models.Queries
	db  *pgxpool.Pool
	log *zap.Logger
}

func NewNotificationService() *NotificationService {
	return &NotificationService{q: models.New(db.Client), db: db.Client, log: zap.L().Named("NotificationService")}
}

func strVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// List returns the caller's notifications newest-first, keyset-paged on `cursor`,
// with the total unread count. Notifications from people now blocked are hidden.
func (s *NotificationService) List(ctx context.Context, userID uuid.UUID, cursor string) (*types.NotificationsResponse, error) {
	rows, err := s.q.ListNotifications(ctx, models.ListNotificationsParams{
		UserID: userID, Before: decodeTimeCursor(cursor), Lim: notificationPageSize,
	})
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	blocked, err := blockedSet(ctx, s.q, userID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}

	items := make([]types.NotificationItem, 0, len(rows))
	for _, r := range rows {
		if r.ActorID != nil && blocked[*r.ActorID] {
			continue // hide notifications from people now blocked (either way)
		}
		item := types.NotificationItem{
			ID: r.ID, Type: r.Type, CommentID: r.CommentID, RSVPStatus: r.RsvpStatus,
			CreatedAt: r.CreatedAt, Read: r.Read,
		}
		if r.ActorID != nil {
			item.Actor = &types.NotificationActor{ID: *r.ActorID, Name: strVal(r.ActorName), PhotoURL: r.ActorPhotoUrl}
		}
		if r.TripID != nil {
			item.Trip = &types.NotificationTrip{ID: *r.TripID, Title: strVal(r.TripTitle)}
		}
		items = append(items, item)
	}

	unread, err := s.q.CountUnreadNotifications(ctx, userID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	var next *string
	if len(rows) == notificationPageSize {
		c := encodeTimeCursor(rows[len(rows)-1].CreatedAt)
		next = &c
	}
	return &types.NotificationsResponse{Notifications: items, Unread: int(unread), NextCursor: next}, nil
}

// MarkRead marks the given ids read, or all of the caller's when ids is empty.
func (s *NotificationService) MarkRead(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return apperror.FromDBOrNil(s.q.MarkAllNotificationsRead(ctx, userID))
	}
	return apperror.FromDBOrNil(s.q.MarkNotificationsRead(ctx, models.MarkNotificationsReadParams{UserID: userID, Ids: ids}))
}
