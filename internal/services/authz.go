package services

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"travel-service/internal/models"
	"travel-service/internal/utils/apperror"
)

// Trip roles. Owner is stored on the trip; collaborator/viewer on participants.
const (
	RoleOwner        = "owner"
	RoleCollaborator = "collaborator"
	RoleViewer       = "viewer"
)

// tripRole resolves the caller's effective role on a trip, enforced on the
// backend for every trip-scoped operation (core rule 7). Returns Forbidden when
// the caller is neither the owner nor an accepted participant — so a removed or
// pending participant has no access.
func tripRole(ctx context.Context, q *models.Queries, tripID, userID uuid.UUID) (string, models.Trip, error) {
	trip, err := q.GetTrip(ctx, tripID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", trip, apperror.NotFound("trip not found")
		}
		return "", trip, apperror.FromDB(err)
	}
	if trip.OwnerID == userID {
		return RoleOwner, trip, nil
	}
	uid := userID
	p, err := q.GetParticipantByTripUser(ctx, models.GetParticipantByTripUserParams{TripID: tripID, UserID: &uid})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", trip, apperror.Forbidden("you do not have access to this trip")
		}
		return "", trip, apperror.FromDB(err)
	}
	if p.Status != "accepted" {
		return "", trip, apperror.Forbidden("you do not have access to this trip")
	}
	return p.Role, trip, nil
}

// requireViewer allows any accepted role (owner, collaborator, viewer).
func requireViewer(ctx context.Context, q *models.Queries, tripID, userID uuid.UUID) (string, models.Trip, error) {
	return tripRole(ctx, q, tripID, userID)
}

// requireContributor allows owner or collaborator (can add content/expenses).
func requireContributor(ctx context.Context, q *models.Queries, tripID, userID uuid.UUID) (string, models.Trip, error) {
	role, trip, err := tripRole(ctx, q, tripID, userID)
	if err != nil {
		return role, trip, err
	}
	if role == RoleViewer {
		return role, trip, apperror.Forbidden("viewers cannot modify this trip")
	}
	return role, trip, nil
}

// requireOwner allows only the trip owner (settings, participants, delete).
func requireOwner(ctx context.Context, q *models.Queries, tripID, userID uuid.UUID) (models.Trip, error) {
	role, trip, err := tripRole(ctx, q, tripID, userID)
	if err != nil {
		return trip, err
	}
	if role != RoleOwner {
		return trip, apperror.Forbidden("only the trip owner can do this")
	}
	return trip, nil
}

// canModify reports whether a caller may edit/delete a specific piece of
// content. The owner manages all content; a collaborator only what they created
// (req 2.2). Removing the creator later does not rewrite the record (rule 9).
func canModify(role string, creatorID, userID uuid.UUID) bool {
	return role == RoleOwner || (role == RoleCollaborator && creatorID == userID)
}
