package services

import (
	"context"
	"encoding/base64"
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

const (
	commentPageSize = 50
	maxCommentLen   = 1000
)

type CommentService struct {
	q   *models.Queries
	db  *pgxpool.Pool
	log *zap.Logger
}

func NewCommentService() *CommentService {
	return &CommentService{q: models.New(db.Client), db: db.Client, log: zap.L().Named("CommentService")}
}

// requirePublicTrip returns the trip only if it is public (else 404).
func (s *CommentService) requirePublicTrip(ctx context.Context, tripID uuid.UUID) (models.Trip, error) {
	row, err := s.q.GetPublicTrip(ctx, tripID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.Trip{}, apperror.NotFound("trip not found")
		}
		return models.Trip{}, apperror.FromDB(err)
	}
	return row.Trip, nil
}

func (s *CommentService) List(ctx context.Context, tripID uuid.UUID, cursor string, viewerID *uuid.UUID) (*types.CommentsResponse, error) {
	if _, err := s.requirePublicTrip(ctx, tripID); err != nil {
		return nil, err
	}
	blocked := []uuid.UUID{} // non-nil: `<> ALL(NULL)` would drop every row
	if viewerID != nil {
		ids, err := s.q.BlockedUserIDs(ctx, *viewerID)
		if err != nil {
			return nil, apperror.FromDB(err)
		}
		if len(ids) > 0 {
			blocked = ids
		}
	}
	rows, err := s.q.ListComments(ctx, models.ListCommentsParams{
		TripID: tripID, Blocked: blocked, After: decodeTimeCursor(cursor), Lim: commentPageSize,
	})
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	items := make([]types.CommentItem, len(rows))
	for i, r := range rows {
		items[i] = types.CommentItem{
			ID:        r.ID,
			Author:    types.CommentAuthor{ID: r.AuthorID, Name: r.AuthorName, PhotoURL: r.AuthorPhotoUrl},
			Body:      r.Body,
			CreatedAt: r.CreatedAt,
			EditedAt:  r.EditedAt,
			ParentID:  r.ParentID,
		}
	}
	var next *string
	if len(rows) == commentPageSize {
		c := encodeTimeCursor(rows[len(rows)-1].CreatedAt)
		next = &c
	}
	return &types.CommentsResponse{Comments: items, NextCursor: next}, nil
}

func (s *CommentService) Create(ctx context.Context, tripID, userID uuid.UUID, in types.CommentInput) (*types.CommentItem, error) {
	trip, err := s.requirePublicTrip(ctx, tripID)
	if err != nil {
		return nil, err
	}
	if blocked, _ := isBlockedBetween(ctx, s.q, userID, trip.OwnerID); blocked {
		return nil, apperror.NotFound("trip not found") // blocked can't comment
	}
	body := strings.TrimSpace(in.Body)
	if n := len([]rune(body)); n < 1 || n > maxCommentLen {
		return nil, apperror.UnprocessableEntity("comment must be between 1 and 1000 characters")
	}

	var parentAuthor *uuid.UUID
	if in.ParentID != nil {
		parent, err := s.q.GetComment(ctx, *in.ParentID)
		if err != nil || parent.TripID != tripID {
			return nil, apperror.UnprocessableEntity("parent comment does not belong to this trip")
		}
		if parent.ParentID != nil {
			return nil, apperror.UnprocessableEntity("replies can only be one level deep")
		}
		pa := parent.AuthorID
		parentAuthor = &pa
	}

	c, err := s.q.InsertComment(ctx, models.InsertCommentParams{
		TripID: tripID, AuthorID: userID, ParentID: in.ParentID, Body: body,
	})
	if err != nil {
		return nil, apperror.FromDB(err)
	}

	// Notifications: top-level → trip owner; reply → parent's author.
	if parentAuthor == nil {
		notify(ctx, s.q, notifyParams{UserID: trip.OwnerID, Type: "comment", ActorID: &userID, TripID: &tripID, CommentID: &c.ID})
	} else {
		notify(ctx, s.q, notifyParams{UserID: *parentAuthor, Type: "comment_reply", ActorID: &userID, TripID: &tripID, CommentID: &c.ID})
	}

	return s.itemFor(ctx, c)
}

func (s *CommentService) Update(ctx context.Context, commentID, userID uuid.UUID, in types.UpdateCommentInput) (*types.CommentItem, error) {
	c, err := s.commentByID(ctx, commentID)
	if err != nil {
		return nil, err
	}
	if c.AuthorID != userID {
		return nil, apperror.Forbidden("you can only edit your own comment")
	}
	body := strings.TrimSpace(in.Body)
	if n := len([]rune(body)); n < 1 || n > maxCommentLen {
		return nil, apperror.UnprocessableEntity("comment must be between 1 and 1000 characters")
	}
	updated, err := s.q.UpdateComment(ctx, models.UpdateCommentParams{Body: body, ID: commentID})
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	return s.itemFor(ctx, updated)
}

func (s *CommentService) Delete(ctx context.Context, commentID, userID uuid.UUID) error {
	c, err := s.commentByID(ctx, commentID)
	if err != nil {
		return err
	}
	if c.AuthorID != userID {
		trip, err := s.q.GetTrip(ctx, c.TripID)
		if err != nil {
			return apperror.FromDB(err)
		}
		if trip.OwnerID != userID {
			return apperror.Forbidden("only the author or the trip owner can delete this comment")
		}
	}
	return apperror.FromDBOrNil(s.q.DeleteComment(ctx, commentID)) // replies cascade
}

func (s *CommentService) commentByID(ctx context.Context, id uuid.UUID) (models.Comment, error) {
	c, err := s.q.GetComment(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c, apperror.NotFound("comment not found")
		}
		return c, apperror.FromDB(err)
	}
	return c, nil
}

func (s *CommentService) itemFor(ctx context.Context, c models.Comment) (*types.CommentItem, error) {
	author, err := s.q.GetUserByID(ctx, c.AuthorID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	return &types.CommentItem{
		ID:        c.ID,
		Author:    types.CommentAuthor{ID: author.ID, Name: author.Name, PhotoURL: author.PhotoUrl},
		Body:      c.Body,
		CreatedAt: c.CreatedAt,
		EditedAt:  c.EditedAt,
		ParentID:  c.ParentID,
	}, nil
}

// Comment cursor encodes the last item's created_at (keyset, oldest-first).
func encodeTimeCursor(t time.Time) string {
	return base64.RawURLEncoding.EncodeToString([]byte(t.Format(time.RFC3339Nano)))
}

func decodeTimeCursor(s string) *time.Time {
	if s == "" {
		return nil
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, string(b))
	if err != nil {
		return nil
	}
	return &t
}
