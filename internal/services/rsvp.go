package services

import (
	"context"
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

const goingFacesLimit = 8

type RSVPService struct {
	q   *models.Queries
	db  *pgxpool.Pool
	log *zap.Logger
}

func NewRSVPService() *RSVPService {
	return &RSVPService{q: models.New(db.Client), db: db.Client, log: zap.L().Named("RSVPService")}
}

func validJoinMode(m string) bool { return m == "anyone" || m == "friends" || m == "approval" }

func toInviteLinkResponse(l models.InviteLink) types.InviteLinkResponse {
	return types.InviteLinkResponse{
		Token: l.Token, Active: l.Active, JoinMode: l.JoinMode, Spots: l.Spots,
		ReplyBy: types.DateFromPtr(l.ReplyBy), Role: l.Role, Message: l.Message,
	}
}

// pastDate reports whether d (a reply_by date) is in the past — true once today
// is a later calendar day than d. nil = no deadline.
func pastDate(d *time.Time) bool {
	if d == nil {
		return false
	}
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return today.After(*d)
}

// plannedCostPerPerson is the per-person figure shown on the join page: the pool's
// per_person when the trip has a pool, else budget_amount ÷ max(1, going). nil when
// neither is set.
func plannedCostPerPerson(ctx context.Context, q *models.Queries, trip models.Trip, going int) *int64 {
	if p := poolPerPerson(ctx, q, trip.ID); p != nil {
		return p
	}
	if trip.BudgetAmount != nil {
		denom := int64(going)
		if denom < 1 {
			denom = 1
		}
		v := *trip.BudgetAmount / denom
		return &v
	}
	return nil
}

// firstName returns the leading word of a name, for friendly messages.
func firstName(name string) string {
	if i := strings.IndexByte(name, ' '); i > 0 {
		return name[:i]
	}
	return name
}

// ─── Organiser endpoints (owner only) ────────────────────────────────────────

func (s *RSVPService) GetLink(ctx context.Context, tripID, userID uuid.UUID) (*types.InviteLinkResponse, error) {
	if _, err := requireOwner(ctx, s.q, tripID, userID); err != nil {
		return nil, err
	}
	link, err := s.q.GetInviteLinkByTrip(ctx, tripID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperror.NotFound("this trip has no invite link yet")
		}
		return nil, apperror.FromDB(err)
	}
	r := toInviteLinkResponse(link)
	return &r, nil
}

func (s *RSVPService) UpsertLink(ctx context.Context, tripID, userID uuid.UUID, in types.InviteLinkInput) (*types.InviteLinkResponse, error) {
	if _, err := requireOwner(ctx, s.q, tripID, userID); err != nil {
		return nil, err
	}
	if !validJoinMode(in.JoinMode) {
		return nil, apperror.UnprocessableEntity("join_mode must be 'anyone', 'friends' or 'approval'")
	}
	if !validRole(in.Role) {
		return nil, apperror.UnprocessableEntity("role must be 'collaborator' or 'viewer'")
	}
	if in.Spots != nil && *in.Spots <= 0 {
		return nil, apperror.UnprocessableEntity("spots must be greater than 0")
	}
	link, err := s.q.UpsertInviteLink(ctx, models.UpsertInviteLinkParams{
		TripID: tripID, Token: uuid.NewString(), Active: in.Active, JoinMode: in.JoinMode,
		Spots: in.Spots, ReplyBy: types.DatePtr(in.ReplyBy), Role: in.Role, Message: in.Message,
	})
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	r := toInviteLinkResponse(link)
	return &r, nil
}

func (s *RSVPService) ResetLink(ctx context.Context, tripID, userID uuid.UUID) (*types.InviteLinkResponse, error) {
	if _, err := requireOwner(ctx, s.q, tripID, userID); err != nil {
		return nil, err
	}
	if _, err := s.q.GetInviteLinkByTrip(ctx, tripID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperror.NotFound("this trip has no invite link yet")
		}
		return nil, apperror.FromDB(err)
	}
	link, err := s.q.ResetInviteLinkToken(ctx, models.ResetInviteLinkTokenParams{Token: uuid.NewString(), TripID: tripID})
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	r := toInviteLinkResponse(link)
	return &r, nil
}

