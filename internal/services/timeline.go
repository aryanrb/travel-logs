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

type TimelineService struct {
	q   *models.Queries
	db  *pgxpool.Pool
	log *zap.Logger
}

func NewTimelineService() *TimelineService {
	return &TimelineService{q: models.New(db.Client), db: db.Client, log: zap.L().Named("TimelineService")}
}

func (s *TimelineService) entryInTrip(ctx context.Context, entryID, tripID uuid.UUID) (models.TimelineEntry, error) {
	e, err := s.q.GetEntry(ctx, entryID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return e, apperror.NotFound("timeline entry not found")
		}
		return e, apperror.FromDB(err)
	}
	if e.TripID != tripID {
		return e, apperror.NotFound("timeline entry not found")
	}
	return e, nil
}

func (s *TimelineService) CreateEntry(ctx context.Context, tripID, userID uuid.UUID, in types.EntryInput) (*models.TimelineEntry, error) {
	if _, _, err := requireContributor(ctx, s.q, tripID, userID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Title) == "" {
		return nil, apperror.UnprocessableEntity("entry title is required")
	}
	if in.EntryDate.Time().IsZero() {
		return nil, apperror.UnprocessableEntity("entry_date is required (YYYY-MM-DD)")
	}
	e, err := s.q.InsertEntry(ctx, models.InsertEntryParams{
		TripID:      tripID,
		Title:       in.Title,
		Description: in.Description,
		EntryDate:   in.EntryDate.Time(),
		EntryTime:   in.EntryTime,
		Location:    in.Location,
		CreatedBy:   userID,
	})
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	return &e, nil
}

func (s *TimelineService) ListEntries(ctx context.Context, tripID, userID uuid.UUID) ([]models.TimelineEntry, error) {
	if _, _, err := requireViewer(ctx, s.q, tripID, userID); err != nil {
		return nil, err
	}
	rows, err := s.q.ListEntries(ctx, tripID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	return rows, nil
}

func (s *TimelineService) UpdateEntry(ctx context.Context, tripID, entryID, userID uuid.UUID, in types.EntryInput) error {
	role, _, err := requireContributor(ctx, s.q, tripID, userID)
	if err != nil {
		return err
	}
	e, err := s.entryInTrip(ctx, entryID, tripID)
	if err != nil {
		return err
	}
	if !canModify(role, e.CreatedBy, userID) {
		return apperror.Forbidden("you can only edit entries you created")
	}
	if strings.TrimSpace(in.Title) == "" {
		return apperror.UnprocessableEntity("entry title is required")
	}
	if in.EntryDate.Time().IsZero() {
		return apperror.UnprocessableEntity("entry_date is required (YYYY-MM-DD)")
	}
	uid := userID
	return apperror.FromDBOrNil(s.q.UpdateEntry(ctx, models.UpdateEntryParams{
		Title:          in.Title,
		Description:    in.Description,
		EntryDate:      in.EntryDate.Time(),
		EntryTime:      in.EntryTime,
		Location:       in.Location,
		LastModifiedBy: &uid,
		ID:             entryID,
	}))
}

func (s *TimelineService) DeleteEntry(ctx context.Context, tripID, entryID, userID uuid.UUID) error {
	role, _, err := requireContributor(ctx, s.q, tripID, userID)
	if err != nil {
		return err
	}
	e, err := s.entryInTrip(ctx, entryID, tripID)
	if err != nil {
		return err
	}
	if !canModify(role, e.CreatedBy, userID) {
		return apperror.Forbidden("you can only delete entries you created")
	}
	// Photos attached to this entry cascade-delete (migration FK). Trip-level
	// photos (no entry_id) are unaffected (req 3.9).
	return apperror.FromDBOrNil(s.q.DeleteEntry(ctx, entryID))
}

// ─── Photos ───────────────────────────────────────────────────────────────

func (s *TimelineService) AddPhoto(ctx context.Context, tripID, userID uuid.UUID, in types.PhotoInput) (*models.Photo, error) {
	if _, _, err := requireContributor(ctx, s.q, tripID, userID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.URL) == "" {
		return nil, apperror.UnprocessableEntity("photo url is required")
	}
	if in.EntryID != nil {
		if _, err := s.entryInTrip(ctx, *in.EntryID, tripID); err != nil {
			return nil, err
		}
	}
	p, err := s.q.InsertPhoto(ctx, models.InsertPhotoParams{
		TripID:    tripID,
		EntryID:   in.EntryID,
		Url:       in.URL,
		Caption:   in.Caption,
		TakenDate: types.DatePtr(in.TakenDate),
		CreatedBy: userID,
	})
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	return &p, nil
}

func (s *TimelineService) ListPhotos(ctx context.Context, tripID, userID uuid.UUID) ([]models.Photo, error) {
	if _, _, err := requireViewer(ctx, s.q, tripID, userID); err != nil {
		return nil, err
	}
	rows, err := s.q.ListTripPhotos(ctx, tripID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	return rows, nil
}

func (s *TimelineService) DeletePhoto(ctx context.Context, tripID, photoID, userID uuid.UUID) error {
	role, _, err := requireContributor(ctx, s.q, tripID, userID)
	if err != nil {
		return err
	}
	p, err := s.q.GetPhoto(ctx, photoID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperror.NotFound("photo not found")
		}
		return apperror.FromDB(err)
	}
	if p.TripID != tripID {
		return apperror.NotFound("photo not found")
	}
	if !canModify(role, p.CreatedBy, userID) {
		return apperror.Forbidden("you can only delete photos you added")
	}
	return apperror.FromDBOrNil(s.q.DeletePhoto(ctx, photoID))
}
