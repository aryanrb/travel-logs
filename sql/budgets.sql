-- name: InsertCategoryBudget :exec
INSERT INTO trip_category_budgets (trip_id, category, amount) VALUES (@trip_id, @category, @amount);

-- name: DeleteCategoryBudgets :exec
DELETE FROM trip_category_budgets WHERE trip_id = @trip_id;

-- name: ListCategoryBudgets :many
SELECT trip_id, category, amount FROM trip_category_budgets WHERE trip_id = @trip_id ORDER BY category;

-- name: ListCategoryBudgetsForTrips :many
SELECT trip_id, category, amount FROM trip_category_budgets
WHERE trip_id = ANY(@trip_ids::uuid[]) ORDER BY trip_id, category;
