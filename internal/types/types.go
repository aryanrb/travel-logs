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

// DateFromPtr converts an optional time.Time into an optional output Date.
func DateFromPtr(t *time.Time) *Date {
	if t == nil {
		return nil
	}
	d := Date(*t)
	return &d
}

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
	Bio      *string `json:"bio"`
}

type PublicUser struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	Email    string    `json:"email"`
	PhotoURL *string   `json:"photo_url"`
	Bio      *string   `json:"bio"`
}

type AuthResponse struct {
	Token string     `json:"token"`
	User  PublicUser `json:"user"`
}

// ─── Trips ──────────────────────────────────────────────────────────────────

type TripInput struct {
	Title         string   `json:"title"`
	CoverImage    *string  `json:"cover_image"`
	Description   *string  `json:"description"`
	StartDate     *Date    `json:"start_date"`
	EndDate       *Date    `json:"end_date"`
	StartLocation *string  `json:"start_location"`
	TripType      *string  `json:"trip_type"`
	Currency      *string  `json:"currency"`
	StartLat      *float64 `json:"start_lat"` // map pin; set only when pinned, wins over name lookup
	StartLng      *float64 `json:"start_lng"`
}

type DestinationInput struct {
	Name        string   `json:"name"`
	ArrivalDate *Date    `json:"arrival_date"`
	SortOrder   int32    `json:"sort_order"`
	Lat         *float64 `json:"lat"`
	Lng         *float64 `json:"lng"`
}

type SetPublicInput struct {
	IsPublic        bool `json:"is_public"`
	ShowCostsPublic bool `json:"show_costs_public"`
}

type CategoryBudget struct {
	Category string `json:"category"`
	Amount   int64  `json:"amount"`
}

// TripResponse is a trip plus the fields every trip response carries: owner_name
// (no email) and category_budgets ([] when none). budget_amount and start coords
// are trip columns, promoted from the embedded Trip.
type TripResponse struct {
	models.Trip
	OwnerName       string           `json:"owner_name"`
	CategoryBudgets []CategoryBudget `json:"category_budgets"`
}

type SharedTripResponse struct {
	Trip TripResponse `json:"trip"`
	Role string       `json:"role"`
}

type TripsListResponse struct {
	Owned  []TripResponse       `json:"owned"`
	Shared []SharedTripResponse `json:"shared"`
}

type TripDetailResponse struct {
	Trip         TripResponse             `json:"trip"`
	Destinations []models.TripDestination `json:"destinations"`
	YourRole     string                   `json:"your_role"`
}

// BudgetInput fully replaces a trip's budget. Amounts are minor units, > 0.
type CategoryBudgetInput struct {
	Category string `json:"category"`
	Amount   int64  `json:"amount"`
}

type BudgetInput struct {
	BudgetAmount    *int64                `json:"budget_amount"`
	CategoryBudgets []CategoryBudgetInput `json:"category_budgets"`
}

type ActivityResponse struct {
	Events []models.TripEvent `json:"events"`
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
	Title       string        `json:"title"`
	Description *string       `json:"description"`
	EntryDate   Date          `json:"entry_date"`
	EntryTime   *string       `json:"entry_time"` // "HH:MM"
	Location    *string       `json:"location"`
	Lat         *float64      `json:"lat"`
	Lng         *float64      `json:"lng"`
	Expense     *ExpenseInput `json:"expense"` // optional: create an expense linked to this entry in the same tx
}

type PhotoInput struct {
	EntryID   *uuid.UUID `json:"entry_id"`
	URL       string     `json:"url"`
	Caption   *string    `json:"caption"`
	TakenDate *Date      `json:"taken_date"`
}

// ─── Transportation ─────────────────────────────────────────────────────────

