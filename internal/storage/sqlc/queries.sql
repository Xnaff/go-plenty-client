-- Jobs

-- name: CreateJob :execlastid
INSERT INTO jobs (name, job_type, config, status)
VALUES (?, ?, ?, ?);

-- name: GetJob :one
SELECT id, name, job_type, config, status, created_at, updated_at
FROM jobs
WHERE id = ?;

-- name: UpdateJobStatus :exec
UPDATE jobs SET status = ? WHERE id = ?;

-- Pipeline Runs

-- name: CreatePipelineRun :execlastid
INSERT INTO pipeline_runs (job_id, status, current_stage)
VALUES (?, ?, ?);

-- name: GetPipelineRun :one
SELECT id, job_id, status, current_stage, started_at, completed_at, error_message, created_at, updated_at
FROM pipeline_runs
WHERE id = ?;

-- name: ListPipelineRunsByJob :many
SELECT id, job_id, status, current_stage, started_at, completed_at, error_message, created_at, updated_at
FROM pipeline_runs
WHERE job_id = ?
ORDER BY created_at DESC;

-- name: UpdatePipelineRunStatus :exec
UPDATE pipeline_runs
SET status = ?, current_stage = ?, error_message = ?, updated_at = NOW()
WHERE id = ?;

-- Stage States

-- name: CreateStageState :execlastid
INSERT INTO stage_states (run_id, stage_name, status)
VALUES (?, ?, ?);

-- name: GetStageState :one
SELECT id, run_id, stage_name, status, processed, total, error_detail, started_at, completed_at
FROM stage_states
WHERE run_id = ? AND stage_name = ?;

-- name: ListStageStatesByRun :many
SELECT id, run_id, stage_name, status, processed, total, error_detail, started_at, completed_at
FROM stage_states
WHERE run_id = ?
ORDER BY stage_name;

-- name: UpdateStageState :exec
UPDATE stage_states
SET status = ?, processed = ?, total = ?
WHERE id = ?;

-- Entity Mappings

-- name: CreateEntityMapping :execlastid
INSERT INTO entity_mappings (run_id, local_id, plenty_id, entity_type, stage, status, error_message)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: GetEntityMapping :one
SELECT id, run_id, local_id, plenty_id, entity_type, stage, status, error_message, created_at, updated_at
FROM entity_mappings
WHERE local_id = ? AND entity_type = ? AND run_id = ?;

-- name: GetEntityMappingByPlentyID :one
SELECT id, run_id, local_id, plenty_id, entity_type, stage, status, error_message, created_at, updated_at
FROM entity_mappings
WHERE plenty_id = ? AND entity_type = ?
LIMIT 1;

-- name: ListEntityMappingsByRun :many
SELECT id, run_id, local_id, plenty_id, entity_type, stage, status, error_message, created_at, updated_at
FROM entity_mappings
WHERE run_id = ? AND entity_type = ?
ORDER BY created_at;

-- name: ListFailedMappingsByRun :many
SELECT id, run_id, local_id, plenty_id, entity_type, stage, status, error_message, created_at, updated_at
FROM entity_mappings
WHERE run_id = ? AND status = 'failed'
ORDER BY created_at;

-- name: UpdateMappingStatus :exec
UPDATE entity_mappings
SET status = ?, plenty_id = ?, error_message = ?, updated_at = NOW()
WHERE id = ?;

-- name: CountMappingsByStatus :many
SELECT entity_type, status, COUNT(*) as count
FROM entity_mappings
WHERE run_id = ?
GROUP BY entity_type, status;

-- Products

-- name: CreateProduct :execlastid
INSERT INTO products (job_id, name, product_type, base_data, status)
VALUES (?, ?, ?, ?, ?);

-- name: GetProduct :one
SELECT id, job_id, name, product_type, base_data, status, created_at, updated_at
FROM products
WHERE id = ?;

-- name: ListProductsByJob :many
SELECT id, job_id, name, product_type, base_data, status, created_at, updated_at
FROM products
WHERE job_id = ?
ORDER BY created_at;

-- Categories

-- name: CreateCategory :execlastid
INSERT INTO categories (job_id, parent_id, name, level, sort_order, status)
VALUES (?, ?, ?, ?, ?, ?);

-- name: ListCategoriesByJob :many
SELECT id, job_id, parent_id, name, level, sort_order, status, created_at
FROM categories
WHERE job_id = ?
ORDER BY level, sort_order;

-- Variations

