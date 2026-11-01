-- Stage 6: likes on public trips (saves already exist via saved_trips).
CREATE TABLE trip_likes (
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    trip_id    uuid NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, trip_id)
);
CREATE INDEX trip_likes_trip_idx ON trip_likes(trip_id);

---- create above / drop below ----

DROP TABLE trip_likes;