type TransportInput struct {
	EntryID        *uuid.UUID `json:"entry_id"`
	ExpenseID      *uuid.UUID `json:"expense_id"`
	Type           string     `json:"type"`
	StartPoint     *string    `json:"start_point"`
	Destination    *string    `json:"destination"`
	DepartAt       *time.Time `json:"depart_at"` // RFC3339
	DistanceKm     *float64   `json:"distance_km"`
	DurationMin    *int32     `json:"duration_min"`
	CostAmount     *int64     `json:"cost_amount"` // minor units
	Notes          *string    `json:"notes"`
	StartLat       *float64   `json:"start_lat"`
	StartLng       *float64   `json:"start_lng"`
	DestinationLat *float64   `json:"destination_lat"`
	DestinationLng *float64   `json:"destination_lng"`
	// Optional nested expense: created first, then the journey is saved with
	// expense_id set to it (overriding any expense_id above), in one transaction.
	Expense *ExpenseInput `json:"expense"`
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
	Title          string       `json:"title"`
	Amount         int64        `json:"amount"` // minor units, > 0
	Currency       *string      `json:"currency"`
	Category       *string      `json:"category"`
	Date           Date         `json:"date"`
	PaidBy         PartyRef     `json:"paid_by"`
	SplitMethod    string       `json:"split_method"` // "equal" | "custom"
	SharedAmong    []ShareInput `json:"shared_among"`
	Notes          *string      `json:"notes"`
	ReceiptURL     *string      `json:"receipt_url"`
	HideFromPublic bool         `json:"hide_from_public"`
	EntryID        *uuid.UUID   `json:"entry_id"` // links the expense to a timeline entry; omit for standalone
	// Foreign currency: supply all three or none. amount stays in the trip
	// currency and drives splits/balances; these record the original.
	OriginalCurrency *string  `json:"original_currency"`
	OriginalAmount   *int64   `json:"original_amount"` // minor units of original_currency
	ExchangeRate     *float64 `json:"exchange_rate"`   // trip-currency units per 1 original unit, > 0
	FromPool         bool     `json:"from_pool"`       // spent from the pool; paid_by must be the collector
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
	Kind        *string  `json:"kind"` // "repayment" (default) | "contribution" (to = collector)
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

// ─── Public (unauthenticated) ────────────────────────────────────────────────

// PublicTripSafe is the whitelist of trip fields safe to expose publicly.
// Nothing financial or personal beyond these appears in public responses.
type PublicTripSafe struct {
	ID            uuid.UUID `json:"id"`
	OwnerID       uuid.UUID `json:"owner_id"` // web app links owner names to /u/<owner_id>
	Title         string    `json:"title"`
	Description   *string   `json:"description"`
	CoverImage    *string   `json:"cover_image"`
	StartDate     *Date     `json:"start_date"`
	EndDate       *Date     `json:"end_date"`
	StartLocation *string   `json:"start_location"`
	TripType      *string   `json:"trip_type"`
	Currency      string    `json:"currency"`
	IsPublic      bool      `json:"is_public"`
	StartLat      *float64  `json:"start_lat"`
	StartLng      *float64  `json:"start_lng"`
}

type PublicTripListItem struct {
	Trip         PublicTripSafe `json:"trip"`
	Destinations []string       `json:"destinations"`
	OwnerName    string         `json:"owner_name"`
	// cost_per_person is total spent ÷ travellers in the trip's currency, or null
	// when the owner keeps costs private. converted is set only when a different
	// `currency` was requested (approximate; the web app labels it "≈").
	CostPerPerson          *int64         `json:"cost_per_person"`
	CostPerPersonConverted *ConvertedCost `json:"cost_per_person_converted,omitempty"`
	// Likes & saves (stage 6). Populated by the explore/profile/detail paths.
	Likes     int64 `json:"likes"`
	LikedByMe bool  `json:"liked_by_me"`
	Saves     int64 `json:"saves"`
	SavedByMe bool  `json:"saved_by_me"`
}

type ConvertedCost struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

type PublicTripListResponse struct {
	Trips      []PublicTripListItem `json:"trips"`
	NextCursor *string              `json:"next_cursor,omitempty"` // absent on the last page
}

// ExploreParams holds the parsed query of GET /public/trips. All fields are
// optional; zero values mean "no filter".
type ExploreParams struct {
	Q                string
	TripType         string
	Destination      string
	MinDays          *int
	MaxDays          *int
	MinCostPerPerson *int64
	MaxCostPerPerson *int64
	Currency         string // target for cost filter + cost_per_person_converted
	Sort             string // shortest | longest | soonest | popular | ""(default)
	Following        bool   // only trips by people the visitor follows (signed in)
	Limit            int
	Cursor           string
}

type PublicPerson struct {
	Name string `json:"name"`
	Role string `json:"role"`
}

type PublicDestination struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	ArrivalDate *Date     `json:"arrival_date"`
	SortOrder   int32     `json:"sort_order"`
	Lat         *float64  `json:"lat"`
	Lng         *float64  `json:"lng"`
}

type PublicEntry struct {
	ID          uuid.UUID `json:"id"`
	Title       string    `json:"title"`
	Description *string   `json:"description"`
	EntryDate   Date      `json:"entry_date"`
	EntryTime   *string   `json:"entry_time"`
	Location    *string   `json:"location"`
	Lat         *float64  `json:"lat"`
	Lng         *float64  `json:"lng"`
}

