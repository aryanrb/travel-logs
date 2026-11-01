-- Stage 3: RSVP invite links + the shared notifications table (emitted from
-- stages 3–7; read endpoints land in stage 8).

-- One shareable link per trip. Resetting changes `token` but keeps the row (and
-- therefore all replies).
CREATE TABLE invite_links (
    id         uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    trip_id    uuid NOT NULL UNIQUE REFERENCES trips(id) ON DELETE CASCADE,
    token      text NOT NULL UNIQUE,
    active     boolean NOT NULL DEFAULT true,
    join_mode  text NOT NULL DEFAULT 'anyone' CHECK (join_mode IN ('anyone', 'friends', 'approval')),
    spots      int CHECK (spots IS NULL OR spots > 0),
    reply_by   date,
    role       text NOT NULL DEFAULT 'viewer' CHECK (role IN ('collaborator', 'viewer')),
    message    text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- One reply per user per trip. `status` is the effective state; `chosen` is what
-- the person actually picked (needed to resolve an approval later).
CREATE TABLE rsvps (
    id         uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    trip_id    uuid NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status     text NOT NULL CHECK (status IN ('going', 'maybe', 'declined', 'waitlist', 'requested')),
    chosen     text NOT NULL CHECK (chosen IN ('going', 'maybe', 'declined')),
    created_at timestamptz NOT NULL DEFAULT now(),
    replied_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (trip_id, user_id)
);
CREATE INDEX rsvps_trip_idx ON rsvps(trip_id);

-- Notifications. actor_id is null for actor-less types (e.g. waitlist_promoted).
CREATE TABLE notifications (
    id          uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type        text NOT NULL,
    actor_id    uuid REFERENCES users(id) ON DELETE CASCADE,
    trip_id     uuid REFERENCES trips(id) ON DELETE CASCADE,
    comment_id  uuid,
    rsvp_status text,
    read        boolean NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX notifications_user_read_created_idx ON notifications(user_id, read, created_at DESC);

---- create above / drop below ----

DROP TABLE notifications;
DROP TABLE rsvps;
DROP TABLE invite_links;
