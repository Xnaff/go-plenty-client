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

-- name: GetCategory :one
SELECT id, job_id, parent_id, name, level, sort_order, status, created_at
FROM categories
WHERE id = ?;

-- Variations

-- name: CreateVariation :execlastid
INSERT INTO variations (product_id, name, sku, price, rrp, b2b_price, b2b_rrp, currency, weight, weight_unit, sales_unit, length_mm, width_mm, height_mm, barcode, model, status)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListVariationsByProduct :many
SELECT id, product_id, name, sku, price, currency, weight, weight_unit, barcode, status, created_at, sales_unit, rrp, length_mm, width_mm, height_mm, b2b_price, b2b_rrp, model
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

-- name: CreateProductCategory :exec
INSERT INTO product_categories (product_id, category_id)
VALUES (?, ?);

-- name: ListCategoryIDsByProduct :many
SELECT category_id FROM product_categories
WHERE product_id = ?
ORDER BY category_id;

-- name: ListRecentJobs :many
SELECT id, name, job_type, config, status, created_at, updated_at
FROM jobs
ORDER BY created_at DESC
LIMIT ?;

-- name: DeleteOAuthToken :exec
DELETE FROM oauth_tokens
WHERE shop_url = ?;

-- Quality Scores

-- name: CreateQualityScore :execlastid
INSERT INTO quality_scores (product_id, job_id, overall, text_score, image_score, data_score, pass, details)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetQualityScoreByProduct :one
SELECT id, product_id, job_id, overall, text_score, image_score, data_score, pass, details, created_at
FROM quality_scores
WHERE product_id = ?;

-- name: ListQualityScoresByJob :many
SELECT id, product_id, job_id, overall, text_score, image_score, data_score, pass, details, created_at
FROM quality_scores
WHERE job_id = ?
ORDER BY overall DESC;

-- name: ListFailedQualityScoresByJob :many
SELECT id, product_id, job_id, overall, text_score, image_score, data_score, pass, details, created_at
FROM quality_scores
WHERE job_id = ? AND pass = FALSE
ORDER BY overall ASC;

-- Enrichment Cache

-- name: GetEnrichmentCache :one
SELECT id, source, query_key, data, expires_at, created_at
FROM enrichment_cache
WHERE source = ? AND query_key = ? AND expires_at > NOW();

