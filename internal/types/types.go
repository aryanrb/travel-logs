// Package types holds request/response DTOs shared by handlers and services.
package types

import (
	"strings"
	"time"

	"github.com/google/uuid"

	"travel-service/internal/models"
)

// Date binds a JSON "2006-01-02" string to a date. Avoids the RFC3339 that
// time.Time expects by default, which rejects plain calendar dates.
type Date time.Time

func (d *Date) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		return nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return err
	}
	*d = Date(t)
	return nil
}

func (d Date) MarshalJSON() ([]byte, error) {
	return []byte(`"` + time.Time(d).Format("2006-01-02") + `"`), nil
}

func (d Date) Time() time.Time { return time.Time(d) }

// DatePtr converts an optional request Date into an optional time.Time.
func DatePtr(d *Date) *time.Time {
	if d == nil {
		return nil
	}
	t := time.Time(*d)
	return &t
}

// ─── Auth & profile ─────────────────────────────────────────────────────────

type RegisterInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

type LoginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type UpdateProfileInput struct {
	Name     string  `json:"name"`
	PhotoURL *string `json:"photo_url"`
}

type PublicUser struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	Email    string    `json:"email"`
	PhotoURL *string   `json:"photo_url"`
}

type AuthResponse struct {
	Token string     `json:"token"`
	User  PublicUser `json:"user"`
}

// ─── Trips ──────────────────────────────────────────────────────────────────

type TripInput struct {
	Title         string  `json:"title"`
	CoverImage    *string `json:"cover_image"`
	Description   *string `json:"description"`
	StartDate     *Date   `json:"start_date"`
	EndDate       *Date   `json:"end_date"`
	StartLocation *string `json:"start_location"`
	TripType      *string `json:"trip_type"`
	Currency      *string `json:"currency"`
}

type DestinationInput struct {
	Name        string `json:"name"`
	ArrivalDate *Date  `json:"arrival_date"`
	SortOrder   int32  `json:"sort_order"`
}

type SetPublicInput struct {
	IsPublic bool `json:"is_public"`
}

type TripsListResponse struct {
	Owned  []models.Trip               `json:"owned"`
	Shared []models.ListSharedTripsRow `json:"shared"`
}

type TripDetailResponse struct {
	Trip         models.Trip              `json:"trip"`
	Destinations []models.TripDestination `json:"destinations"`
	YourRole     string                   `json:"your_role"`
}

// ─── Participants ───────────────────────────────────────────────────────────

// InviteInput invites either an existing user (UserID) or an email (InvitedEmail).
type InviteInput struct {
	UserID       *uuid.UUID `json:"user_id"`
	InvitedEmail *string    `json:"invited_email"`
	Role         string     `json:"role"`
}

type AcceptInviteInput struct {
	Token string `json:"token"`
}

type UpdateRoleInput struct {
	Role string `json:"role"`
}

// ─── Timeline & photos ──────────────────────────────────────────────────────

type EntryInput struct {
	Title       string  `json:"title"`
	Description *string `json:"description"`
	EntryDate   Date    `json:"entry_date"`
	EntryTime   *string `json:"entry_time"` // "HH:MM"
	Location    *string `json:"location"`
}

type PhotoInput struct {
	EntryID   *uuid.UUID `json:"entry_id"`
	URL       string     `json:"url"`
	Caption   *string    `json:"caption"`
	TakenDate *Date      `json:"taken_date"`
}

// ─── Transportation ─────────────────────────────────────────────────────────

type TransportInput struct {
	EntryID     *uuid.UUID `json:"entry_id"`
	ExpenseID   *uuid.UUID `json:"expense_id"`
	Type        string     `json:"type"`
	StartPoint  *string    `json:"start_point"`
	Destination *string    `json:"destination"`
	DepartAt    *time.Time `json:"depart_at"` // RFC3339
	DistanceKm  *float64   `json:"distance_km"`
	DurationMin *int32     `json:"duration_min"`
	CostAmount  *int64     `json:"cost_amount"` // minor units
	Notes       *string    `json:"notes"`
}

// ─── Expenses ───────────────────────────────────────────────────────────────

// PartyRef points at a user, an existing external payer, or (for payers) a new
// external payer created inline by name. Exactly one identity must resolve.
type PartyRef struct {
	UserID          *uuid.UUID `json:"user_id"`
	ExternalID      *uuid.UUID `json:"external_id"`
	ExternalName    *string    `json:"external_name"`
	ExternalContact *string    `json:"external_contact"`
}

// ShareInput is one party's slice of an expense. For "equal" split the amounts
// are computed and these are ignored. For "custom" supply amount OR percent.
type ShareInput struct {
	UserID     *uuid.UUID `json:"user_id"`
	ExternalID *uuid.UUID `json:"external_id"`
	Amount     *int64     `json:"amount"`
	Percent    *float64   `json:"percent"`
}

type ExpenseInput struct {
	Title       string       `json:"title"`
	Amount      int64        `json:"amount"` // minor units, > 0
	Currency    *string      `json:"currency"`
	Category    *string      `json:"category"`
	Date        Date         `json:"date"`
	PaidBy      PartyRef     `json:"paid_by"`
	SplitMethod string       `json:"split_method"` // "equal" | "custom"
	SharedAmong []ShareInput `json:"shared_among"`
	Notes       *string      `json:"notes"`
	ReceiptURL  *string      `json:"receipt_url"`
}

type Party struct {
	Type string    `json:"type"` // "user" | "external"
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type ShareOut struct {
	Party  Party `json:"party"`
	Amount int64 `json:"amount"`
}

type ExpenseResponse struct {
	Expense models.Expense `json:"expense"`
	PaidBy  Party          `json:"paid_by"`
	Shares  []ShareOut     `json:"shares"`
}

// ─── Settlements & balances ─────────────────────────────────────────────────

type SettlementInput struct {
	From        PartyRef `json:"from"`
	To          PartyRef `json:"to"`
	Amount      int64    `json:"amount"` // minor units, > 0
	SettledDate Date     `json:"settled_date"`
	Status      *string  `json:"status"` // "completed" (default) | "pending"
	Notes       *string  `json:"notes"`
}

type BalanceEntry struct {
	Party           Party `json:"party"`
	TotalPaid       int64 `json:"total_paid"`
	TotalShare      int64 `json:"total_share"`
	SettledPaid     int64 `json:"settled_paid"`
	SettledReceived int64 `json:"settled_received"`
	Net             int64 `json:"net"` // + should receive, - owes, 0 settled
}

type BalancesResponse struct {
	Currency string         `json:"currency"`
	Balances []BalanceEntry `json:"balances"`
}

// ─── Summary ────────────────────────────────────────────────────────────────

type SummaryResponse struct {
	Currency          string                         `json:"currency"`
	StartDate         *time.Time                     `json:"start_date"`
	EndDate           *time.Time                     `json:"end_date"`
	TravelDays        int                            `json:"travel_days"`
	Destinations      []models.TripDestination       `json:"destinations"`
	TimelineEntries   int                            `json:"timeline_entries"`
	Photos            int64                          `json:"photos"`
	TotalExpenses     int64                          `json:"total_expenses"`
	CategoryBreakdown []models.CategoryBreakdownRow  `json:"category_breakdown"`
	Transportation    []models.TransportBreakdownRow `json:"transportation"`
	Balances          []BalanceEntry                 `json:"balances"`
}