-- name: CreateVariation :execlastid
INSERT INTO variations (product_id, name, sku, price, currency, weight, weight_unit, barcode, status)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListVariationsByProduct :many
SELECT id, product_id, name, sku, price, currency, weight, weight_unit, barcode, status, created_at
FROM variations
WHERE product_id = ?
ORDER BY created_at;

-- Texts

-- name: CreateText :execlastid
INSERT INTO texts (product_id, field, lang, content, status)
VALUES (?, ?, ?, ?, ?);

-- name: ListTextsByProduct :many
SELECT id, product_id, field, lang, content, status, created_at, updated_at
FROM texts
WHERE product_id = ?
ORDER BY field, lang;

-- name: GetTextByProductFieldLang :one
SELECT id, product_id, field, lang, content, status, created_at, updated_at
FROM texts
WHERE product_id = ? AND field = ? AND lang = ?;

-- Images

-- name: CreateImage :execlastid
INSERT INTO images (product_id, source_url, local_path, position, source_type, attribution, status)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: ListImagesByProduct :many
SELECT id, product_id, source_url, local_path, position, source_type, attribution, status, created_at
FROM images
WHERE product_id = ?
ORDER BY position;

-- OAuth Tokens

-- name: UpsertOAuthToken :exec
INSERT INTO oauth_tokens (shop_url, access_token, refresh_token, token_type, expires_at)
VALUES (?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
    access_token = VALUES(access_token),
    refresh_token = VALUES(refresh_token),
    token_type = VALUES(token_type),
    expires_at = VALUES(expires_at);

-- name: GetOAuthToken :one
SELECT id, shop_url, access_token, refresh_token, token_type, expires_at, created_at, updated_at
FROM oauth_tokens
WHERE shop_url = ?;

-- name: GetMappingByLocalIDAndType :one
SELECT id, run_id, local_id, plenty_id, entity_type, stage, status, error_message, created_at, updated_at
FROM entity_mappings
WHERE run_id = ? AND local_id = ? AND entity_type = ?;

-- name: ListCreatedMappingsByRunAndType :many
SELECT id, run_id, local_id, plenty_id, entity_type, stage, status, error_message, created_at, updated_at
FROM entity_mappings
WHERE run_id = ? AND entity_type = ? AND status = 'created'
ORDER BY created_at;

-- name: ListOrphanedMappingsByRun :many
SELECT id, run_id, local_id, plenty_id, entity_type, stage, status, error_message, created_at, updated_at
FROM entity_mappings
WHERE run_id = ? AND status = 'orphaned'
ORDER BY created_at;

-- name: UpdateStageStateTimestamps :exec
UPDATE stage_states
SET status = ?, processed = ?, total = ?, started_at = ?, completed_at = ?
WHERE id = ?;

-- name: UpdatePipelineRunCompleted :exec
UPDATE pipeline_runs
SET status = ?, current_stage = ?, completed_at = NOW(), updated_at = NOW()
WHERE id = ?;

-- name: GetPipelineRunByJobLatest :one
SELECT id, job_id, status, current_stage, started_at, completed_at, error_message, created_at, updated_at
FROM pipeline_runs
WHERE job_id = ?
ORDER BY created_at DESC
LIMIT 1;

-- name: ListProductsByJobAndStatus :many
SELECT id, job_id, name, product_type, base_data, status, created_at, updated_at
FROM products
WHERE job_id = ? AND status = ?
ORDER BY created_at;

-- name: ListAttributesByJob :many
SELECT id, job_id, name, attr_type, status, created_at
FROM attributes
WHERE job_id = ?
ORDER BY created_at;

-- name: ListPropertiesByJob :many
SELECT id, job_id, name, property_type, status, created_at
FROM properties
WHERE job_id = ?
ORDER BY created_at;

-- name: ListAttributeValuesByAttribute :many
SELECT id, attribute_id, name, sort_order
FROM attribute_values
WHERE attribute_id = ?
ORDER BY sort_order;

-- name: CountFailedByRun :one
SELECT COUNT(*) as count
FROM entity_mappings
WHERE run_id = ? AND status = 'failed';

-- name: ResetFailedMappingsForRetry :exec
UPDATE entity_mappings
SET status = 'pending', error_message = NULL, updated_at = NOW()
WHERE run_id = ? AND status = 'failed';

-- name: DeleteOAuthToken :exec
DELETE FROM oauth_tokens
WHERE shop_url = ?;
