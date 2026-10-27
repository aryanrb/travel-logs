# Travel Service — API Reference (Phase 1)

Backend for the Travel Logging platform. Go + Echo + Postgres. This document is
the contract for the frontend.

## Conventions

- **Base path:** `/v1` (e.g. `http://localhost:8080/v1/trips`).
- **Auth:** send `Authorization: Bearer <token>` on every route except
  `POST /v1/auth/register` and `POST /v1/auth/login`. The token comes back from
  register/login and lasts 7 days.
- **Success envelope:** `{ "success": true, "payload": <data> }`.
- **Error envelope:** `{ "success": false, "message": "<reason>" }` with an HTTP
  status: `400` bad input, `401` not logged in / bad token, `403` wrong role,
  `404` not found, `409` duplicate, `422` validation, `500` server.
- **Money:** every amount (`amount`, `cost_amount`, share amounts, settlement
  amount) is an **integer in minor units** — cents/paisa. `NPR 1,500.00` → `150000`.
  Never send decimals. Splits always reconcile exactly to the total.
- **Dates:** `"YYYY-MM-DD"` (e.g. `"2026-10-27"`). Timestamps (`depart_at`) are
  RFC3339 (`"2026-10-27T14:30:00Z"`). Time-of-day on entries is a plain string
  `"HH:MM"`.
- **Roles:** `owner` (creator, full control), `collaborator` (adds/edits own
  content & expenses), `viewer` (read-only). Enforced on the backend.
- **Party reference** (payer / share / settlement side) — send **exactly one**
  identity:
  ```json
  { "user_id": "<uuid>" }                                  // a platform user
  { "external_id": "<uuid>" }                              // existing external payer
  { "external_name": "Local Guide", "external_contact": "+977..." }  // new/reused external payer (payers & settlements only)
  ```

---

## Auth & profile

| Method | Path | Auth | Description |
|---|---|---|---|
| POST | `/v1/auth/register` | public | Create account. Body `{email, password, name}` (password ≥ 8). Returns `{token, user}`. |
| POST | `/v1/auth/login` | public | Log in. Body `{email, password}`. Returns `{token, user}`. |
| GET | `/v1/me` | user | Current user profile `{id, name, email, photo_url}`. |
| PUT | `/v1/me` | user | Update profile. Body `{name, photo_url?}`. |

`user` object: `{ id, name, email, photo_url }`.

---

## Trips

| Method | Path | Role | Description |
|---|---|---|---|
| POST | `/v1/trips` | user | Create a trip. Returns the trip. Creator becomes owner. |
| GET | `/v1/trips` | user | List my trips: `{ owned: [trip], shared: [{trip, role}] }`. |
| GET | `/v1/trips/:id` | viewer+ | Trip detail `{ trip, destinations, your_role }`. |
| PUT | `/v1/trips/:id` | owner | Update trip fields. |
| DELETE | `/v1/trips/:id` | owner | Delete trip and ALL its content (confirm on the frontend). |
| POST | `/v1/trips/:id/publish` | owner | Body `{is_public: bool}`. Toggles public flag (Phase 2 surface). |
| POST | `/v1/trips/:id/archive` | owner | Soft-archive (sets `archived_at`). |
| POST | `/v1/trips/:id/destinations` | owner | Add a destination. Body `{name, arrival_date?, sort_order?}`. |
| DELETE | `/v1/trips/:id/destinations/:destID` | owner | Remove a destination. |

**Trip create/update body:**
```json
{
  "title": "Nepal 2026",
  "cover_image": "https://...",       // optional
  "description": "...",               // optional
  "start_date": "2026-03-01",         // optional
  "end_date": "2026-03-10",           // optional
  "start_location": "Kathmandu",      // optional
  "trip_type": "trekking",            // optional
  "currency": "NPR"                   // optional, defaults to USD
}
```

---

## Participants & invitations

| Method | Path | Role | Description |
|---|---|---|---|
| POST | `/v1/trips/:id/participants` | owner | Invite. Body `{role, user_id?, invited_email?}` — exactly one of user_id/invited_email. `role` ∈ `collaborator`/`viewer`. Returns pending participant with `invite_token`. |
| GET | `/v1/trips/:id/participants` | viewer+ | List participants with name/email/role/status. |
| PUT | `/v1/trips/:id/participants/:pid/role` | owner | Change role. Body `{role}`. |
| DELETE | `/v1/trips/:id/participants/:pid` | owner | Revoke access (status → `revoked`; history kept). |
| POST | `/v1/invites/accept` | user | Accept an invite. Body `{token}`. Invitee must be logged in. |

