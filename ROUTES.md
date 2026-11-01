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
| GET | `/v1/me` | user | Current user profile `{id, name, email, photo_url, bio}`. |
| PUT | `/v1/me` | user | Update profile. Body `{name, photo_url?, bio?}`. `photo_url` must be an `https://` URL; `""` clears it. `bio` ≤ 160 chars; `""` clears it. (PUT /me is full-replace: an omitted field is cleared.) |

`user` object: `{ id, name, email, photo_url, bio }`.

---

## Trips

| Method | Path | Role | Description |
|---|---|---|---|
| POST | `/v1/trips` | user | Create a trip. Returns the trip. Creator becomes owner. |
| GET | `/v1/trips` | user | List my trips: `{ owned: [trip], shared: [{trip, role}] }`. |
| GET | `/v1/trips/:id` | viewer+ | Trip detail `{ trip, destinations, your_role }`. |
| PUT | `/v1/trips/:id` | owner | Update trip fields. |
| DELETE | `/v1/trips/:id` | owner | Delete trip and ALL its content (confirm on the frontend). |
| POST | `/v1/trips/:id/publish` | owner | Body `{is_public, show_costs_public}`. |
| POST | `/v1/trips/:id/archive` | owner | Soft-archive (sets `archived_at`). |
| PUT | `/v1/trips/:id/budget` | owner | Replace the whole budget (one tx). Body `{budget_amount?, category_budgets: [{category, amount}]}`. Returns the trip. |
| GET | `/v1/trips/:id/activity` | viewer+ | Activity feed. Query `?before=<RFC3339>`. Returns `{events: [...]}`, newest first, max 50. |
| POST | `/v1/trips/:id/destinations` | owner | Add a destination. Body `{name, arrival_date?, sort_order?}`. |
| DELETE | `/v1/trips/:id/destinations/:destID` | owner | Remove a destination. |

Every **trip object** (owned/shared list, detail, and POST/PUT/budget responses)
includes `owner_name` (no email), `budget_amount` (or null), `category_budgets`
(`[]` when none), and `start_lat`/`start_lng`. Destinations include `lat`/`lng`.
Budget `category` must be one of: food, transport, accommodation, activities,
shopping, groceries, entertainment, health, fees, other. 422 for invalid amounts,
duplicate or unknown categories.

**Trip create/update body:**
```json
{
  "title": "Nepal 2026",
  "cover_image": "https://...",       // optional; must be https://, "" clears it
  "description": "...",               // optional
  "start_date": "2026-03-01",         // optional
  "end_date": "2026-03-10",           // optional
  "start_location": "Kathmandu",      // optional
  "trip_type": "trekking",            // optional
  "currency": "NPR",                  // optional, defaults to USD
  "start_lat": 27.7172,               // optional map pin (−90..90); set wins over name lookup
  "start_lng": 85.3240                // optional map pin (−180..180)
}
```
Destination body also accepts `lat`/`lng` (same ranges; a set pin skips the
name lookup).

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

## RSVP invite links

