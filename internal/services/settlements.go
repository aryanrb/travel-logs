package services

import (
	"context"
	"errors"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	db "travel-service/internal/database"
	"travel-service/internal/models"
	"travel-service/internal/types"
	"travel-service/internal/utils/apperror"
)

type SettlementService struct {
	q   *models.Queries
	db  *pgxpool.Pool
	log *zap.Logger
}

func NewSettlementService() *SettlementService {
	return &SettlementService{q: models.New(db.Client), db: db.Client, log: zap.L().Named("SettlementService")}
}

// partyKey identifies a party (user or external) for balance aggregation.
type partyKey struct {
	isUser bool
	id     uuid.UUID
}

type balanceAgg struct {
	paid, share, settledPaid, settledRecv int64
}

func mkKey(userID, externalID *uuid.UUID) (partyKey, bool) {
	switch {
	case userID != nil:
		return partyKey{true, *userID}, true
	case externalID != nil:
		return partyKey{false, *externalID}, true
	default:
		return partyKey{}, false
	}
}

// aggregateBalances is the pure settlement math (no DB), kept separate so it can
// be unit-tested. Net = paid − share + settledPaid − settledReceived.
// Only COMPLETED settlements move balances — a pending/intended repayment does
// not (core rule 11). A payer is never auto-added to the share side (rule 5): a
// party only accrues `share` if they appear in an expense_shares row.
func aggregateBalances(expenses []models.Expense, shares []models.ListTripSharesRow, settlements []models.Settlement) map[partyKey]*balanceAgg {
	m := map[partyKey]*balanceAgg{}
	get := func(k partyKey) *balanceAgg {
		if m[k] == nil {
			m[k] = &balanceAgg{}
		}
		return m[k]
	}
	for _, e := range expenses {
		if k, ok := mkKey(e.PaidByUserID, e.PaidByExternalID); ok {
			get(k).paid += e.Amount
		}
	}
	for _, sh := range shares {
		if k, ok := mkKey(sh.UserID, sh.ExternalID); ok {
			get(k).share += sh.Amount
		}
	}
	for _, st := range settlements {
		if st.Status != "completed" {
			continue
		}
		if k, ok := mkKey(st.FromUserID, st.FromExternalID); ok {
			get(k).settledPaid += st.Amount
		}
		if k, ok := mkKey(st.ToUserID, st.ToExternalID); ok {
			get(k).settledRecv += st.Amount
		}
	}
	return m
}

// computeTripBalances builds the named, sorted balance list for a trip. Shared
// by the settlements endpoint and the trip summary.
func computeTripBalances(ctx context.Context, q *models.Queries, tripID uuid.UUID) ([]types.BalanceEntry, error) {
	expenses, err := q.ListExpenses(ctx, tripID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	shares, err := q.ListTripShares(ctx, tripID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	setts, err := q.ListSettlements(ctx, tripID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}

	agg := aggregateBalances(expenses, shares, setts)
	r := newPartyResolver(ctx, q)
	entries := make([]types.BalanceEntry, 0, len(agg))
	for k, v := range agg {
		var uid, eid *uuid.UUID
		id := k.id
		if k.isUser {
			uid = &id
		} else {
			eid = &id
		}
		entries = append(entries, types.BalanceEntry{
			Party:           r.party(uid, eid),
			TotalPaid:       v.paid,
			TotalShare:      v.share,
			SettledPaid:     v.settledPaid,
			SettledReceived: v.settledRecv,
			Net:             v.paid - v.share + v.settledPaid - v.settledRecv,
		})
	}
	// Deterministic order: largest creditor first, then by name.
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Net != entries[j].Net {
			return entries[i].Net > entries[j].Net
		}
		return entries[i].Party.Name < entries[j].Party.Name
	})
	return entries, nil
}

func (s *SettlementService) Balances(ctx context.Context, tripID, userID uuid.UUID) (*types.BalancesResponse, error) {
	_, trip, err := requireViewer(ctx, s.q, tripID, userID)
	if err != nil {
		return nil, err
	}
	entries, err := computeTripBalances(ctx, s.q, tripID)
	if err != nil {
		return nil, err
	}
	return &types.BalancesResponse{Currency: trip.Currency, Balances: entries}, nil
}

// ─── Settlement CRUD ─────────────────────────────────────────────────────────

func (s *SettlementService) List(ctx context.Context, tripID, userID uuid.UUID) ([]models.Settlement, error) {
	if _, _, err := requireViewer(ctx, s.q, tripID, userID); err != nil {
		return nil, err
	}
	rows, err := s.q.ListSettlements(ctx, tripID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	return rows, nil
}

func (s *SettlementService) Create(ctx context.Context, tripID, userID uuid.UUID, in types.SettlementInput) (*models.Settlement, error) {
	if _, _, err := requireContributor(ctx, s.q, tripID, userID); err != nil {
		return nil, err
	}
	if in.Amount <= 0 {
		return nil, apperror.UnprocessableEntity("amount must be greater than 0 (in minor units)")
	}
	if in.SettledDate.Time().IsZero() {
		return nil, apperror.UnprocessableEntity("settled_date is required (YYYY-MM-DD)")
	}
	status := "completed"
	if in.Status != nil {
		if *in.Status != "completed" && *in.Status != "pending" {
			return nil, apperror.UnprocessableEntity("status must be 'completed' or 'pending'")
		}
		status = *in.Status
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, apperror.Internal("could not start transaction")
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)

	fromUser, fromExt, err := resolvePartyRef(ctx, qtx, tripID, in.From)
	if err != nil {
		return nil, err
	}
	toUser, toExt, err := resolvePartyRef(ctx, qtx, tripID, in.To)
	if err != nil {
		return nil, err
	}
	fk, _ := mkKey(fromUser, fromExt)
	tk, _ := mkKey(toUser, toExt)
	if fk == tk {
		return nil, apperror.UnprocessableEntity("a settlement must be between two different parties")
	}

	st, err := qtx.InsertSettlement(ctx, models.InsertSettlementParams{
		TripID:         tripID,
		FromUserID:     fromUser,
		FromExternalID: fromExt,
		ToUserID:       toUser,
		ToExternalID:   toExt,
		Amount:         in.Amount,
		SettledDate:    in.SettledDate.Time(),
		Status:         status,
		Notes:          in.Notes,
		RecordedBy:     userID,
	})
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, apperror.Internal("could not save settlement")
	}
	return &st, nil
}

func (s *SettlementService) Delete(ctx context.Context, tripID, settlementID, userID uuid.UUID) error {
	role, _, err := requireContributor(ctx, s.q, tripID, userID)
	if err != nil {
		return err
	}
	st, err := s.q.GetSettlement(ctx, settlementID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperror.NotFound("settlement not found")
		}
		return apperror.FromDB(err)
	}
	if st.TripID != tripID {
		return apperror.NotFound("settlement not found")
	}
	if !canModify(role, st.RecordedBy, userID) {
		return apperror.Forbidden("you can only delete settlements you recorded")
	}
	return apperror.FromDBOrNil(s.q.DeleteSettlement(ctx, settlementID))
}
