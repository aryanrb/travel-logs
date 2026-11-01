-- Stage 7: comments & questions on public trips. Replies are one level deep
-- (parent_id points at a top-level comment); deleting a parent cascades replies.
CREATE TABLE comments (
    id         uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    trip_id    uuid NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    author_id  uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    parent_id  uuid REFERENCES comments(id) ON DELETE CASCADE,
    body       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    edited_at  timestamptz
);
CREATE INDEX comments_trip_created_idx ON comments(trip_id, created_at);

---- create above / drop below ----

DROP TABLE comments;