Status values: `pending`, `accepted`, `revoked`. Access is granted only once
`accepted`. An invite addressed to a specific `user_id` can only be accepted by
that user.

---

## Timeline entries

| Method | Path | Role | Description |
|---|---|---|---|
| POST | `/v1/trips/:id/entries` | collaborator+ | Create a day entry. |
| GET | `/v1/trips/:id/entries` | viewer+ | List entries (chronological). |
| PUT | `/v1/trips/:id/entries/:eid` | owner, or collaborator who created it | Update entry. |
| DELETE | `/v1/trips/:id/entries/:eid` | owner, or collaborator who created it | Delete entry (attached entry-photos cascade). |

**Entry body:**
```json
{
  "title": "Visited Durbar Square",
  "description": "...",          // optional
  "entry_date": "2026-03-02",    // required
  "entry_time": "14:30",         // optional "HH:MM"
  "location": "Patan"            // optional
}
```
Each entry records `created_by` and `last_modified_by`.

---

## Photos

| Method | Path | Role | Description |
|---|---|---|---|
| POST | `/v1/trips/:id/photos` | collaborator+ | Add a photo (URL). Body `{url, entry_id?, caption?, taken_date?}`. If `entry_id` set it must belong to the trip. |
| GET | `/v1/trips/:id/photos` | viewer+ | Trip gallery. |
| DELETE | `/v1/trips/:id/photos/:pid` | owner, or collaborator who added it | Delete a photo. |

> Upload itself is out of scope — the API stores a URL. Wire your storage/CDN on
> the frontend (or add an upload endpoint later) and pass the resulting URL.

---

## Transportation

| Method | Path | Role | Description |
|---|---|---|---|
| POST | `/v1/trips/:id/transportation` | collaborator+ | Record a journey. |
| GET | `/v1/trips/:id/transportation` | viewer+ | List journeys. |
| PUT | `/v1/trips/:id/transportation/:tid` | owner, or collaborator who created it | Update. |
| DELETE | `/v1/trips/:id/transportation/:tid` | owner, or collaborator who created it | Delete. |

**Transport body:**
```json
{
  "type": "flight",              // required: walking/bicycle/car/taxi/bus/train/flight/boat/other...
  "start_point": "KTM",          // optional
  "destination": "PKR",          // optional
  "depart_at": "2026-03-02T09:00:00Z", // optional RFC3339
  "distance_km": 200.5,          // optional
  "duration_min": 35,            // optional
  "cost_amount": 500000,         // optional, minor units
  "entry_id": "<uuid>",          // optional link to a timeline entry
  "expense_id": "<uuid>",        // optional link to an expense (avoids double-counting)
  "notes": "..."                 // optional
}
```
If a journey's cost is tracked as an expense, create the expense and set
`expense_id` here — summaries count expenses only, so nothing is double-counted.

---

## Expenses

The core rule: **paid_by**, **recorded_by**, and **shared_among** are three
separate things. The recorder (current user) must be owner/collaborator; the
payer can be anyone (incl. a viewer or a non-member external person); only the
parties you list in `shared_among` owe a share.

| Method | Path | Role | Description |
|---|---|---|---|
| POST | `/v1/trips/:id/expenses` | collaborator+ | Record an expense. Returns `{expense, paid_by, shares}`. |
| GET | `/v1/trips/:id/expenses` | viewer+ | List expenses. |
| GET | `/v1/trips/:id/expenses/:eid` | viewer+ | One expense with resolved payer + shares. |
| PUT | `/v1/trips/:id/expenses/:eid` | owner, or collaborator who recorded it | Update (recomputes shares). |
| DELETE | `/v1/trips/:id/expenses/:eid` | owner, or collaborator who recorded it | Delete. |
| GET | `/v1/trips/:id/external-payers` | viewer+ | List external payers for this trip. |
| POST | `/v1/trips/:id/external-payers` | collaborator+ | Create/reuse an external payer. Body `{name, contact?}`. |