type PublicTransport struct {
	ID             uuid.UUID  `json:"id"`
	Type           string     `json:"type"`
	StartPoint     *string    `json:"start_point"`
	Destination    *string    `json:"destination"`
	DepartAt       *time.Time `json:"depart_at"`
	DistanceKm     *float64   `json:"distance_km"`
	DurationMin    *int32     `json:"duration_min"`
	Notes          *string    `json:"notes"`
	EntryID        *uuid.UUID `json:"entry_id"`
	StartLat       *float64   `json:"start_lat"`
	StartLng       *float64   `json:"start_lng"`
	DestinationLat *float64   `json:"destination_lat"`
	DestinationLng *float64   `json:"destination_lng"`
	CostAmount     *int64     `json:"cost_amount,omitempty"` // only when show_costs_public
}

type PublicPhoto struct {
	ID        uuid.UUID  `json:"id"`
	URL       string     `json:"url"`
	Caption   *string    `json:"caption"`
	TakenDate *Date      `json:"taken_date"`
	EntryID   *uuid.UUID `json:"entry_id"`
}

type PublicBudget struct {
	Currency   string                              `json:"currency"`
	Total      int64                               `json:"total"`
	Travellers int                                 `json:"travellers"`
	ByCategory []models.PublicCategoryBreakdownRow `json:"by_category"`
	Planned    *int64                              `json:"planned"` // the trip's budget_amount
}

type PublicTripDetail struct {
	Trip           PublicTripSafe      `json:"trip"`
	Destinations   []PublicDestination `json:"destinations"`
	People         []PublicPerson      `json:"people"`
	Entries        []PublicEntry       `json:"entries"`
	Transportation []PublicTransport   `json:"transportation"`
	Photos         []PublicPhoto       `json:"photos"`
	Budget         *PublicBudget       `json:"budget"`
	Likes          int64               `json:"likes"`
	LikedByMe      bool                `json:"liked_by_me"`
	Saves          int64               `json:"saves"`
	SavedByMe      bool                `json:"saved_by_me"`
}

type CopyTripInput struct {
	Title     string `json:"title"`
	StartDate Date   `json:"start_date"`
}

// ─── RSVP invite links ────────────────────────────────────────────────────────

type InviteLinkInput struct {
	Active   bool    `json:"active"`
	JoinMode string  `json:"join_mode"` // anyone | friends | approval
	Spots    *int32  `json:"spots"`     // null = unlimited
	ReplyBy  *Date   `json:"reply_by"`  // null = no deadline
	Role     string  `json:"role"`      // collaborator | viewer (granted on Going)
	Message  *string `json:"message"`
}

type InviteLinkResponse struct {
	Token    string  `json:"token"`
	Active   bool    `json:"active"`
	JoinMode string  `json:"join_mode"`
	Spots    *int32  `json:"spots"`
	ReplyBy  *Date   `json:"reply_by"`
	Role     string  `json:"role"`
	Message  *string `json:"message"`
}

type RSVPItem struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	Name      string    `json:"name"`
	PhotoURL  *string   `json:"photo_url"`
	Status    string    `json:"status"`
	RepliedAt time.Time `json:"replied_at"`
}

type RSVPListResponse struct {
	RSVPs []RSVPItem `json:"rsvps"`
}

// Invitee-facing /join/:token views.

type JoinTripInfo struct {
	ID            uuid.UUID `json:"id"`
	Title         string    `json:"title"`
	CoverImage    *string   `json:"cover_image"`
	StartDate     *Date     `json:"start_date"`
	EndDate       *Date     `json:"end_date"`
	StartLocation *string   `json:"start_location"`
	TripType      *string   `json:"trip_type"`
	Currency      string    `json:"currency"`
}

type JoinPerson struct {
	Name     string  `json:"name"`
	PhotoURL *string `json:"photo_url"`
}

type JoinCounts struct {
	Going int `json:"going"`
	Maybe int `json:"maybe"`
}

type JoinResponse struct {
	Trip                 JoinTripInfo `json:"trip"`
	Destinations         []string     `json:"destinations"`
	Organiser            JoinPerson   `json:"organiser"`
	Message              *string      `json:"message"`
	JoinMode             string       `json:"join_mode"`
	Spots                *int32       `json:"spots"`
	ReplyBy              *Date        `json:"reply_by"`
	Closed               bool         `json:"closed"`
	Counts               JoinCounts   `json:"counts"`
	GoingPeople          []JoinPerson `json:"going_people"`
	PlannedCostPerPerson *int64       `json:"planned_cost_per_person,omitempty"`
	MyRSVP               *string      `json:"my_rsvp,omitempty"`
}

