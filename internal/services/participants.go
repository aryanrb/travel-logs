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

type ParticipantService struct {
	q   *models.Queries
	db  *pgxpool.Pool
	log *zap.Logger
}

func NewParticipantService() *ParticipantService {
	return &ParticipantService{q: models.New(db.Client), db: db.Client, log: zap.L().Named("ParticipantService")}
}

func validRole(r string) bool { return r == RoleCollaborator || r == RoleViewer }

// participantTitle names a member for the activity feed: their user name, else
// the pending invite email.
func participantTitle(ctx context.Context, q *models.Queries, p models.TripParticipant) string {
	if p.UserID != nil {
		if u, err := q.GetUserByID(ctx, *p.UserID); err == nil {
			return u.Name
		}
	}
	if p.InvitedEmail != nil {
		return *p.InvitedEmail
	}
	return "member"
}

// Invite adds a pending collaborator/viewer, by existing user or by email.
// Only the owner may invite (req 3.5). Access is granted only once accepted.
func (s *ParticipantService) Invite(ctx context.Context, tripID, userID uuid.UUID, in types.InviteInput) (*models.TripParticipant, error) {
	trip, err := requireOwner(ctx, s.q, tripID, userID)
	if err != nil {
		return nil, err
	}
	if !validRole(in.Role) {
		return nil, apperror.UnprocessableEntity("role must be 'collaborator' or 'viewer'")
	}
	byUser := in.UserID != nil
	byEmail := in.InvitedEmail != nil && strings.TrimSpace(*in.InvitedEmail) != ""
	if byUser == byEmail {
		return nil, apperror.UnprocessableEntity("provide exactly one of user_id or invited_email")
	}
	if byUser && *in.UserID == trip.OwnerID {
		return nil, apperror.BadRequest("the owner already has access")
	}

	token := uuid.NewString()
	params := models.InsertParticipantParams{
		TripID:      tripID,
		Role:        in.Role,
		Status:      "pending",
		InviteToken: &token,
	}
	if byUser {
		params.UserID = in.UserID
	} else {
		email := strings.ToLower(strings.TrimSpace(*in.InvitedEmail))
		params.InvitedEmail = &email
	}
	var created models.TripParticipant
	err = inTx(ctx, s.db, s.q, userID, func(qtx *models.Queries) (*activity, error) {
		p, err := qtx.InsertParticipant(ctx, params)
		if err != nil {
			return nil, apperror.FromDB(err) // dup (trip,user) -> Conflict
		}
		created = p
		return &activity{TripID: tripID, Action: "created", SubjectType: "participant", SubjectID: p.UserID,
			Data: eventData{Title: participantTitle(ctx, qtx, p)}}, nil
	})
	if err != nil {
		return nil, err
	}
	return &created, nil
}

func (s *ParticipantService) List(ctx context.Context, tripID, userID uuid.UUID) ([]models.ListParticipantsRow, error) {
	if _, _, err := requireViewer(ctx, s.q, tripID, userID); err != nil {
		return nil, err
	}
	rows, err := s.q.ListParticipants(ctx, tripID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	return rows, nil
}

// Accept consumes an invite token and grants the current user access. An invite
// addressed to a specific registered user can only be accepted by that user.
func (s *ParticipantService) Accept(ctx context.Context, userID uuid.UUID, token string) (*models.TripParticipant, error) {
	if strings.TrimSpace(token) == "" {
		return nil, apperror.BadRequest("invite token is required")
	}
	tkn := token
	existing, err := s.q.GetParticipantByToken(ctx, &tkn)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperror.NotFound("invitation not found")
		}
		return nil, apperror.FromDB(err)
	}
	if existing.Status != "pending" {
		return nil, apperror.Conflict("invitation is no longer pending")
	}
	if existing.UserID != nil && *existing.UserID != userID {
		return nil, apperror.Forbidden("this invitation was addressed to a different user")
	}
	uid := userID
	var accepted models.TripParticipant
	err = inTx(ctx, s.db, s.q, userID, func(qtx *models.Queries) (*activity, error) {
		p, err := qtx.AcceptInvite(ctx, models.AcceptInviteParams{UserID: &uid, InviteToken: &tkn})
		if err != nil {
			return nil, apperror.FromDB(err)
		}
		accepted = p
		return &activity{TripID: p.TripID, Action: "updated", SubjectType: "participant", SubjectID: &userID,
			Data: eventData{Title: participantTitle(ctx, qtx, p)}}, nil
	})
	if err != nil {
		return nil, err
	}
	return &accepted, nil
}

func (s *ParticipantService) ChangeRole(ctx context.Context, tripID, participantID, userID uuid.UUID, role string) error {
	if _, err := requireOwner(ctx, s.q, tripID, userID); err != nil {
		return err
	}
	if !validRole(role) {
		return apperror.UnprocessableEntity("role must be 'collaborator' or 'viewer'")
	}
	p, err := s.participantInTrip(ctx, participantID, tripID)
	if err != nil {
		return err
	}
	return inTx(ctx, s.db, s.q, userID, func(qtx *models.Queries) (*activity, error) {
		if err := qtx.UpdateParticipantRole(ctx, models.UpdateParticipantRoleParams{
			Role: role, ID: participantID, TripID: tripID,
		}); err != nil {
			return nil, apperror.FromDB(err)
		}
		return &activity{TripID: tripID, Action: "updated", SubjectType: "participant", SubjectID: p.UserID,
			Data: eventData{Title: participantTitle(ctx, qtx, p)}}, nil
	})
}

// participantInTrip loads a participant and confirms it belongs to the trip.
func (s *ParticipantService) participantInTrip(ctx context.Context, participantID, tripID uuid.UUID) (models.TripParticipant, error) {
	p, err := s.q.GetParticipant(ctx, participantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return p, apperror.NotFound("participant not found")
		}
		return p, apperror.FromDB(err)
	}
	if p.TripID != tripID {
		return p, apperror.NotFound("participant not found")
	}
	return p, nil
}

// Remove revokes a participant's access while keeping their historical
// attribution and financial records intact (rule 9).
func (s *ParticipantService) Remove(ctx context.Context, tripID, participantID, userID uuid.UUID) error {
	if _, err := requireOwner(ctx, s.q, tripID, userID); err != nil {
		return err
	}
	p, err := s.participantInTrip(ctx, participantID, tripID)
	if err != nil {
		return err
	}
	return inTx(ctx, s.db, s.q, userID, func(qtx *models.Queries) (*activity, error) {
		if err := qtx.SetParticipantStatus(ctx, models.SetParticipantStatusParams{
			Status: "revoked", ID: participantID, TripID: tripID,
		}); err != nil {
			return nil, apperror.FromDB(err)
		}
		return &activity{TripID: tripID, Action: "deleted", SubjectType: "participant", SubjectID: p.UserID,
			Data: eventData{Title: participantTitle(ctx, qtx, p)}}, nil
	})
}
