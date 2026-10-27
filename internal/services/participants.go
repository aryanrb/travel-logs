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
	p, err := s.q.InsertParticipant(ctx, params)
	if err != nil {
		return nil, apperror.FromDB(err) // dup (trip,user) -> Conflict
	}
	return &p, nil
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
	p, err := s.q.AcceptInvite(ctx, models.AcceptInviteParams{UserID: &uid, InviteToken: &tkn})
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	return &p, nil
}

func (s *ParticipantService) ChangeRole(ctx context.Context, tripID, participantID, userID uuid.UUID, role string) error {
	if _, err := requireOwner(ctx, s.q, tripID, userID); err != nil {
		return err
	}
	if !validRole(role) {
		return apperror.UnprocessableEntity("role must be 'collaborator' or 'viewer'")
	}
	return apperror.FromDBOrNil(s.q.UpdateParticipantRole(ctx, models.UpdateParticipantRoleParams{
		Role: role, ID: participantID, TripID: tripID,
	}))
}

// Remove revokes a participant's access while keeping their historical
// attribution and financial records intact (rule 9).
func (s *ParticipantService) Remove(ctx context.Context, tripID, participantID, userID uuid.UUID) error {
	if _, err := requireOwner(ctx, s.q, tripID, userID); err != nil {
		return err
	}
	return apperror.FromDBOrNil(s.q.SetParticipantStatus(ctx, models.SetParticipantStatusParams{
		Status: "revoked", ID: participantID, TripID: tripID,
	}))
}
