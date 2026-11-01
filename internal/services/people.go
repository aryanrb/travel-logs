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

type SocialService struct {
	q   *models.Queries
	db  *pgxpool.Pool
	log *zap.Logger
}

func NewSocialService() *SocialService {
	return &SocialService{q: models.New(db.Client), db: db.Client, log: zap.L().Named("SocialService")}
}

func brief(id uuid.UUID, name string, photo *string) types.PersonBrief {
	return types.PersonBrief{ID: id, Name: name, PhotoURL: photo}
}

// ─── Profile ──────────────────────────────────────────────────────────────────

func (s *SocialService) Profile(ctx context.Context, targetID uuid.UUID, viewerID *uuid.UUID) (*types.ProfileResponse, error) {
	target, err := s.q.GetUserByID(ctx, targetID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperror.NotFound("profile not found")
		}
		return nil, apperror.FromDB(err)
	}

	self := viewerID != nil && *viewerID == targetID
	var viewerBlockedTarget bool
	if viewerID != nil && !self {
		if targetBlockedViewer, _ := s.q.DidBlock(ctx, models.DidBlockParams{BlockerID: targetID, BlockedID: *viewerID}); targetBlockedViewer {
			return nil, apperror.NotFound("profile not found") // they blocked you → as if gone
		}
		viewerBlockedTarget, _ = s.q.DidBlock(ctx, models.DidBlockParams{BlockerID: *viewerID, BlockedID: targetID})
	}

	tripsCount, _ := s.q.CountPublicTripsByOwner(ctx, targetID)
	followers, _ := s.q.CountFollowers(ctx, targetID)
	following, _ := s.q.CountFollowing(ctx, targetID)
	friends, _ := s.q.CountFriends(ctx, targetID)

	trips := []types.PublicTripListItem{}
	if !viewerBlockedTarget { // you blocked them → hide their trips (offer Unblock)
		rows, err := s.q.ListPublicTripsByOwner(ctx, targetID)
		if err != nil {
			return nil, apperror.FromDB(err)
		}
		trips = make([]types.PublicTripListItem, len(rows))
		for i, r := range rows {
			trips[i] = types.PublicTripListItem{
				Trip:          toPublicTripSafe(r.Trip),
				Destinations:  r.DestinationNames,
				OwnerName:     r.OwnerName,
				CostPerPerson: costPerPerson(r.Trip.ShowCostsPublic, r.TotalSpent, r.Travellers),
				Likes:         int64(r.LikesCount),
				Saves:         int64(r.SavesCount),
			}
		}
		if err := decorateViewerFlags(ctx, s.q, trips, viewerID); err != nil {
			return nil, err
		}
	}

	var rel *types.Relationship
	if viewerID != nil && !self {
		v := *viewerID
		isFollowing, _ := s.q.IsFollowing(ctx, models.IsFollowingParams{FollowerID: v, FolloweeID: targetID})
		followsYou, _ := s.q.IsFollowing(ctx, models.IsFollowingParams{FollowerID: targetID, FolloweeID: v})
		rel = &types.Relationship{
			Following:  isFollowing,
			FollowsYou: followsYou,
			Friend:     s.friendStatus(ctx, v, targetID),
			Blocked:    viewerBlockedTarget,
		}
	}

	return &types.ProfileResponse{
		User: types.ProfileUser{
			ID: target.ID, Name: target.Name, PhotoURL: target.PhotoUrl, Bio: target.Bio, CreatedAt: target.CreatedAt,
		},
		Stats:        types.ProfileStats{Trips: int(tripsCount), Followers: int(followers), Following: int(following), Friends: int(friends)},
		Trips:        trips,
		Relationship: rel,
	}, nil
}

func (s *SocialService) friendStatus(ctx context.Context, viewer, target uuid.UUID) string {
	pair, err := s.q.GetFriendshipPair(ctx, models.GetFriendshipPairParams{A: viewer, B: target})
	if err != nil {
		return "none"
	}
	if pair.Status == "accepted" {
		return "friends"
	}
	if pair.RequesterID == viewer {
		return "requested" // you asked them
	}
	return "incoming" // they asked you
}

// ─── Follows ──────────────────────────────────────────────────────────────────

func (s *SocialService) Follow(ctx context.Context, viewerID, targetID uuid.UUID) error {
	if viewerID == targetID {
		return apperror.BadRequest("you cannot follow yourself")
	}
	if err := s.ensureUser(ctx, targetID); err != nil {
		return err
	}
	if blocked, _ := isBlockedBetween(ctx, s.q, viewerID, targetID); blocked {
		return apperror.NotFound("user not found")
	}
	already, _ := s.q.IsFollowing(ctx, models.IsFollowingParams{FollowerID: viewerID, FolloweeID: targetID})
	if err := s.q.Follow(ctx, models.FollowParams{FollowerID: viewerID, FolloweeID: targetID}); err != nil {
		return apperror.FromDB(err)
	}
	if !already {
		notify(ctx, s.q, notifyParams{UserID: targetID, Type: "follow", ActorID: &viewerID})
	}
	return nil
}

func (s *SocialService) Unfollow(ctx context.Context, viewerID, targetID uuid.UUID) error {
	return apperror.FromDBOrNil(s.q.Unfollow(ctx, models.UnfollowParams{FollowerID: viewerID, FolloweeID: targetID}))
}

// ─── Friends ──────────────────────────────────────────────────────────────────

