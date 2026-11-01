-- Stage 5: optional trip pool. Balances are unchanged — a contribution is a
-- settlement to the collector, and a pool expense is paid by the collector, so
-- the existing balance formula already produces the right refunds/top-ups.

CREATE TABLE trip_pools (
    trip_id      uuid PRIMARY KEY REFERENCES trips(id) ON DELETE CASCADE,
    collector_id uuid NOT NULL REFERENCES users(id),
    per_person   bigint NOT NULL CHECK (per_person > 0),
    due_date     date,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

-- A settlement is a repayment (default) or a contribution into the pool.
ALTER TABLE settlements ADD COLUMN kind text NOT NULL DEFAULT 'repayment'
    CHECK (kind IN ('repayment', 'contribution'));

-- Spending from the pool: an expense paid by the collector, flagged here.
ALTER TABLE expenses ADD COLUMN from_pool boolean NOT NULL DEFAULT false;

---- create above / drop below ----

ALTER TABLE expenses DROP COLUMN from_pool;
ALTER TABLE settlements DROP COLUMN kind;
DROP TABLE trip_pools;
