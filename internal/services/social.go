package services

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"travel-service/internal/models"
)

// Relationship predicates shared across stages (RSVP, notifications, explore,
// comments), backed by the stage-4 follows/friendships/blocks tables.

// isBlockedBetween reports whether a and b block each other in either direction.
func isBlockedBetween(ctx context.Context, q *models.Queries, a, b uuid.UUID) (bool, error) {
	return q.IsBlockedEitherWay(ctx, models.IsBlockedEitherWayParams{A: a, B: b})
}

// areFriends reports whether a and b are accepted friends.
func areFriends(ctx context.Context, q *models.Queries, a, b uuid.UUID) (bool, error) {
	pair, err := q.GetFriendshipPair(ctx, models.GetFriendshipPairParams{A: a, B: b})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return pair.Status == "accepted", nil
}

// followedOwnerIDs returns the ids the user follows (for explore's `following`).
func followedOwnerIDs(ctx context.Context, q *models.Queries, userID uuid.UUID) ([]uuid.UUID, error) {
	return q.FollowedIDs(ctx, userID)
}

// blockedSet returns the ids blocked (either direction) by userID, for hiding
// content in explore/feeds.
func blockedSet(ctx context.Context, q *models.Queries, userID uuid.UUID) (map[uuid.UUID]bool, error) {
	ids, err := q.BlockedUserIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	m := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m, nil
}
