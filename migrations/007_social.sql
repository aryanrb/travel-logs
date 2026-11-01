-- Stage 4: people graph — follows (one-way), friendships (request/accept),
-- blocks, and a moderation report queue.

CREATE TABLE follows (
    follower_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    followee_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (follower_id, followee_id),
    CHECK (follower_id <> followee_id)
);
CREATE INDEX follows_followee_idx ON follows(followee_id);

-- One row per directed request; status flips to accepted when the addressee
-- accepts. Friendship between two people = any row between them with accepted.
CREATE TABLE friendships (
    requester_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    addressee_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status       text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'accepted')),
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (requester_id, addressee_id),
    CHECK (requester_id <> addressee_id)
);
CREATE INDEX friendships_addressee_idx ON friendships(addressee_id);

CREATE TABLE blocks (
    blocker_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    blocked_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (blocker_id, blocked_id),
    CHECK (blocker_id <> blocked_id)
);

CREATE TABLE reports (
    id           uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    reporter_id  uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    subject_type text NOT NULL CHECK (subject_type IN ('user', 'trip', 'comment')),
    subject_id   uuid NOT NULL,
    reason       text NOT NULL CHECK (reason IN ('spam', 'harassment', 'inappropriate', 'misleading', 'other')),
    details      text,
    status       text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'reviewed', 'actioned', 'dismissed')),
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX reports_status_idx ON reports(status, created_at);

---- create above / drop below ----

DROP TABLE reports;
DROP TABLE blocks;
DROP TABLE friendships;
DROP TABLE follows;
