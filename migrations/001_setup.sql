CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ─── Users ──────────────────────────────────────────────────────────────────
CREATE TABLE users (
    id            uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    email         text NOT NULL UNIQUE,
    password_hash text NOT NULL,
    name          text NOT NULL,
    photo_url     text,
    created_at    timestamptz NOT NULL DEFAULT now()
);

-- ─── Trips ──────────────────────────────────────────────────────────────────
CREATE TABLE trips (
    id             uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    owner_id       uuid NOT NULL REFERENCES users(id),
    title          text NOT NULL,
    cover_image    text,
    description    text,
    start_date     date,
    end_date       date,
    start_location text,
    trip_type      text,
    currency       text NOT NULL DEFAULT 'USD',
    is_public      boolean NOT NULL DEFAULT false,
    archived_at    timestamptz,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX trips_owner_idx ON trips(owner_id);

CREATE TABLE trip_destinations (
    id           uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    trip_id      uuid NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    name         text NOT NULL,
    arrival_date date,
    sort_order   int NOT NULL DEFAULT 0
);
CREATE INDEX trip_destinations_trip_idx ON trip_destinations(trip_id);

-- ─── Participants & invitations ─────────────────────────────────────────────
-- Owner lives in trips.owner_id. This table holds collaborators & viewers,
-- including pending email invites (user_id null until the invitee registers).
CREATE TABLE trip_participants (
    id            uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    trip_id       uuid NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    user_id       uuid REFERENCES users(id),
    invited_email text,
    role          text NOT NULL CHECK (role IN ('collaborator', 'viewer')),
    status        text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'accepted', 'revoked')),
    invite_token  text UNIQUE,
    created_at    timestamptz NOT NULL DEFAULT now(),
    accepted_at   timestamptz
);
CREATE UNIQUE INDEX trip_participants_trip_user_uniq
    ON trip_participants(trip_id, user_id) WHERE user_id IS NOT NULL;
CREATE INDEX trip_participants_user_idx ON trip_participants(user_id);

-- ─── Timeline ───────────────────────────────────────────────────────────────
CREATE TABLE timeline_entries (
    id               uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    trip_id          uuid NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    title            text NOT NULL,
    description      text,
    entry_date       date NOT NULL,
    entry_time       text,
    location         text,
    created_by       uuid NOT NULL REFERENCES users(id),
    last_modified_by uuid REFERENCES users(id),
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX timeline_trip_date_idx ON timeline_entries(trip_id, entry_date, entry_time);

CREATE TABLE photos (
    id         uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    trip_id    uuid NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    entry_id   uuid REFERENCES timeline_entries(id) ON DELETE CASCADE,
    url        text NOT NULL,
    caption    text,
    taken_date date,
    created_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX photos_trip_idx ON photos(trip_id);
CREATE INDEX photos_entry_idx ON photos(entry_id);

-- ─── Expenses ───────────────────────────────────────────────────────────────
-- An external payer is someone without an account who paid for the group.
-- Reusable per trip (unique name) so expenses don't duplicate the record.
CREATE TABLE external_payers (
    id         uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    trip_id    uuid NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    name       text NOT NULL,
    contact    text,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (trip_id, name)
);

-- Money is stored as integer minor units (e.g. cents/paisa) so allocated
-- shares always reconcile to the total exactly (core business rule 10).
-- Payer is a user XOR an external person; recorder is always a user.
CREATE TABLE expenses (
    id                  uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    trip_id             uuid NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    title               text NOT NULL,
    amount              bigint NOT NULL CHECK (amount > 0),
    currency            text NOT NULL,
    category            text,
    expense_date        date NOT NULL,
    paid_by_user_id     uuid REFERENCES users(id),
    paid_by_external_id uuid REFERENCES external_payers(id),
    recorded_by         uuid NOT NULL REFERENCES users(id),
    split_method        text NOT NULL CHECK (split_method IN ('equal', 'custom')),
    notes               text,
    receipt_url         text,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT expense_one_payer CHECK (
        (paid_by_user_id IS NOT NULL) <> (paid_by_external_id IS NOT NULL)
    )
);
CREATE INDEX expenses_trip_idx ON expenses(trip_id);

-- Each row = one party's allocated share of an expense. Party is user XOR external.
CREATE TABLE expense_shares (
    id          uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    expense_id  uuid NOT NULL REFERENCES expenses(id) ON DELETE CASCADE,
    user_id     uuid REFERENCES users(id),
    external_id uuid REFERENCES external_payers(id),
    amount      bigint NOT NULL CHECK (amount >= 0),
    CONSTRAINT share_one_party CHECK (
        (user_id IS NOT NULL) <> (external_id IS NOT NULL)
    )
);
CREATE INDEX expense_shares_expense_idx ON expense_shares(expense_id);

-- Manual repayments. Recording an expense is NOT a repayment; balances only
-- move when a settlement row exists (core business rule 11).
CREATE TABLE settlements (
    id               uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    trip_id          uuid NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    from_user_id     uuid REFERENCES users(id),
    from_external_id uuid REFERENCES external_payers(id),
    to_user_id       uuid REFERENCES users(id),
    to_external_id   uuid REFERENCES external_payers(id),
    amount           bigint NOT NULL CHECK (amount > 0),
    settled_date     date NOT NULL,
    status           text NOT NULL DEFAULT 'completed' CHECK (status IN ('completed', 'pending')),
    notes            text,
    recorded_by      uuid NOT NULL REFERENCES users(id),
    created_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT settle_one_from CHECK ((from_user_id IS NOT NULL) <> (from_external_id IS NOT NULL)),
    CONSTRAINT settle_one_to CHECK ((to_user_id IS NOT NULL) <> (to_external_id IS NOT NULL))
);
CREATE INDEX settlements_trip_idx ON settlements(trip_id);

-- ─── Transportation ─────────────────────────────────────────────────────────
-- May stand alone or attach to a timeline entry. If the cost is also tracked as
-- an expense, link expense_id so summaries never double-count (req 3.4).
CREATE TABLE transportation (
    id           uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    trip_id      uuid NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    entry_id     uuid REFERENCES timeline_entries(id) ON DELETE SET NULL,
    expense_id   uuid REFERENCES expenses(id) ON DELETE SET NULL,
    type         text NOT NULL,
    start_point  text,
    destination  text,
    depart_at    timestamptz,
    distance_km  double precision,
    duration_min int,
    cost_amount  bigint,
    notes        text,
    created_by   uuid NOT NULL REFERENCES users(id),
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX transportation_trip_idx ON transportation(trip_id);

---- create above / drop below ----

DROP TABLE transportation;
DROP TABLE settlements;
DROP TABLE expense_shares;
DROP TABLE expenses;
DROP TABLE external_payers;
DROP TABLE photos;
DROP TABLE timeline_entries;
DROP TABLE trip_participants;
DROP TABLE trip_destinations;
DROP TABLE trips;
DROP TABLE users;
