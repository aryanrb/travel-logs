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
	exp *ExpenseService // for nested expenses created alongside an entry
	log *zap.Logger
}

func NewTimelineService() *TimelineService {
	return &TimelineService{q: models.New(db.Client), db: db.Client, exp: NewExpenseService(), log: zap.L().Named("TimelineService")}
}

// createNestedExpense creates the expense supplied inline on an entry, forcing
// its link to that entry, using the entry's own transaction so both commit or
// roll back together.
func (s *TimelineService) createNestedExpense(ctx context.Context, qtx *models.Queries, tripID, userID uuid.UUID, trip models.Trip, entryID uuid.UUID, in types.ExpenseInput) error {
	in.EntryID = &entryID
	if err := validateExpenseInput(in); err != nil {
		return err
	}
	_, err := s.exp.insertWithinTx(ctx, qtx, tripID, userID, in, resolveCurrency(trip, in.Currency))
	return err
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
	_, trip, err := requireContributor(ctx, s.q, tripID, userID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Title) == "" {
		return nil, apperror.UnprocessableEntity("entry title is required")
	}
	if in.EntryDate.Time().IsZero() {
		return nil, apperror.UnprocessableEntity("entry_date is required (YYYY-MM-DD)")
	}

	var created models.TimelineEntry
	err = inTx(ctx, s.db, s.q, userID, func(qtx *models.Queries) (*activity, error) {
		e, err := qtx.InsertEntry(ctx, models.InsertEntryParams{
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
		created = e
		if in.Expense != nil {
			if err := s.createNestedExpense(ctx, qtx, tripID, userID, trip, e.ID, *in.Expense); err != nil {
				return nil, err
			}
		}
		return &activity{TripID: tripID, Action: "created", SubjectType: "entry", SubjectID: &e.ID,
			Data: eventData{Title: e.Title}}, nil
	})
	if err != nil {
		return nil, err
	}
	return &created, nil
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
	role, trip, err := requireContributor(ctx, s.q, tripID, userID)
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
	params := models.UpdateEntryParams{
		Title:          in.Title,
		Description:    in.Description,
		EntryDate:      in.EntryDate.Time(),
		EntryTime:      in.EntryTime,
		Location:       in.Location,
		LastModifiedBy: &uid,
		ID:             entryID,
	}
	return inTx(ctx, s.db, s.q, userID, func(qtx *models.Queries) (*activity, error) {
		if err := qtx.UpdateEntry(ctx, params); err != nil {
			return nil, apperror.FromDB(err)
		}
		if in.Expense != nil {
			if err := s.createNestedExpense(ctx, qtx, tripID, userID, trip, entryID, *in.Expense); err != nil {
				return nil, err
			}
		}
		return &activity{TripID: tripID, Action: "updated", SubjectType: "entry", SubjectID: &entryID,
			Data: eventData{Title: in.Title}}, nil
	})
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
	// photos (no entry_id) are unaffected (req 3.9). Expenses keep their row with
	// entry_id set to null (ON DELETE SET NULL).
	return inTx(ctx, s.db, s.q, userID, func(qtx *models.Queries) (*activity, error) {
		if err := qtx.DeleteEntry(ctx, entryID); err != nil {
			return nil, apperror.FromDB(err)
		}
		return &activity{TripID: tripID, Action: "deleted", SubjectType: "entry", SubjectID: &e.ID,
			Data: eventData{Title: e.Title}}, nil
	})
}

// photoTitle labels a photo for the activity feed — its caption, or "photo".
func photoTitle(p models.Photo) string {
	if p.Caption != nil && strings.TrimSpace(*p.Caption) != "" {
		return *p.Caption
	}
	return "photo"
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
	var created models.Photo
	err := inTx(ctx, s.db, s.q, userID, func(qtx *models.Queries) (*activity, error) {
		p, err := qtx.InsertPhoto(ctx, models.InsertPhotoParams{
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
		created = p
		return &activity{TripID: tripID, Action: "created", SubjectType: "photo", SubjectID: &p.ID,
			Data: eventData{Title: photoTitle(p)}}, nil
	})
	if err != nil {
		return nil, err
	}
	return &created, nil
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
	return inTx(ctx, s.db, s.q, userID, func(qtx *models.Queries) (*activity, error) {
		if err := qtx.DeletePhoto(ctx, photoID); err != nil {
			return nil, apperror.FromDB(err)
		}
		return &activity{TripID: tripID, Action: "deleted", SubjectType: "photo", SubjectID: &p.ID,
			Data: eventData{Title: photoTitle(p)}}, nil
	})
}
