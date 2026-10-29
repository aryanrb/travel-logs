ALTER TABLE trips ADD COLUMN show_costs_public boolean NOT NULL DEFAULT false;
ALTER TABLE expenses ADD COLUMN hide_from_public boolean NOT NULL DEFAULT false;

---- create above / drop below ----

ALTER TABLE expenses DROP COLUMN hide_from_public;
ALTER TABLE trips DROP COLUMN show_costs_public;
