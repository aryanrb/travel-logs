package services

import (
	"context"

	"github.com/google/uuid"

	"travel-service/internal/models"
	"travel-service/internal/types"
)

// partyResolver turns (user_id | external_id) references into named parties for
// expense, settlement, and balance responses. Names are cached so repeated
// references within one request don't re-query. Trip-scoped data is small.
type partyResolver struct {
	ctx   context.Context
	q     *models.Queries
	users map[uuid.UUID]string
	ext   map[uuid.UUID]string
}

func newPartyResolver(ctx context.Context, q *models.Queries) *partyResolver {
	return &partyResolver{ctx: ctx, q: q, users: map[uuid.UUID]string{}, ext: map[uuid.UUID]string{}}
}

func (r *partyResolver) userName(id uuid.UUID) string {
	if n, ok := r.users[id]; ok {
		return n
	}
	n := "unknown user"
	if u, err := r.q.GetUserByID(r.ctx, id); err == nil {
		n = u.Name
	}
	r.users[id] = n
	return n
}

func (r *partyResolver) extName(id uuid.UUID) string {
	if n, ok := r.ext[id]; ok {
		return n
	}
	n := "unknown payer"
	if e, err := r.q.GetExternalPayer(r.ctx, id); err == nil {
		n = e.Name
	}
	r.ext[id] = n
	return n
}

// party resolves exactly one of the two refs into a named Party.
func (r *partyResolver) party(userID, externalID *uuid.UUID) types.Party {
	switch {
	case userID != nil:
		return types.Party{Type: "user", ID: *userID, Name: r.userName(*userID)}
	case externalID != nil:
		return types.Party{Type: "external", ID: *externalID, Name: r.extName(*externalID)}
	default:
		return types.Party{}
	}
}
