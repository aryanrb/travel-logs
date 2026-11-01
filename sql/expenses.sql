-- name: InsertExpense :one
INSERT INTO expenses (
    trip_id, title, amount, currency, category, expense_date,
    paid_by_user_id, paid_by_external_id, recorded_by, split_method, notes, receipt_url, hide_from_public, entry_id,
    original_currency, original_amount, exchange_rate, from_pool
) VALUES (
    @trip_id, @title, @amount, @currency, @category, @expense_date,
    @paid_by_user_id, @paid_by_external_id, @recorded_by, @split_method, @notes, @receipt_url, @hide_from_public, @entry_id,
    @original_currency, @original_amount, @exchange_rate, @from_pool
)
RETURNING *;

-- name: GetExpense :one
SELECT * FROM expenses WHERE id = @id;

-- name: ListExpenses :many
SELECT * FROM expenses WHERE trip_id = @trip_id ORDER BY expense_date DESC, created_at DESC;

-- name: UpdateExpense :exec
UPDATE expenses SET
    title = @title,
    amount = @amount,
    currency = @currency,
    category = @category,
    expense_date = @expense_date,
    paid_by_user_id = @paid_by_user_id,
    paid_by_external_id = @paid_by_external_id,
    split_method = @split_method,
    notes = @notes,
    receipt_url = @receipt_url,
    hide_from_public = @hide_from_public,
    entry_id = @entry_id,
    original_currency = @original_currency,
    original_amount = @original_amount,
    exchange_rate = @exchange_rate,
    from_pool = @from_pool,
    updated_at = now()
WHERE id = @id;

-- name: DeleteExpense :exec
DELETE FROM expenses WHERE id = @id;

-- name: CategoryBreakdown :many
SELECT COALESCE(category, 'uncategorized') AS category, SUM(amount)::bigint AS total
FROM expenses
WHERE trip_id = @trip_id
GROUP BY category;

-- name: TotalExpenses :one
SELECT COALESCE(SUM(amount), 0)::bigint FROM expenses WHERE trip_id = @trip_id;

-- ─── Shares ───────────────────────────────────────────────────────────────
-- name: InsertShare :exec
INSERT INTO expense_shares (expense_id, user_id, external_id, amount)
VALUES (@expense_id, @user_id, @external_id, @amount);

-- name: DeleteSharesForExpense :exec
DELETE FROM expense_shares WHERE expense_id = @expense_id;

-- name: ListSharesForExpense :many
SELECT * FROM expense_shares WHERE expense_id = @expense_id;

-- name: ListTripShares :many
SELECT s.user_id, s.external_id, s.amount
FROM expense_shares s
JOIN expenses e ON e.id = s.expense_id
WHERE e.trip_id = @trip_id;