func (s *RSVPService) ListRSVPs(ctx context.Context, tripID, userID uuid.UUID) (*types.RSVPListResponse, error) {
	if _, err := requireOwner(ctx, s.q, tripID, userID); err != nil {
		return nil, err
	}
	rows, err := s.q.ListRSVPs(ctx, tripID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	items := make([]types.RSVPItem, len(rows))
	for i, r := range rows {
		items[i] = types.RSVPItem{ID: r.ID, UserID: r.UserID, Name: r.Name, PhotoURL: r.PhotoUrl, Status: r.Status, RepliedAt: r.RepliedAt}
	}
	return &types.RSVPListResponse{RSVPs: items}, nil
}

func (s *RSVPService) rsvpInTrip(ctx context.Context, rsvpID, tripID uuid.UUID) (models.Rsvp, error) {
	r, err := s.q.GetRSVP(ctx, rsvpID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return r, apperror.NotFound("reply not found")
		}
		return r, apperror.FromDB(err)
	}
	if r.TripID != tripID {
		return r, apperror.NotFound("reply not found")
	}
	return r, nil
}

// Approve turns a `requested` reply into going (or waitlist if full) or maybe,
// according to what the person chose.
func (s *RSVPService) Approve(ctx context.Context, tripID, rsvpID, userID uuid.UUID) error {
	if _, err := requireOwner(ctx, s.q, tripID, userID); err != nil {
		return err
	}
	r, err := s.rsvpInTrip(ctx, rsvpID, tripID)
	if err != nil {
		return err
	}
	if r.Status != "requested" {
		return apperror.Conflict("this reply is not awaiting approval")
	}
	link, err := s.q.GetInviteLinkByTrip(ctx, tripID)
	if err != nil {
		return apperror.FromDB(err)
	}

	status := "maybe"
	if r.Chosen == "going" {
		going, err := s.q.CountGoing(ctx, tripID)
		if err != nil {
			return apperror.FromDB(err)
		}
		if link.Spots != nil && int(going) >= int(*link.Spots) {
			status = "waitlist"
		} else {
			status = "going"
		}
	} else if r.Chosen == "declined" {
		status = "declined"
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return apperror.Internal("could not start transaction")
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)

	if err := qtx.SetRSVPStatus(ctx, models.SetRSVPStatusParams{Status: status, ID: r.ID}); err != nil {
		return apperror.FromDB(err)
	}
	if err := grantForStatus(ctx, qtx, tripID, r.UserID, status, link.Role); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return apperror.Internal("could not approve reply")
	}
	return nil
}

// DeleteRSVP removes a reply. For a Going person it also revokes participant
// access and promotes the earliest waitlisted person.
func (s *RSVPService) DeleteRSVP(ctx context.Context, tripID, rsvpID, userID uuid.UUID) error {
	if _, err := requireOwner(ctx, s.q, tripID, userID); err != nil {
		return err
	}
	r, err := s.rsvpInTrip(ctx, rsvpID, tripID)
	if err != nil {
		return err
	}
	link, hasLink := s.inviteLinkOrNil(ctx, tripID)

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return apperror.Internal("could not start transaction")
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)

	if err := qtx.DeleteRSVP(ctx, r.ID); err != nil {
		return apperror.FromDB(err)
	}
	if r.Status == "going" || r.Status == "maybe" {
		if err := qtx.RevokeParticipantByUser(ctx, models.RevokeParticipantByUserParams{TripID: tripID, UserID: &r.UserID}); err != nil {
			return apperror.FromDB(err)
		}
	}
	var promoted *uuid.UUID
	if r.Status == "going" {
		var lp *models.InviteLink
		if hasLink {
			lp = &link
		}
		promoted, err = s.tryPromote(ctx, qtx, tripID, lp)
		if err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return apperror.Internal("could not remove reply")
	}
	if promoted != nil {
		notify(ctx, s.q, notifyParams{UserID: *promoted, Type: "waitlist_promoted", TripID: &tripID})
	}
	return nil
}

func (s *RSVPService) inviteLinkOrNil(ctx context.Context, tripID uuid.UUID) (models.InviteLink, bool) {
	l, err := s.q.GetInviteLinkByTrip(ctx, tripID)
	if err != nil {
		return models.InviteLink{}, false
	}
	return l, true
}

