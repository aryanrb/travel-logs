package services

import (
	"context"
	"errors"
	"math"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	db "travel-service/internal/database"
	"travel-service/internal/models"
	"travel-service/internal/types"
	"travel-service/internal/utils/apperror"
	"travel-service/internal/utils/money"
)

type ExpenseService struct {
	q   *models.Queries
	db  *pgxpool.Pool
	log *zap.Logger
}

func NewExpenseService() *ExpenseService {
	return &ExpenseService{q: models.New(db.Client), db: db.Client, log: zap.L().Named("ExpenseService")}
}

// ─── External payers ────────────────────────────────────────────────────────

func (s *ExpenseService) ListExternalPayers(ctx context.Context, tripID, userID uuid.UUID) ([]models.ExternalPayer, error) {
	if _, _, err := requireViewer(ctx, s.q, tripID, userID); err != nil {
		return nil, err
	}
	rows, err := s.q.ListExternalPayers(ctx, tripID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	return rows, nil
}

func (s *ExpenseService) CreateExternalPayer(ctx context.Context, tripID, userID uuid.UUID, name string, contact *string) (*models.ExternalPayer, error) {
	if _, _, err := requireContributor(ctx, s.q, tripID, userID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(name) == "" {
		return nil, apperror.UnprocessableEntity("payer name is required")
	}
	p, err := s.q.UpsertExternalPayer(ctx, models.UpsertExternalPayerParams{TripID: tripID, Name: strings.TrimSpace(name), Contact: contact})
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	return &p, nil
}

// ─── Party validation / resolution (within a tx) ────────────────────────────

func validatePartyExists(ctx context.Context, q *models.Queries, tripID uuid.UUID, userID, externalID *uuid.UUID) error {
	switch {
	case userID != nil && externalID != nil:
		return apperror.UnprocessableEntity("a party cannot be both a user and an external payer")
	case userID != nil:
		if _, err := q.GetUserByID(ctx, *userID); err != nil {
			return apperror.BadRequest("referenced user does not exist")
		}
	case externalID != nil:
		e, err := q.GetExternalPayer(ctx, *externalID)
		if err != nil || e.TripID != tripID {
			return apperror.BadRequest("referenced external payer does not belong to this trip")
		}
	default:
		return apperror.UnprocessableEntity("each party must reference a user or an external payer")
	}
	return nil
}

// resolvePartyRef validates a party reference and, for a new external payer
// named inline, creates (or reuses) the record. Returns the party as user XOR
// external. The party needs no edit permission on the trip (rule 4). Shared by
// expense payers and settlement from/to parties.
func resolvePartyRef(ctx context.Context, q *models.Queries, tripID uuid.UUID, p types.PartyRef) (*uuid.UUID, *uuid.UUID, error) {
	hasUser := p.UserID != nil
	hasExtID := p.ExternalID != nil
	hasExtName := p.ExternalName != nil && strings.TrimSpace(*p.ExternalName) != ""
	count := 0
	for _, b := range []bool{hasUser, hasExtID, hasExtName} {
		if b {
			count++
		}
	}
	if count != 1 {
		return nil, nil, apperror.UnprocessableEntity("party must reference exactly one of user_id, external_id, or external_name")
	}
	switch {
	case hasUser:
		if _, err := q.GetUserByID(ctx, *p.UserID); err != nil {
			return nil, nil, apperror.BadRequest("referenced user does not exist")
		}
		return p.UserID, nil, nil
	case hasExtID:
		e, err := q.GetExternalPayer(ctx, *p.ExternalID)
		if err != nil || e.TripID != tripID {
			return nil, nil, apperror.BadRequest("referenced external_id does not belong to this trip")
		}
		return nil, p.ExternalID, nil
	default:
		e, err := q.UpsertExternalPayer(ctx, models.UpsertExternalPayerParams{
			TripID: tripID, Name: strings.TrimSpace(*p.ExternalName), Contact: p.ExternalContact,
		})
		if err != nil {
			return nil, nil, apperror.FromDB(err)
		}
		return nil, &e.ID, nil
	}
}

// computeShares validates the split and returns one allocation per party. The
// allocations always reconcile to the expense total (core rule 10). Nobody is
// added automatically — only the parties listed get a share (rules 3 and 5).
func (s *ExpenseService) computeShares(ctx context.Context, q *models.Queries, tripID uuid.UUID, in types.ExpenseInput) ([]models.InsertShareParams, error) {
	if len(in.SharedAmong) == 0 {
		return nil, apperror.UnprocessableEntity("shared_among must list at least one party")
	}
	for _, sh := range in.SharedAmong {
		if err := validatePartyExists(ctx, q, tripID, sh.UserID, sh.ExternalID); err != nil {
			return nil, err
		}
	}

	var amounts []int64
	switch in.SplitMethod {
	case "equal":
		amounts = money.EqualSplit(in.Amount, len(in.SharedAmong))
	case "custom":
		usePercent := false
		for _, sh := range in.SharedAmong {
			if sh.Percent != nil {
				usePercent = true
				break
			}
		}
		if usePercent {
			pcts := make([]float64, len(in.SharedAmong))
			var sum float64
			for i, sh := range in.SharedAmong {
				if sh.Percent == nil {
					return nil, apperror.UnprocessableEntity("mix of percent and non-percent shares is not allowed")
				}
				pcts[i] = *sh.Percent
				sum += *sh.Percent
			}
			if math.Abs(sum-100) > 0.01 {
				return nil, apperror.UnprocessableEntity("custom percentages must total 100%")
			}
			amounts = money.PercentToAmounts(in.Amount, pcts)
		} else {
			amounts = make([]int64, len(in.SharedAmong))
			var sum int64
			for i, sh := range in.SharedAmong {
				if sh.Amount == nil {
					return nil, apperror.UnprocessableEntity("each custom share needs an amount or a percent")
				}
				if *sh.Amount < 0 {
					return nil, apperror.UnprocessableEntity("share amounts cannot be negative")
				}
				amounts[i] = *sh.Amount
				sum += *sh.Amount
			}
			if sum != in.Amount {
				return nil, apperror.UnprocessableEntity("custom amounts must total the expense amount")
			}
		}
	default:
		return nil, apperror.UnprocessableEntity("split_method must be 'equal' or 'custom'")
	}

	out := make([]models.InsertShareParams, len(in.SharedAmong))
	for i, sh := range in.SharedAmong {
		out[i] = models.InsertShareParams{UserID: sh.UserID, ExternalID: sh.ExternalID, Amount: amounts[i]}
	}
	return out, nil
}

func validateExpenseInput(in types.ExpenseInput) error {
	if strings.TrimSpace(in.Title) == "" {
		return apperror.UnprocessableEntity("expense title is required")
	}
	if in.Amount <= 0 {
		return apperror.UnprocessableEntity("amount must be greater than 0 (in minor units)")
	}
	if in.Date.Time().IsZero() {
		return apperror.UnprocessableEntity("date is required (YYYY-MM-DD)")
	}
	return nil
}

// resolveCurrency returns the expense currency: the per-expense override when
// given, otherwise the trip default.
func resolveCurrency(trip models.Trip, override *string) string {
	if override != nil && strings.TrimSpace(*override) != "" {
		return strings.ToUpper(strings.TrimSpace(*override))
	}
	return trip.Currency
}

// validateEntryLink ensures an expense's optional entry_id points at a timeline
// entry in the same trip, so expenses can't be stitched to another trip (422).
func validateEntryLink(ctx context.Context, q *models.Queries, tripID uuid.UUID, entryID *uuid.UUID) error {
	if entryID == nil {
		return nil
	}
	e, err := q.GetEntry(ctx, *entryID)
	if err != nil || e.TripID != tripID {
		return apperror.UnprocessableEntity("entry_id does not belong to this trip")
	}
	return nil
}

// validateCurrencyFields enforces the foreign-currency rule: the three original_*
// fields are all present or all absent; rate > 0; original_amount > 0; and the
// original currency differs from the trip currency.
func validateCurrencyFields(in types.ExpenseInput, tripCurrency string) error {
	n := 0
	for _, set := range []bool{in.OriginalCurrency != nil, in.OriginalAmount != nil, in.ExchangeRate != nil} {
		if set {
			n++
		}
	}
	if n == 0 {
		return nil
	}
	if n != 3 {
		return apperror.UnprocessableEntity("original_currency, original_amount and exchange_rate must be provided together")
	}
	if strings.TrimSpace(*in.OriginalCurrency) == "" {
		return apperror.UnprocessableEntity("original_currency is required")
	}
	if *in.ExchangeRate <= 0 {
		return apperror.UnprocessableEntity("exchange_rate must be greater than 0")
	}
	if *in.OriginalAmount <= 0 {
		return apperror.UnprocessableEntity("original_amount must be greater than 0 (in minor units)")
	}
	if strings.EqualFold(strings.TrimSpace(*in.OriginalCurrency), tripCurrency) {
		return apperror.UnprocessableEntity("original_currency must differ from the trip currency")
	}
	return nil
}

// validateFromPool enforces that a pool expense exists and is paid by the pool
// collector (so spending from the pool is modelled as the collector paying).
func validateFromPool(ctx context.Context, q *models.Queries, tripID uuid.UUID, in types.ExpenseInput) error {
	if !in.FromPool {
		return nil
	}
	pool, err := q.GetPool(ctx, tripID)
	if err != nil {
		return apperror.UnprocessableEntity("this trip has no pool")
	}
	if in.PaidBy.UserID == nil || *in.PaidBy.UserID != pool.CollectorID {
		return apperror.UnprocessableEntity("a pool expense must be paid by the collector")
	}
	return nil
}

// upperPtr trims and upper-cases an optional currency code.
func upperPtr(s *string) *string {
	if s == nil {
		return nil
	}
	u := strings.ToUpper(strings.TrimSpace(*s))
	return &u
}

// insertWithinTx inserts an expense and its shares using the given transactional
// queries, linked to in.EntryID when set. The caller owns the transaction and
// has already verified contributor permission and resolved the currency. Shared
// by direct expense creation and the nested expense on a timeline entry.
func (s *ExpenseService) insertWithinTx(ctx context.Context, qtx *models.Queries, tripID, userID uuid.UUID, in types.ExpenseInput, currency string) (models.Expense, error) {
	if err := validateCurrencyFields(in, currency); err != nil {
		return models.Expense{}, err
	}
	if err := validateFromPool(ctx, qtx, tripID, in); err != nil {
		return models.Expense{}, err
	}
	payerUser, payerExt, err := resolvePartyRef(ctx, qtx, tripID, in.PaidBy)
	if err != nil {
		return models.Expense{}, err
	}
	shares, err := s.computeShares(ctx, qtx, tripID, in)
	if err != nil {
		return models.Expense{}, err
	}
	exp, err := qtx.InsertExpense(ctx, models.InsertExpenseParams{
		TripID:           tripID,
		Title:            in.Title,
		Amount:           in.Amount,
		Currency:         currency,
		Category:         in.Category,
		ExpenseDate:      in.Date.Time(),
		PaidByUserID:     payerUser,
		PaidByExternalID: payerExt,
		RecordedBy:       userID,
		SplitMethod:      in.SplitMethod,
		Notes:            in.Notes,
		ReceiptUrl:       in.ReceiptURL,
		HideFromPublic:   in.HideFromPublic,
		EntryID:          in.EntryID,
		OriginalCurrency: upperPtr(in.OriginalCurrency),
		OriginalAmount:   in.OriginalAmount,
		ExchangeRate:     in.ExchangeRate,
		FromPool:         in.FromPool,
	})
	if err != nil {
		return models.Expense{}, apperror.FromDB(err)
	}
	for _, sh := range shares {
		sh.ExpenseID = exp.ID
		if err := qtx.InsertShare(ctx, sh); err != nil {
			return models.Expense{}, apperror.FromDB(err)
		}
	}
	// Every expense creation — direct or nested on an entry/journey — is logged here.
	if err := recordActivity(ctx, qtx, userID, activity{
		TripID: tripID, Action: "created", SubjectType: "expense", SubjectID: &exp.ID,
		Data: eventData{Title: exp.Title, Amount: &exp.Amount, Currency: &exp.Currency},
	}); err != nil {
		return models.Expense{}, err
	}
	return exp, nil
}

func (s *ExpenseService) Create(ctx context.Context, tripID, userID uuid.UUID, in types.ExpenseInput) (*types.ExpenseResponse, error) {
	_, trip, err := requireContributor(ctx, s.q, tripID, userID)
	if err != nil {
		return nil, err
	}
	if err := validateExpenseInput(in); err != nil {
		return nil, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, apperror.Internal("could not start transaction")
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)

	if err := validateEntryLink(ctx, qtx, tripID, in.EntryID); err != nil {
		return nil, err
	}
	exp, err := s.insertWithinTx(ctx, qtx, tripID, userID, in, resolveCurrency(trip, in.Currency))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, apperror.Internal("could not save expense")
	}
	return s.buildResponse(ctx, exp)
}

func (s *ExpenseService) expenseInTrip(ctx context.Context, expenseID, tripID uuid.UUID) (models.Expense, error) {
	e, err := s.q.GetExpense(ctx, expenseID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return e, apperror.NotFound("expense not found")
		}
		return e, apperror.FromDB(err)
	}
	if e.TripID != tripID {
		return e, apperror.NotFound("expense not found")
	}
	return e, nil
}

