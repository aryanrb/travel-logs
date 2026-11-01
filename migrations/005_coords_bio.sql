-- Stage 1: map pins on activities & journeys, and a user bio.
-- Trip and destination coords already exist (004). These add the rest.
ALTER TABLE timeline_entries ADD COLUMN lat double precision;
ALTER TABLE timeline_entries ADD COLUMN lng double precision;

ALTER TABLE transportation ADD COLUMN start_lat       double precision;
ALTER TABLE transportation ADD COLUMN start_lng       double precision;
ALTER TABLE transportation ADD COLUMN destination_lat double precision;
ALTER TABLE transportation ADD COLUMN destination_lng double precision;

ALTER TABLE users ADD COLUMN bio text;

---- create above / drop below ----

ALTER TABLE users DROP COLUMN bio;
ALTER TABLE transportation DROP COLUMN destination_lng;
ALTER TABLE transportation DROP COLUMN destination_lat;
ALTER TABLE transportation DROP COLUMN start_lng;
ALTER TABLE transportation DROP COLUMN start_lat;
ALTER TABLE timeline_entries DROP COLUMN lng;
ALTER TABLE timeline_entries DROP COLUMN lat;
