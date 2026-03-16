-- Property name translations (multilingual property names for PlentyONE).
CREATE TABLE property_name_translations (
    id          BIGINT AUTO_INCREMENT PRIMARY KEY,
    property_id BIGINT NOT NULL,
    lang        VARCHAR(5) NOT NULL,
    name        TEXT NOT NULL,
    UNIQUE KEY idx_prop_lang (property_id, lang),
    CONSTRAINT fk_pnt_property FOREIGN KEY (property_id) REFERENCES properties(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Property option translations (multilingual selection option names).
CREATE TABLE property_option_translations (
    id          BIGINT AUTO_INCREMENT PRIMARY KEY,
    property_id BIGINT NOT NULL,
    option_key  VARCHAR(255) NOT NULL,
    lang        VARCHAR(5) NOT NULL,
    name        TEXT NOT NULL,
    UNIQUE KEY idx_prop_opt_lang (property_id, option_key, lang),
    CONSTRAINT fk_pot_property FOREIGN KEY (property_id) REFERENCES properties(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Category translations (multilingual category names).
CREATE TABLE category_translations (
    id          BIGINT AUTO_INCREMENT PRIMARY KEY,
    category_id BIGINT NOT NULL,
    lang        VARCHAR(5) NOT NULL,
    name        TEXT NOT NULL,
    UNIQUE KEY idx_cat_lang (category_id, lang),
    CONSTRAINT fk_ct_category FOREIGN KEY (category_id) REFERENCES categories(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Property groups (one group per job, organizes properties).
CREATE TABLE property_groups (
    id         BIGINT AUTO_INCREMENT PRIMARY KEY,
    job_id     BIGINT NOT NULL,
    name       VARCHAR(255) NOT NULL,
    status     VARCHAR(20) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_job (job_id),
    CONSTRAINT fk_pg_job FOREIGN KEY (job_id) REFERENCES jobs(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Property group translations (multilingual group names).
CREATE TABLE property_group_translations (
    id                BIGINT AUTO_INCREMENT PRIMARY KEY,
    property_group_id BIGINT NOT NULL,
    lang              VARCHAR(5) NOT NULL,
    name              TEXT NOT NULL,
    UNIQUE KEY idx_pg_lang (property_group_id, lang),
    CONSTRAINT fk_pgt_group FOREIGN KEY (property_group_id) REFERENCES property_groups(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Link properties to their group.
ALTER TABLE properties ADD COLUMN property_group_id BIGINT NULL AFTER property_type,
    ADD CONSTRAINT fk_prop_group FOREIGN KEY (property_group_id) REFERENCES property_groups(id);