func (s *ExpenseService) Get(ctx context.Context, tripID, expenseID, userID uuid.UUID) (*types.ExpenseResponse, error) {
	if _, _, err := requireViewer(ctx, s.q, tripID, userID); err != nil {
		return nil, err
	}
	e, err := s.expenseInTrip(ctx, expenseID, tripID)
	if err != nil {
		return nil, err
	}
	return s.buildResponse(ctx, e)
}

func (s *ExpenseService) List(ctx context.Context, tripID, userID uuid.UUID) ([]models.Expense, error) {
	if _, _, err := requireViewer(ctx, s.q, tripID, userID); err != nil {
		return nil, err
	}
	rows, err := s.q.ListExpenses(ctx, tripID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	return rows, nil
}

func (s *ExpenseService) Update(ctx context.Context, tripID, expenseID, userID uuid.UUID, in types.ExpenseInput) (*types.ExpenseResponse, error) {
	role, trip, err := requireContributor(ctx, s.q, tripID, userID)
	if err != nil {
		return nil, err
	}
	e, err := s.expenseInTrip(ctx, expenseID, tripID)
	if err != nil {
		return nil, err
	}
	if !canModify(role, e.RecordedBy, userID) {
		return nil, apperror.Forbidden("you can only edit expenses you recorded")
	}
	if err := validateExpenseInput(in); err != nil {
		return nil, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, apperror.Internal("could not start transaction")
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)

	currency := resolveCurrency(trip, in.Currency)
	if err := validateEntryLink(ctx, qtx, tripID, in.EntryID); err != nil {
		return nil, err
	}
	if err := validateCurrencyFields(in, currency); err != nil {
		return nil, err
	}
	if err := validateFromPool(ctx, qtx, tripID, in); err != nil {
		return nil, err
	}
	payerUser, payerExt, err := resolvePartyRef(ctx, qtx, tripID, in.PaidBy)
	if err != nil {
		return nil, err
	}
	shares, err := s.computeShares(ctx, qtx, tripID, in)
	if err != nil {
		return nil, err
	}
	if err := qtx.UpdateExpense(ctx, models.UpdateExpenseParams{
		Title:            in.Title,
		Amount:           in.Amount,
		Currency:         currency,
		Category:         in.Category,
		ExpenseDate:      in.Date.Time(),
		PaidByUserID:     payerUser,
		PaidByExternalID: payerExt,
		SplitMethod:      in.SplitMethod,
		Notes:            in.Notes,
		ReceiptUrl:       in.ReceiptURL,
		HideFromPublic:   in.HideFromPublic,
		EntryID:          in.EntryID, // omitted in body → nil → NULL
		OriginalCurrency: upperPtr(in.OriginalCurrency),
		OriginalAmount:   in.OriginalAmount,
		ExchangeRate:     in.ExchangeRate,
		FromPool:         in.FromPool,
		ID:               expenseID,
	}); err != nil {
		return nil, apperror.FromDB(err)
	}
	if err := qtx.DeleteSharesForExpense(ctx, expenseID); err != nil {
		return nil, apperror.FromDB(err)
	}
	for _, sh := range shares {
		sh.ExpenseID = expenseID
		if err := qtx.InsertShare(ctx, sh); err != nil {
			return nil, apperror.FromDB(err)
		}
	}
	if err := recordActivity(ctx, qtx, userID, activity{
		TripID: tripID, Action: "updated", SubjectType: "expense", SubjectID: &expenseID,
		Data: eventData{Title: in.Title, Amount: &in.Amount, Currency: &currency},
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, apperror.Internal("could not update expense")
	}
	updated, err := s.q.GetExpense(ctx, expenseID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	return s.buildResponse(ctx, updated)
}

func (s *ExpenseService) Delete(ctx context.Context, tripID, expenseID, userID uuid.UUID) error {
	role, _, err := requireContributor(ctx, s.q, tripID, userID)
	if err != nil {
		return err
	}
	e, err := s.expenseInTrip(ctx, expenseID, tripID)
	if err != nil {
		return err
	}
	if !canModify(role, e.RecordedBy, userID) {
		return apperror.Forbidden("you can only delete expenses you recorded")
	}
	return inTx(ctx, s.db, s.q, userID, func(qtx *models.Queries) (*activity, error) {
		if err := qtx.DeleteExpense(ctx, expenseID); err != nil { // shares cascade
			return nil, apperror.FromDB(err)
		}
		return &activity{TripID: tripID, Action: "deleted", SubjectType: "expense", SubjectID: &e.ID,
			Data: eventData{Title: e.Title, Amount: &e.Amount, Currency: &e.Currency}}, nil
	})
}

func (s *ExpenseService) buildResponse(ctx context.Context, e models.Expense) (*types.ExpenseResponse, error) {
	rawShares, err := s.q.ListSharesForExpense(ctx, e.ID)
	if err != nil {
		return nil, apperror.FromDB(err)
	}
	r := newPartyResolver(ctx, s.q)
	shares := make([]types.ShareOut, len(rawShares))
	for i, sh := range rawShares {
		shares[i] = types.ShareOut{Party: r.party(sh.UserID, sh.ExternalID), Amount: sh.Amount}
	}
	return &types.ExpenseResponse{
		Expense: e,
		PaidBy:  r.party(e.PaidByUserID, e.PaidByExternalID),
		Shares:  shares,
	}, nil
}
