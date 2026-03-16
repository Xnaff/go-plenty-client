-- Add description column to category_translations.
ALTER TABLE category_translations ADD COLUMN description TEXT NOT NULL DEFAULT '';

-- Category registry for cross-job reuse of PlentyONE categories.
CREATE TABLE category_registry (
    id          BIGINT AUTO_INCREMENT PRIMARY KEY,
    name        VARCHAR(255) NOT NULL,
    parent_name VARCHAR(255) NOT NULL DEFAULT '',
    plenty_id   BIGINT NOT NULL,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY idx_name_parent (name, parent_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