type RSVPReplyInput struct {
	Status string `json:"status"` // going | maybe | declined
}

type RSVPReplyResponse struct {
	Status string    `json:"status"` // what actually happened (may be waitlist/requested)
	TripID uuid.UUID `json:"trip_id"`
}

// ─── People: profiles, follows, friends, blocks, reports ──────────────────────

type ProfileUser struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	PhotoURL  *string   `json:"photo_url"`
	Bio       *string   `json:"bio"`
	CreatedAt time.Time `json:"created_at"`
}

type ProfileStats struct {
	Trips     int `json:"trips"` // public trips only
	Followers int `json:"followers"`
	Following int `json:"following"`
	Friends   int `json:"friends"`
}

// Relationship is the viewer's relationship to the profile owner; null when
// signed out or viewing yourself. friend ∈ none|requested|incoming|friends.
type Relationship struct {
	Following  bool   `json:"following"`
	FollowsYou bool   `json:"follows_you"`
	Friend     string `json:"friend"`
	Blocked    bool   `json:"blocked"`
}

type ProfileResponse struct {
	User         ProfileUser          `json:"user"`
	Stats        ProfileStats         `json:"stats"`
	Trips        []PublicTripListItem `json:"trips"`
	Relationship *Relationship        `json:"relationship"`
}

type PersonBrief struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	PhotoURL *string   `json:"photo_url"`
}

type FriendsResponse struct {
	Friends  []PersonBrief `json:"friends"`
	Incoming []PersonBrief `json:"incoming"`
	Outgoing []PersonBrief `json:"outgoing"`
}

type BlockedResponse struct {
	Users []PersonBrief `json:"users"`
}

type ReportInput struct {
	SubjectType string    `json:"subject_type"` // user | trip | comment
	SubjectID   uuid.UUID `json:"subject_id"`
	Reason      string    `json:"reason"` // spam|harassment|inappropriate|misleading|other
	Details     *string   `json:"details"`
}

// ─── Notifications ────────────────────────────────────────────────────────────

type NotificationActor struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	PhotoURL *string   `json:"photo_url"`
}

type NotificationTrip struct {
	ID    uuid.UUID `json:"id"`
	Title string    `json:"title"`
}

type NotificationItem struct {
	ID         uuid.UUID          `json:"id"`
	Type       string             `json:"type"`
	Actor      *NotificationActor `json:"actor"`
	Trip       *NotificationTrip  `json:"trip"`
	CommentID  *uuid.UUID         `json:"comment_id"`
	RSVPStatus *string            `json:"rsvp_status"`
	CreatedAt  time.Time          `json:"created_at"`
	Read       bool               `json:"read"`
}

type NotificationsResponse struct {
	Notifications []NotificationItem `json:"notifications"`
	Unread        int                `json:"unread"` // total unread, not just this page
	NextCursor    *string            `json:"next_cursor,omitempty"`
}

type MarkReadInput struct {
	IDs []uuid.UUID `json:"ids"` // absent/empty → mark all read
}

// ─── Comments ─────────────────────────────────────────────────────────────────

type CommentInput struct {
	Body     string     `json:"body"`
	ParentID *uuid.UUID `json:"parent_id"` // reply target; must be a top-level comment on the same trip
}

type UpdateCommentInput struct {
	Body string `json:"body"`
}

type CommentAuthor struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	PhotoURL *string   `json:"photo_url"`
}

type CommentItem struct {
	ID        uuid.UUID     `json:"id"`
	Author    CommentAuthor `json:"author"`
	Body      string        `json:"body"`
	CreatedAt time.Time     `json:"created_at"`
	EditedAt  *time.Time    `json:"edited_at"`
	ParentID  *uuid.UUID    `json:"parent_id"`
}

type CommentsResponse struct {
	Comments   []CommentItem `json:"comments"`
	NextCursor *string       `json:"next_cursor,omitempty"`
}

// ─── Trip pool ────────────────────────────────────────────────────────────────

type PoolInput struct {
	CollectorID uuid.UUID `json:"collector_id"`
	PerPerson   int64     `json:"per_person"` // minor units, > 0
	DueDate     *Date     `json:"due_date"`
}

type PoolResponse struct {
	Enabled     bool      `json:"enabled"`
	CollectorID uuid.UUID `json:"collector_id"`
	PerPerson   int64     `json:"per_person"`
	DueDate     *Date     `json:"due_date"`
	Collected   int64     `json:"collected"` // sum of completed contributions
	Spent       int64     `json:"spent"`     // sum of from_pool expenses
}
