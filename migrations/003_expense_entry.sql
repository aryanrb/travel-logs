ALTER TABLE expenses ADD COLUMN entry_id uuid REFERENCES timeline_entries(id) ON DELETE SET NULL;

---- create above / drop below ----

ALTER TABLE expenses DROP COLUMN entry_id;