One shareable link per trip (alongside the email invitations above). People open
it and reply **Going / Maybe / Can't go**.

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/v1/trips/:id/invite-link` | owner | `{token, active, join_mode, spots, reply_by, role, message}`. **404** when the trip has no link yet. |
| PUT | `/v1/trips/:id/invite-link` | owner | Create or update. Body `{active, join_mode, spots\|null, reply_by\|null, role, message}`. Returns the link. |
| POST | `/v1/trips/:id/invite-link/reset` | owner | Rotate the token (old one stops working; replies kept). Returns the link. |
| GET | `/v1/trips/:id/rsvps` | owner | `{rsvps: [{id, user_id, name, photo_url, status, replied_at}]}`. |
| POST | `/v1/trips/:id/rsvps/:rsvpId/approve` | owner | Approve a `requested` reply → going (or waitlist if full) / maybe per what they chose. |
| DELETE | `/v1/trips/:id/rsvps/:rsvpId` | owner | Remove a reply; for a Going person also revokes access and promotes the waitlist. |
| GET | `/v1/join/:token` | public (optional auth) | Invitee view (see below). **404** when the token is unknown/turned off/reset. |
| PUT | `/v1/join/:token/rsvp` | user | Reply. Body `{status: going\|maybe\|declined}`. Returns `{status, trip_id}` — `status` is what actually happened (may be `waitlist` or `requested`). |

- **Going** → accepted participant with the link's `role` (counts against `spots`).
- **Maybe** → accepted viewer (read access, no spot, not in expense splits).
- **Can't go** → recorded only; changing away from Going revokes access.
- `join_mode`: `anyone`, `friends` (owner's friends only, else 403), `approval` (going/maybe become `requested` until approved).
- `spots` caps Going; overflow becomes `waitlist`; a freed spot auto-promotes the earliest waitlisted person (notified `waitlist_promoted`).
- Past `reply_by` → link closed to replies (403); `active:false` → link `404`s.
- Blocked people (either direction) can't reply.

`GET /join/:token` returns: `{trip:{id,title,cover_image,start_date,end_date,
start_location,trip_type,currency}, destinations:[name], organiser:{name,photo_url},
message, join_mode, spots, reply_by, closed, counts:{going,maybe}, going_people:
[{name,photo_url}] (≤8), planned_cost_per_person?, my_rsvp?}`. `planned_cost_per_person`
= the pool's per_person, else budget ÷ max(1, going); omitted when neither exists.
`my_rsvp` only when signed in and replied.

---

## People — profiles, follows, friends, blocks, reports

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/v1/users/:id/profile` | public (optional auth) | `{user:{id,name,photo_url,bio,created_at}, stats:{trips,followers,following,friends}, trips:[...explore items...], relationship}`. **404** if that user blocked you (or unknown). Never includes email. `stats.trips` = public trips only. |
| POST / DELETE | `/v1/users/:id/follow` | user | Follow / unfollow (idempotent). Can't follow yourself or someone blocked either way. New follow notifies them. |
| POST | `/v1/users/:id/friend` | user | Send a request, or **accept theirs** if they already asked. Notifies `friend_request` / `friend_accepted`. |
| DELETE | `/v1/users/:id/friend` | user | Cancel your request, decline theirs, or unfriend. |
| GET | `/v1/me/friends` | user | `{friends, incoming, outgoing}` — each `[{id,name,photo_url}]`. |
| POST / DELETE | `/v1/users/:id/block` | user | Block / unblock. Blocking ends any friendship, request and follows **both ways**. |
| GET | `/v1/me/blocked` | user | `{users: [{id,name,photo_url}]}`. |
| POST | `/v1/reports` | user | Body `{subject_type: user\|trip\|comment, subject_id, reason, details?}`. `reason` ∈ spam/harassment/inappropriate/misleading/other; `details` required when `other`. Stored in a moderation queue (reporter never revealed). |

`relationship` (null signed-out or viewing yourself): `{following, follows_you,
friend: none\|requested\|incoming\|friends, blocked}`. When **you** blocked them,
the profile returns with `blocked:true` and empty `trips` (so the UI can offer
Unblock). Blocks are enforced **both directions** across Explore, profiles, the
Following feed, invite replies, comments and notifications.

---

