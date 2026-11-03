-- Expense last-editor, mirroring timeline_entries.last_modified_by: who last
-- touched the expense (set to the creator on insert, the editor on update).
ALTER TABLE expenses ADD COLUMN last_modified_by uuid REFERENCES users(id);

---- create above / drop below ----

ALTER TABLE expenses DROP COLUMN last_modified_by;