**Expense body:**
```json
{
  "title": "Lunch",
  "amount": 200000,               // minor units, required > 0
  "currency": "NPR",              // optional, defaults to trip currency
  "category": "food",             // optional
  "date": "2026-03-02",           // required
  "paid_by": { "external_name": "Local Guide" },   // a party reference
  "split_method": "equal",        // "equal" | "custom"
  "shared_among": [
    { "user_id": "<uuid>" },
    { "user_id": "<uuid>" },
    { "user_id": "<uuid>" },
    { "user_id": "<uuid>" }
  ],
  "notes": "...",                 // optional
  "receipt_url": "https://..."    // optional
}
```

Split rules:
- **equal** — `amount` is divided evenly; any odd minor units go to the first
  parties so the shares always sum to `amount`. (`shared_among` amounts ignored.)
- **custom** — give each share **either** an `amount` (all shares; must sum to
  `amount`) **or** a `percent` (all shares; must sum to 100, converted to exact
  minor units). Don't mix the two.
- The payer is included in the split only if you add them to `shared_among`.

**Response shape** (`POST`/`GET`/`PUT` single):
```json
{
  "expense": { ...expense row... },
  "paid_by": { "type": "external", "id": "<uuid>", "name": "Local Guide" },
  "shares": [ { "party": {"type":"user","id":"<uuid>","name":"Asha"}, "amount": 50000 } ]
}
```

---

## Settlements & balances

| Method | Path | Role | Description |
|---|---|---|---|
| GET | `/v1/trips/:id/balances` | viewer+ | Net balance per party. |
| GET | `/v1/trips/:id/settlements` | viewer+ | List recorded repayments. |
| POST | `/v1/trips/:id/settlements` | collaborator+ | Record a repayment. |
| DELETE | `/v1/trips/:id/settlements/:sid` | owner, or collaborator who recorded it | Delete a settlement. |

**Settlement body:**
```json
{
  "from": { "user_id": "<uuid>" },      // who paid the repayment
  "to":   { "external_name": "Local Guide" }, // who received it
  "amount": 200000,                     // minor units
  "settled_date": "2026-03-05",
  "status": "completed",                // "completed" (default) | "pending"
  "notes": "..."
}
```

**Balances response:**
```json
{
  "currency": "NPR",
  "balances": [
    {
      "party": { "type": "user", "id": "<uuid>", "name": "Asha" },
      "total_paid": 200000,
      "total_share": 50000,
      "settled_paid": 0,
      "settled_received": 0,
      "net": 150000     // + should receive, - owes, 0 settled
    }
  ]
}
```
`net = total_paid − total_share + settled_paid − settled_received`. Only
`completed` settlements move balances; a `pending` one does not. External payers
appear here too (e.g. a guide owed a reimbursement).

---

## Trip summary

| Method | Path | Role | Description |
|---|---|---|---|
| GET | `/v1/trips/:id/summary` | viewer+ | Aggregate stats for the trip. |

**Response:**
```json
{
  "currency": "NPR",
  "start_date": "2026-03-01",
  "end_date": "2026-03-10",
  "travel_days": 10,
  "destinations": [ ... ],
  "timeline_entries": 12,
  "photos": 40,
  "total_expenses": 1200000,
  "category_breakdown": [ { "category": "food", "total": 400000 } ],
  "transportation": [ { "type": "flight", "count": 2, "total_distance_km": 410 } ],
  "balances": [ ...same shape as /balances... ]
}
```

---

## Phase 1 defaults chosen (confirm with client — PRD §10)

- Invitees must **register/log in** before accepting an invite (no guest mode).
- Collaborators edit **only their own** entries/expenses/photos/settlements; the
  owner edits anything.
- A settlement is recorded by **one** authorized participant (no two-party approval).
- **One currency per trip** (no multi-currency conversion).
- **Publishing** is just an `is_public` flag + toggle; the public read API and
  the rest of the community features (discovery, follow, bookmark, likes,
  comments, tips, moderation) are **Phase 2 — not built yet**.
- Trip delete is a **hard delete** (cascades). Archive is separate (`archived_at`).
- Photo/receipt storage is a **URL** the frontend provides; no upload endpoint yet.