## Comments & questions (public trips)

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/v1/public/trips/:id/comments?cursor=` | public (optional auth) | `{comments: [{id, author:{id,name,photo_url}, body, created_at, edited_at, parent_id}], next_cursor}`. Oldest first, replies interleaved (group by `parent_id`). Hides comments by people you've blocked or who blocked you. |
| POST | `/v1/public/trips/:id/comments` | user | Body `{body, parent_id?}`. `body` trimmed, 1–1000 chars. `parent_id` must be a **top-level** comment on the same trip (replies are one level). Only on public trips; blocked people can't comment. Returns the comment. |
| PUT | `/v1/comments/:id` | author | Body `{body}`. Sets `edited_at`. |
| DELETE | `/v1/comments/:id` | author or trip owner | Deleting a top-level comment deletes its replies. |

A new top-level comment notifies the trip owner (`comment`); a reply notifies the
parent's author (`comment_reply`). Both carry `comment_id`; you're never notified
about your own comment.

---

## Notifications

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/v1/me/notifications?cursor=` | user | `{notifications: [...], unread, next_cursor}`, **newest first**. `unread` = total unread (not just this page). Poll-friendly (indexed on `(user_id, read, created_at)`). |
| POST | `/v1/me/notifications/read` | user | Body `{ids?: [...]}`. Marks those read, or **all** when `ids` is absent/empty. |

Each notification: `{id, type, actor: {id,name,photo_url} | null, trip: {id,title}
| null, comment_id, rsvp_status, created_at, read}`. Notifications from people you've
since blocked (either direction) are hidden from the list.

| `type` | Sent to | When |
|---|---|---|
| `rsvp` | organiser | someone replies (with `rsvp_status`) |
| `rsvp_request` | organiser | a reply needs approval |
| `waitlist_promoted` | the promoted person | a spot opened up (no actor) |
| `comment` | trip owner | new top-level comment |
| `comment_reply` | parent comment's author | a reply |
| `friend_request` | them | a friend request |
| `friend_accepted` | the requester | their request was accepted |
| `follow` | them | a new follower |
| `like` | trip owner | a like |
| `pool_contribution` | collector | a member says they've paid in (pending) |

Never fired for your own actions, or involving someone blocked either way. The web
app ignores unknown types, so new ones are safe to add.

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
  "location": "Patan",           // optional
  "lat": 27.6727,                // optional map pin (−90..90)
  "lng": 85.3250,                // optional map pin (−180..180)
  "expense": { ...expense body... } // optional: records an expense linked to this
                                    // entry in the same transaction (see Expenses)
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
  "notes": "...",                // optional
  "start_lat": 27.7172,          // optional map pins (−90..90 / −180..180)
  "start_lng": 85.3240,
  "destination_lat": 28.2096,
  "destination_lng": 83.9856
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
  "receipt_url": "https://...",   // optional
  "entry_id": "<uuid>"            // optional link to a timeline entry; must belong
                                  // to the trip. On PUT, omitting it clears the link.
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
| GET | `/v1/trips/:id/settlements` | viewer+ | List settlements (repayments & pool contributions). |
| POST | `/v1/trips/:id/settlements` | collaborator+ | Record a settlement. |
| POST | `/v1/trips/:id/settlements/:sid/confirm` | receiver or owner | Mark a `pending` settlement `completed`. |
| DELETE | `/v1/trips/:id/settlements/:sid` | owner, or collaborator who recorded it | Delete a settlement. |

**Settlement body:**
```json
{
  "from": { "user_id": "<uuid>" },      // who paid
  "to":   { "external_name": "Local Guide" }, // who received it
  "amount": 200000,                     // minor units
  "settled_date": "2026-03-05",
  "status": "completed",                // "completed" (default) | "pending"
  "kind": "repayment",                  // "repayment" (default) | "contribution" (to = pool collector)
  "notes": "..."
}
```
`kind` is returned on settlement responses. A **pending contribution** notifies
the collector (`pool_contribution`); only the receiver (collector) or trip owner
can confirm it.

### Trip pool (optional)

Off by default — trips without a pool behave exactly as before. Balances are
unchanged: a contribution is a settlement to the collector, and a pool expense is
paid by the collector.