-- name: UpsertEnrichmentCache :exec
INSERT INTO enrichment_cache (source, query_key, data, expires_at)
VALUES (?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
    data = VALUES(data),
    expires_at = VALUES(expires_at);

-- name: DeleteExpiredEnrichmentCache :exec
DELETE FROM enrichment_cache
WHERE expires_at <= NOW();

-- Properties

-- name: CreateProperty :execlastid
INSERT INTO properties (job_id, name, property_type, status)
VALUES (?, ?, ?, ?);

-- name: GetPropertyByJobAndName :one
SELECT id, job_id, name, property_type, status, created_at
FROM properties
WHERE job_id = ? AND name = ?;

-- Variation Properties

-- name: CreateVariationProperty :execlastid
INSERT INTO variation_properties (variation_id, property_id, value_text, value_int, value_float)
VALUES (?, ?, ?, ?, ?);

-- name: ListVariationPropertiesByVariation :many
SELECT vp.id, vp.variation_id, vp.property_id, vp.value_text, vp.value_int, vp.value_float,
       p.name as property_name, p.property_type
FROM variation_properties vp
JOIN properties p ON p.id = vp.property_id
WHERE vp.variation_id = ?
ORDER BY p.name;

-- name: ListDistinctPropertyValues :many
SELECT DISTINCT vp.value_text
FROM variation_properties vp
WHERE vp.property_id = ? AND vp.value_text IS NOT NULL
ORDER BY vp.value_text;

-- Variations Without Properties (for backfill)

-- name: ListVariationsWithoutPropertiesByJob :many
SELECT v.id as variation_id, v.product_id, v.name as variation_name,
       p.name as product_name, p.product_type
FROM variations v
JOIN products p ON v.product_id = p.id
WHERE p.job_id = ?
AND NOT EXISTS (SELECT 1 FROM variation_properties vp WHERE vp.variation_id = v.id)
ORDER BY v.id;

-- Property Name Translations

-- name: CreatePropertyNameTranslation :execlastid
INSERT INTO property_name_translations (property_id, lang, name)
VALUES (?, ?, ?);

-- name: ListPropertyNameTranslations :many
SELECT id, property_id, lang, name
FROM property_name_translations
WHERE property_id = ?
ORDER BY lang;

-- Property Option Translations

-- name: CreatePropertyOptionTranslation :execlastid
INSERT INTO property_option_translations (property_id, option_key, lang, name)
VALUES (?, ?, ?, ?);

-- name: ListPropertyOptionTranslations :many
SELECT id, property_id, option_key, lang, name
FROM property_option_translations
WHERE property_id = ? AND option_key = ?
ORDER BY lang;

-- name: ListAllPropertyOptionTranslations :many
SELECT id, property_id, option_key, lang, name
FROM property_option_translations
WHERE property_id = ?
ORDER BY option_key, lang;

-- Category Translations

-- name: CreateCategoryTranslation :execlastid
INSERT INTO category_translations (category_id, lang, name, description)
VALUES (?, ?, ?, ?);

-- name: ListCategoryTranslations :many
SELECT id, category_id, lang, name, description
FROM category_translations
WHERE category_id = ?
ORDER BY lang;

-- Category Registry (cross-job reuse)

-- name: LookupCategoryRegistry :one
SELECT id, name, parent_name, plenty_id, created_at
FROM category_registry
WHERE name = ? AND parent_name = ?;

-- name: RegisterCategory :execlastid
INSERT INTO category_registry (name, parent_name, plenty_id)
VALUES (?, ?, ?);

-- name: GetCategoryByJobAndName :one
SELECT id, job_id, parent_id, name, level, sort_order, status, created_at
FROM categories
WHERE job_id = ? AND name = ?
LIMIT 1;

-- Property Groups

-- name: CreatePropertyGroup :execlastid
INSERT INTO property_groups (job_id, name, status)
VALUES (?, ?, ?);

-- name: GetPropertyGroupByJob :one
SELECT id, job_id, name, status, created_at
FROM property_groups
WHERE job_id = ?
LIMIT 1;

-- name: UpdatePropertyGroupID :exec
UPDATE properties SET property_group_id = ? WHERE id = ?;

-- Property Group Translations

-- name: CreatePropertyGroupTranslation :execlastid
INSERT INTO property_group_translations (property_group_id, lang, name)
VALUES (?, ?, ?);

-- name: ListPropertyGroupTranslations :many
SELECT id, property_group_id, lang, name
FROM property_group_translations
WHERE property_group_id = ?
ORDER BY lang;

-- Graduated Prices

-- name: CreateGraduatedPrice :execlastid
INSERT INTO variation_graduated_prices (variation_id, minimum_quantity, price, rrp)
VALUES (?, ?, ?, ?);

-- name: ListGraduatedPricesByVariation :many
SELECT id, variation_id, minimum_quantity, price, rrp
FROM variation_graduated_prices
WHERE variation_id = ?
ORDER BY minimum_quantity;

-- Attributes (CRUD)

-- name: CreateAttribute :execlastid
INSERT INTO attributes (job_id, name, attr_type, status)
VALUES (?, ?, ?, ?);

-- name: GetAttributeByJobAndName :one
SELECT id, job_id, name, attr_type, status, created_at
FROM attributes
WHERE job_id = ? AND name = ?
LIMIT 1;

-- name: CreateAttributeValue :execlastid
INSERT INTO attribute_values (attribute_id, name, sort_order)
VALUES (?, ?, ?);

-- name: GetAttributeValueByAttrAndName :one
SELECT id, attribute_id, name, sort_order
FROM attribute_values
WHERE attribute_id = ? AND name = ?
LIMIT 1;

-- Variation-Attribute Links

-- name: CreateVariationAttribute :exec
INSERT INTO variation_attributes (variation_id, attribute_id, attribute_value_id)
VALUES (?, ?, ?);

-- name: ListVariationAttributesByVariation :many
SELECT variation_id, attribute_id, attribute_value_id
FROM variation_attributes
WHERE variation_id = ?
ORDER BY attribute_id;

-- Attribute Name Translations

-- name: CreateAttributeNameTranslation :execlastid
INSERT INTO attribute_name_translations (attribute_id, lang, name)
VALUES (?, ?, ?);

-- name: ListAttributeNameTranslations :many
SELECT id, attribute_id, lang, name
FROM attribute_name_translations
WHERE attribute_id = ?
ORDER BY lang;

-- Attribute Value Translations

-- name: CreateAttributeValueTranslation :execlastid
INSERT INTO attribute_value_translations (attribute_value_id, lang, name)
VALUES (?, ?, ?);

-- name: ListAttributeValueTranslations :many
SELECT id, attribute_value_id, lang, name
FROM attribute_value_translations
WHERE attribute_value_id = ?
ORDER BY lang;