// ─── Invitee endpoints (/join/:token) ────────────────────────────────────────

func (s *RSVPService) JoinInfo(ctx context.Context, token string, viewerID *uuid.UUID) (*types.JoinResponse, error) {
	link, err := s.q.GetInviteLinkByToken(ctx, token)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperror.NotFound("invite link not found")
		}
		return nil, apperror.FromDB(err)
	}
	if !link.Active {
		return nil, apperror.NotFound("invite link not found") // turned off / reset → gone
	}
	trip, err := s.q.GetTrip(ctx, link.TripID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	if viewerID != nil {
		if blocked, _ := isBlockedBetween(ctx, s.q, *viewerID, trip.OwnerID); blocked {
			return nil, apperror.NotFound("invite link not found")
		}
	}
	owner, err := s.q.GetUserByID(ctx, trip.OwnerID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	rawDests, err := s.q.ListDestinations(ctx, trip.ID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	destNames := make([]string, len(rawDests))
	for i, d := range rawDests {
		destNames[i] = d.Name
	}
	counts, err := s.q.RSVPCounts(ctx, trip.ID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	goingRows, err := s.q.GoingPeople(ctx, models.GoingPeopleParams{TripID: trip.ID, Lim: goingFacesLimit})
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	going := make([]types.JoinPerson, len(goingRows))
	for i, g := range goingRows {
		going[i] = types.JoinPerson{Name: g.Name, PhotoURL: g.PhotoUrl}
	}

	var myRSVP *string
	if viewerID != nil {
		if r, err := s.q.GetRSVPByTripUser(ctx, models.GetRSVPByTripUserParams{TripID: trip.ID, UserID: *viewerID}); err == nil {
			st := r.Status
			myRSVP = &st
		}
	}

	return &types.JoinResponse{
		Trip: types.JoinTripInfo{
			ID: trip.ID, Title: trip.Title, CoverImage: trip.CoverImage,
			StartDate: types.DateFromPtr(trip.StartDate), EndDate: types.DateFromPtr(trip.EndDate),
			StartLocation: trip.StartLocation, TripType: trip.TripType, Currency: trip.Currency,
		},
		Destinations:         destNames,
		Organiser:            types.JoinPerson{Name: owner.Name, PhotoURL: owner.PhotoUrl},
		Message:              link.Message,
		JoinMode:             link.JoinMode,
		Spots:                link.Spots,
		ReplyBy:              types.DateFromPtr(link.ReplyBy),
		Closed:               pastDate(link.ReplyBy),
		Counts:               types.JoinCounts{Going: int(counts.Going), Maybe: int(counts.Maybe)},
		GoingPeople:          going,
		PlannedCostPerPerson: plannedCostPerPerson(ctx, s.q, trip, int(counts.Going)),
		MyRSVP:               myRSVP,
	}, nil
}

// Reply records (or changes) the signed-in user's reply. The returned status is
// what actually happened — which may be waitlist or requested.
func (s *RSVPService) Reply(ctx context.Context, token string, userID uuid.UUID, chosen string) (*types.RSVPReplyResponse, error) {
	if chosen != "going" && chosen != "maybe" && chosen != "declined" {
		return nil, apperror.UnprocessableEntity("status must be 'going', 'maybe' or 'declined'")
	}
	link, err := s.q.GetInviteLinkByToken(ctx, token)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperror.NotFound("invite link not found")
		}
		return nil, apperror.FromDB(err)
	}
	if !link.Active {
		return nil, apperror.NotFound("invite link not found")
	}
	trip, err := s.q.GetTrip(ctx, link.TripID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	if blocked, _ := isBlockedBetween(ctx, s.q, userID, trip.OwnerID); blocked {
		return nil, apperror.NotFound("invite link not found")
	}
	if pastDate(link.ReplyBy) {
		return nil, apperror.Forbidden("This invite link is closed.")
	}
	if link.JoinMode == "friends" && userID != trip.OwnerID {
		fr, _ := areFriends(ctx, s.q, userID, trip.OwnerID)
		if !fr {
			owner, _ := s.q.GetUserByID(ctx, trip.OwnerID)
			return nil, apperror.Forbidden("This trip is open to " + firstName(owner.Name) + "'s friends only.")
		}
	}

	prev, hadPrev := s.prevRSVP(ctx, trip.ID, userID)
	wasGoing := hadPrev && prev.Status == "going"
	hadAccess := hadPrev && (prev.Status == "going" || prev.Status == "maybe")
	approval := link.JoinMode == "approval"

	status := "declined"
	switch chosen {
	case "maybe":
		if approval {
			status = "requested"
		} else {
			status = "maybe"
		}
	case "going":
		if approval {
			status = "requested"
		} else {
			going, err := s.q.CountGoing(ctx, trip.ID)
			if err != nil {
				return nil, apperror.FromDB(err)
			}
			others := int(going)
			if wasGoing {
				others--
			}
			if link.Spots != nil && others >= int(*link.Spots) {
				status = "waitlist"
			} else {
				status = "going"
			}
		}
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, apperror.Internal("could not start transaction")
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)

	if _, err := qtx.UpsertRSVP(ctx, models.UpsertRSVPParams{TripID: trip.ID, UserID: userID, Status: status, Chosen: chosen}); err != nil {
		return nil, apperror.FromDB(err)
	}
	if status == "going" || status == "maybe" {
		if err := grantForStatus(ctx, qtx, trip.ID, userID, status, link.Role); err != nil {
			return nil, err
		}
	} else if hadAccess {
		if err := qtx.RevokeParticipantByUser(ctx, models.RevokeParticipantByUserParams{TripID: trip.ID, UserID: &userID}); err != nil {
			return nil, apperror.FromDB(err)
		}
	}
	var promoted *uuid.UUID
	if wasGoing && status != "going" {
		lp := link
		promoted, err = s.tryPromote(ctx, qtx, trip.ID, &lp)
		if err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, apperror.Internal("could not save reply")
	}

	// Notifications (best-effort, after commit).
	rs := status
	notify(ctx, s.q, notifyParams{UserID: trip.OwnerID, Type: "rsvp", ActorID: &userID, TripID: &trip.ID, RSVPStatus: &rs})
	if status == "requested" {
		notify(ctx, s.q, notifyParams{UserID: trip.OwnerID, Type: "rsvp_request", ActorID: &userID, TripID: &trip.ID})
	}
	if promoted != nil {
		notify(ctx, s.q, notifyParams{UserID: *promoted, Type: "waitlist_promoted", TripID: &trip.ID})
	}
	return &types.RSVPReplyResponse{Status: status, TripID: trip.ID}, nil
}

func (s *RSVPService) prevRSVP(ctx context.Context, tripID, userID uuid.UUID) (models.Rsvp, bool) {
	r, err := s.q.GetRSVPByTripUser(ctx, models.GetRSVPByTripUserParams{TripID: tripID, UserID: userID})
	if err != nil {
		return r, false
	}
	return r, true
}

// grantForStatus grants trip access for a Going (link role) or Maybe (viewer, no
// spot) reply. Other statuses grant nothing.
func grantForStatus(ctx context.Context, qtx *models.Queries, tripID, userID uuid.UUID, status, linkRole string) error {
	role := ""
	switch status {
	case "going":
		role = linkRole
	case "maybe":
		role = RoleViewer
	default:
		return nil
	}
	return apperror.FromDBOrNil(qtx.UpsertRSVPParticipant(ctx, models.UpsertRSVPParticipantParams{
		TripID: tripID, UserID: &userID, Role: role,
	}))
}

// tryPromote moves the earliest waitlisted person to Going and grants access.
// Returns their id (to notify) or nil when the waitlist is empty.
func (s *RSVPService) tryPromote(ctx context.Context, qtx *models.Queries, tripID uuid.UUID, link *models.InviteLink) (*uuid.UUID, error) {
	w, err := qtx.EarliestWaitlisted(ctx, tripID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, apperror.FromDB(err)
	}
	if err := qtx.SetRSVPStatus(ctx, models.SetRSVPStatusParams{Status: "going", ID: w.ID}); err != nil {
		return nil, apperror.FromDB(err)
	}
	role := RoleViewer
	if link != nil {
		role = link.Role
	}
	if err := qtx.UpsertRSVPParticipant(ctx, models.UpsertRSVPParticipantParams{TripID: tripID, UserID: &w.UserID, Role: role}); err != nil {
		return nil, apperror.FromDB(err)
	}
	id := w.UserID
	return &id, nil
}