| Method | Path | Role | Description |
|---|---|---|---|
| GET | `/v1/trips/:id/pool` | viewer+ | `{enabled, collector_id, per_person, due_date, collected, spent}`. **404** when there's no pool. `collected` = completed contributions; `spent` = `from_pool` expenses. |
| PUT | `/v1/trips/:id/pool` | owner | Create/update. Body `{collector_id, per_person, due_date\|null}`. `collector_id` must be a current member with an account. |
| DELETE | `/v1/trips/:id/pool` | owner | Turn the pool off (existing contributions/expenses are kept). |

Expenses accept `from_pool` (boolean) on create/update and return it; when `true`,
`paid_by` must be the collector.

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

## Public trips, saving & discovery

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/v1/public/trips` | public (optional auth) | Discover public trips. See the Explore section below. Returns `{trips: [...], next_cursor}`. |
| GET | `/v1/public/trips/:id` | public | Public trip detail (404 if not public). `{trip, destinations, people: [{name, role}], entries, transportation, photos, budget}`. People = owner + accepted collaborators. Never exposes emails, user ids, `expense_id`, receipts, payers, shares, settlements or balances. `transportation.cost_amount` only when `show_costs_public`. `budget` is null unless `show_costs_public`, else `{currency, total, travellers, by_category, planned}` (totals exclude hidden expenses; `planned` = the trip's `budget_amount`). |
| POST | `/v1/public/trips/:id/copy` | user | Copy a public trip into a new private trip you own. Body `{title, start_date}`. Shifts all dates; copies description/type/currency/start location/cover/destinations/entries/journeys. Does **not** copy photos, expenses, settlements, participants, external payers, `expense_id` or `cost_amount`. One tx. Returns the new trip. |
| PUT | `/v1/public/trips/:id/save` | user | Bookmark a public trip (idempotent; 404 if not public). |
| DELETE | `/v1/public/trips/:id/save` | user | Remove a bookmark (idempotent; 404 if not public). |
| POST | `/v1/public/trips/:id/like` | user | Like a public trip (idempotent; 404 if not public). Notifies the owner once (not for your own trip). |
| DELETE | `/v1/public/trips/:id/like` | user | Unlike (idempotent). |
| GET | `/v1/me/saved-trips` | user | Your saved trips, newest-saved first, public only. Same row shape as `/public/trips`. |

### Explore — `GET /v1/public/trips`

All query params are optional and combine with **AND**. Filtering, currency
conversion and sorting happen on the server so paging stays correct.

| Param | Meaning |
|---|---|
| `q` | Matches title, description, start location, destination names (case-insensitive). |
| `trip_type` | Exact match. |
| `destination` | Place name; matches start location or any destination (case-insensitive). |
| `min_days`, `max_days` | Whole days, **start and end both counted** (12–15 Nov = 4). Trips missing either date are excluded when either bound is set. |
| `min_cost_per_person`, `max_cost_per_person` | Minor units of `currency`. Trips with private costs are excluded whenever a cost bound is set. |
| `currency` | Sent only with a cost bound. Each trip's cost per person is converted into this currency before comparing (daily rates from open.er-api.com; approximate). |
| `sort` | `shortest`, `longest`, `soonest` (start date), `popular` (likes then saves). Default: newest first. Trips lacking the needed dates sort last. |
| `following` | `true` = only trips by people you follow (needs auth; ignored signed out). |
| `limit`, `cursor` | Paging (default `limit` 20, max 100). Response `next_cursor` is absent on the last page; pass it back as `cursor`. |

Each list item: `{ trip, destinations: [name], owner_name, cost_per_person,
cost_per_person_converted?, likes, liked_by_me, saves, saved_by_me }`.
- `cost_per_person`: total spent ÷ travellers (minor units of the trip currency), or `null` when the owner keeps costs private.
- `cost_per_person_converted`: `{amount, currency}`, only when a different `currency` was requested (approximate; label it "≈").
- `likes`/`liked_by_me`/`saves`/`saved_by_me`: see Likes & saves.

Public trip objects carry `owner_id` (list items and detail) so the web app can
link owner names to `/u/<owner_id>`. Public trips and destinations carry
`start_lat`/`start_lng` and `lat`/`lng`; public entries carry `lat`/`lng` and
public journeys carry `start_lat`/`start_lng`/`destination_lat`/`destination_lng`,
so the web app can draw route and day maps.

**Likes & saves.** Every explore/profile list item and `GET /public/trips/:id`
carry `likes`, `liked_by_me`, `saves`, `saved_by_me`. `liked_by_me`/`saved_by_me`
are `false` when signed out; both public read endpoints take optional auth to fill
them. `sort=popular` orders by likes then saves.

## Activity feed

Every create/update/delete writes an event in the **same transaction** as the
change. `GET /v1/trips/:id/activity` returns:
```json
{ "events": [ {
  "id": "<uuid>", "trip_id": "<uuid>",
  "actor_id": "<uuid>", "actor_name": "Asha",
  "action": "created",                // created | updated | deleted
  "subject_type": "expense",          // entry|journey|expense|photo|settlement|participant|trip
  "subject_id": "<uuid>",             // for participants: the member's user id
  "data": { "title": "Dinner", "amount": 200000, "currency": "NPR" },
  "created_at": "2026-03-02T12:00:00Z"
} ] }
```
`data.amount`/`currency` are present for expenses and settlements. Page with
`?before=<RFC3339>` (the `created_at` of the oldest event you have).

## Expenses in another currency

Expense bodies (including nested expenses) accept `original_currency`,
`original_amount` (minor units of that currency) and `exchange_rate`
(trip-currency units per 1 original unit). Send **all three or none**; `rate > 0`
and `original_currency` must differ from the trip currency (else 422). `amount`
stays in the trip currency and drives splits and balances. All three are returned
on every expense response.

## Nested expense on entries & journeys

`POST`/`PUT` on `/entries` and `/transportation` accept an optional `expense`
object (same shape/validation as the expense body). For entries the expense is
created with `entry_id` = the entry. For journeys the expense is created first and
the journey saved with `expense_id` = it (overriding any `expense_id` in the body).
All in one transaction — if anything fails, nothing is written.

## Map coordinates

Trips, destinations, entries and journeys accept optional map pins (`*_lat`/
`*_lng`, WGS84; validated and rejected with 422 if out of range). **A pin the
user sets always wins** — when a pin is present the server stores it and skips the
name lookup.

When no pin is given, destinations and trip start locations are geocoded in the
background via Nominatim (set `NOMINATIM_CONTACT` to enable; empty disables it).
Coordinates stay null until resolved; lookups are cached and rate-limited to
1/sec, and failures are ignored.

> **Known issue — name lookups return nothing (e.g. "Kathmandu", "Panch
> Pokhari"):** background geocoding is gated on `NOMINATIM_CONTACT`, which is
> **unset** in the deployed env, so `lookup()` short-circuits and *no* name is
> ever resolved. Set `NOMINATIM_CONTACT` to a contact email to enable it. Even
> then, vague local names like "Panch Pokhari" may not resolve at `limit=1`
> without a region qualifier — which is exactly why user-set pins now override
> the lookup.

---

## Phase 1 defaults chosen (confirm with client — PRD §10)

- Invitees must **register/log in** before accepting an invite (no guest mode).
- Collaborators edit **only their own** entries/expenses/photos/settlements; the
  owner edits anything.
- A settlement is recorded by **one** authorized participant (no two-party approval).
- Trip totals/splits are in **one currency per trip**; individual expenses may
  record an original foreign amount + rate for display (see above).
- **Publishing**, public discovery/detail, copy-as-template, and bookmarking are
  built (see the Public trips section). Follow, likes, comments, tips and
  moderation remain Phase 2.
- Trip delete is a **hard delete** (cascades). Archive is separate (`archived_at`).
- Photo/receipt storage is a **URL** the frontend provides; no upload endpoint yet.