// Friend sends a request, or accepts the target's pending request if one exists.
func (s *SocialService) Friend(ctx context.Context, viewerID, targetID uuid.UUID) error {
	if viewerID == targetID {
		return apperror.BadRequest("you cannot friend yourself")
	}
	if err := s.ensureUser(ctx, targetID); err != nil {
		return err
	}
	if blocked, _ := isBlockedBetween(ctx, s.q, viewerID, targetID); blocked {
		return apperror.NotFound("user not found")
	}
	pair, err := s.q.GetFriendshipPair(ctx, models.GetFriendshipPairParams{A: viewerID, B: targetID})
	if err == nil {
		if pair.Status == "accepted" {
			return nil // already friends
		}
		if pair.RequesterID == targetID { // they asked you → accept
			if err := s.q.AcceptFriendRequest(ctx, models.AcceptFriendRequestParams{RequesterID: targetID, AddresseeID: viewerID}); err != nil {
				return apperror.FromDB(err)
			}
			notify(ctx, s.q, notifyParams{UserID: targetID, Type: "friend_accepted", ActorID: &viewerID})
			return nil
		}
		return nil // you already requested them
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return apperror.FromDB(err)
	}
	if err := s.q.InsertFriendRequest(ctx, models.InsertFriendRequestParams{RequesterID: viewerID, AddresseeID: targetID}); err != nil {
		return apperror.FromDB(err)
	}
	notify(ctx, s.q, notifyParams{UserID: targetID, Type: "friend_request", ActorID: &viewerID})
	return nil
}

// Unfriend cancels your request, declines theirs, or ends the friendship.
func (s *SocialService) Unfriend(ctx context.Context, viewerID, targetID uuid.UUID) error {
	return apperror.FromDBOrNil(s.q.DeleteFriendshipPair(ctx, models.DeleteFriendshipPairParams{A: viewerID, B: targetID}))
}

func (s *SocialService) Friends(ctx context.Context, userID uuid.UUID) (*types.FriendsResponse, error) {
	fr, err := s.q.ListAcceptedFriends(ctx, userID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	in, err := s.q.ListIncomingRequests(ctx, userID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	out, err := s.q.ListOutgoingRequests(ctx, userID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	resp := &types.FriendsResponse{
		Friends:  make([]types.PersonBrief, len(fr)),
		Incoming: make([]types.PersonBrief, len(in)),
		Outgoing: make([]types.PersonBrief, len(out)),
	}
	for i, u := range fr {
		resp.Friends[i] = brief(u.ID, u.Name, u.PhotoUrl)
	}
	for i, u := range in {
		resp.Incoming[i] = brief(u.ID, u.Name, u.PhotoUrl)
	}
	for i, u := range out {
		resp.Outgoing[i] = brief(u.ID, u.Name, u.PhotoUrl)
	}
	return resp, nil
}

// ─── Blocks ───────────────────────────────────────────────────────────────────

// Block blocks the target and ends any friendship, request and follows between
// the two in both directions.
func (s *SocialService) Block(ctx context.Context, viewerID, targetID uuid.UUID) error {
	if viewerID == targetID {
		return apperror.BadRequest("you cannot block yourself")
	}
	if err := s.ensureUser(ctx, targetID); err != nil {
		return err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return apperror.Internal("could not start transaction")
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)
	if err := qtx.Block(ctx, models.BlockParams{BlockerID: viewerID, BlockedID: targetID}); err != nil {
		return apperror.FromDB(err)
	}
	if err := qtx.DeleteFriendshipPair(ctx, models.DeleteFriendshipPairParams{A: viewerID, B: targetID}); err != nil {
		return apperror.FromDB(err)
	}
	if err := qtx.DeleteFollowsBothWays(ctx, models.DeleteFollowsBothWaysParams{A: viewerID, B: targetID}); err != nil {
		return apperror.FromDB(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return apperror.Internal("could not block user")
	}
	return nil
}

func (s *SocialService) Unblock(ctx context.Context, viewerID, targetID uuid.UUID) error {
	return apperror.FromDBOrNil(s.q.Unblock(ctx, models.UnblockParams{BlockerID: viewerID, BlockedID: targetID}))
}

func (s *SocialService) Blocked(ctx context.Context, userID uuid.UUID) (*types.BlockedResponse, error) {
	rows, err := s.q.ListBlocked(ctx, userID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	users := make([]types.PersonBrief, len(rows))
	for i, u := range rows {
		users[i] = brief(u.ID, u.Name, u.PhotoUrl)
	}
	return &types.BlockedResponse{Users: users}, nil
}

// ─── Reports ──────────────────────────────────────────────────────────────────

func (s *SocialService) Report(ctx context.Context, reporterID uuid.UUID, in types.ReportInput) error {
	st := strings.ToLower(strings.TrimSpace(in.SubjectType))
	if st != "user" && st != "trip" && st != "comment" {
		return apperror.UnprocessableEntity("subject_type must be 'user', 'trip' or 'comment'")
	}
	reason := strings.ToLower(strings.TrimSpace(in.Reason))
	switch reason {
	case "spam", "harassment", "inappropriate", "misleading", "other":
	default:
		return apperror.UnprocessableEntity("reason must be spam, harassment, inappropriate, misleading or other")
	}
	if reason == "other" && (in.Details == nil || strings.TrimSpace(*in.Details) == "") {
		return apperror.UnprocessableEntity("details are required when reason is 'other'")
	}
	if in.SubjectID == uuid.Nil {
		return apperror.UnprocessableEntity("subject_id is required")
	}
	_, err := s.q.InsertReport(ctx, models.InsertReportParams{
		ReporterID: reporterID, SubjectType: st, SubjectID: in.SubjectID, Reason: reason, Details: in.Details,
	})
	return apperror.FromDBOrNil(err)
}

func (s *SocialService) ensureUser(ctx context.Context, id uuid.UUID) error {
	if _, err := s.q.GetUserByID(ctx, id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperror.NotFound("user not found")
		}
		return apperror.FromDB(err)
	}
	return nil
}
