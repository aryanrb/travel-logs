-- A1: an activity can have many expenses; index the entry link (column from 003).
CREATE INDEX expenses_trip_entry_idx ON expenses(trip_id, entry_id);

-- F16: an expense may be recorded in another currency. amount stays in trip
-- currency and drives splits/balances; these three capture the original.
ALTER TABLE expenses ADD COLUMN original_currency text;
ALTER TABLE expenses ADD COLUMN original_amount   bigint;
ALTER TABLE expenses ADD COLUMN exchange_rate     numeric(18,6);

-- D10: trip budget (minor units) + per-category budgets.
ALTER TABLE trips ADD COLUMN budget_amount bigint;
CREATE TABLE trip_category_budgets (
    trip_id  uuid NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    category text NOT NULL,
    amount   bigint NOT NULL CHECK (amount > 0),
    PRIMARY KEY (trip_id, category)
);

-- G18: map coordinates (filled asynchronously by the geocoder).
ALTER TABLE trip_destinations ADD COLUMN lat double precision;
ALTER TABLE trip_destinations ADD COLUMN lng double precision;
ALTER TABLE trips ADD COLUMN start_lat double precision;
ALTER TABLE trips ADD COLUMN start_lng double precision;

-- E13: trips a user has bookmarked.
CREATE TABLE saved_trips (
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    trip_id    uuid NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, trip_id)
);

-- H21: activity feed. actor_name is denormalised so it survives a rename/removal.
CREATE TABLE trip_events (
    id           uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    trip_id      uuid NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    actor_id     uuid NOT NULL REFERENCES users(id),
    actor_name   text NOT NULL,
    action       text NOT NULL CHECK (action IN ('created', 'updated', 'deleted')),
    subject_type text NOT NULL CHECK (subject_type IN ('entry', 'journey', 'expense', 'photo', 'settlement', 'participant', 'trip')),
    subject_id   uuid,
    data         jsonb,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX trip_events_trip_created_idx ON trip_events(trip_id, created_at DESC);

---- create above / drop below ----

DROP TABLE trip_events;
DROP TABLE saved_trips;
ALTER TABLE trips DROP COLUMN start_lng;
ALTER TABLE trips DROP COLUMN start_lat;
ALTER TABLE trip_destinations DROP COLUMN lng;
ALTER TABLE trip_destinations DROP COLUMN lat;
DROP TABLE trip_category_budgets;
ALTER TABLE trips DROP COLUMN budget_amount;
ALTER TABLE expenses DROP COLUMN exchange_rate;
ALTER TABLE expenses DROP COLUMN original_amount;
ALTER TABLE expenses DROP COLUMN original_currency;
DROP INDEX expenses_trip_entry_idx;
