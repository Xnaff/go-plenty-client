-- Jobs track top-level generation/push requests
CREATE TABLE jobs (
    id          BIGINT AUTO_INCREMENT PRIMARY KEY,
    name        VARCHAR(255) NOT NULL DEFAULT '',
    job_type    VARCHAR(50) NOT NULL,
    config      JSON NOT NULL,
    status      VARCHAR(20) NOT NULL DEFAULT 'pending',
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_status (status),
    INDEX idx_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Pipeline runs track execution of the 6-stage pipeline for a job
CREATE TABLE pipeline_runs (
    id            BIGINT AUTO_INCREMENT PRIMARY KEY,
    job_id        BIGINT NOT NULL,
    status        VARCHAR(20) NOT NULL DEFAULT 'pending',
    current_stage VARCHAR(50),
    started_at    TIMESTAMP NULL,
    completed_at  TIMESTAMP NULL,
    error_message TEXT,
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_job (job_id),
    INDEX idx_status (status),
    CONSTRAINT fk_runs_job FOREIGN KEY (job_id) REFERENCES jobs(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Stage states track per-stage progress within a pipeline run
CREATE TABLE stage_states (
    id            BIGINT AUTO_INCREMENT PRIMARY KEY,
    run_id        BIGINT NOT NULL,
    stage_name    VARCHAR(50) NOT NULL,
    status        VARCHAR(20) NOT NULL DEFAULT 'pending',
    processed     INT NOT NULL DEFAULT 0,
    total         INT NOT NULL DEFAULT 0,
    error_detail  TEXT,
    started_at    TIMESTAMP NULL,
    completed_at  TIMESTAMP NULL,
    UNIQUE KEY idx_run_stage (run_id, stage_name),
    CONSTRAINT fk_stages_run FOREIGN KEY (run_id) REFERENCES pipeline_runs(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Entity mappings: the core audit trail linking local IDs to PlentyONE IDs
CREATE TABLE entity_mappings (
    id            BIGINT AUTO_INCREMENT PRIMARY KEY,
    run_id        BIGINT NOT NULL,
    local_id      BIGINT NOT NULL,
    plenty_id     BIGINT NOT NULL DEFAULT 0,
    entity_type   VARCHAR(50) NOT NULL,
    stage         VARCHAR(50) NOT NULL,
    status        VARCHAR(20) NOT NULL DEFAULT 'pending',
    error_message TEXT,
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY idx_local_entity (local_id, entity_type, run_id),
    INDEX idx_run_type_status (run_id, entity_type, status),
    INDEX idx_plenty (plenty_id, entity_type),
    CONSTRAINT fk_mappings_run FOREIGN KEY (run_id) REFERENCES pipeline_runs(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Categories (generated data)
CREATE TABLE categories (
    id          BIGINT AUTO_INCREMENT PRIMARY KEY,
    job_id      BIGINT NOT NULL,
    parent_id   BIGINT,
    name        VARCHAR(255) NOT NULL,
    level       INT NOT NULL DEFAULT 1,
    sort_order  INT NOT NULL DEFAULT 0,
    status      VARCHAR(20) NOT NULL DEFAULT 'pending',
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_job (job_id),
    INDEX idx_parent (parent_id),
    CONSTRAINT fk_categories_job FOREIGN KEY (job_id) REFERENCES jobs(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Attributes (e.g., Color, Size -- shared across products)
CREATE TABLE attributes (
    id          BIGINT AUTO_INCREMENT PRIMARY KEY,
    job_id      BIGINT NOT NULL,
    name        VARCHAR(255) NOT NULL,
    attr_type   VARCHAR(50) NOT NULL,
    status      VARCHAR(20) NOT NULL DEFAULT 'pending',
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_job (job_id),
    CONSTRAINT fk_attributes_job FOREIGN KEY (job_id) REFERENCES jobs(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Attribute values (e.g., Color: Red, Blue, Green)
CREATE TABLE attribute_values (
    id            BIGINT AUTO_INCREMENT PRIMARY KEY,
    attribute_id  BIGINT NOT NULL,
    name          VARCHAR(255) NOT NULL,
    sort_order    INT NOT NULL DEFAULT 0,
    CONSTRAINT fk_attrvals_attr FOREIGN KEY (attribute_id) REFERENCES attributes(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Properties (metadata fields on variations)
CREATE TABLE properties (
    id            BIGINT AUTO_INCREMENT PRIMARY KEY,
    job_id        BIGINT NOT NULL,
    name          VARCHAR(255) NOT NULL,
    property_type VARCHAR(50) NOT NULL,
    status        VARCHAR(20) NOT NULL DEFAULT 'pending',
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_job (job_id),
    CONSTRAINT fk_properties_job FOREIGN KEY (job_id) REFERENCES jobs(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Products (generated parent products)
CREATE TABLE products (
    id            BIGINT AUTO_INCREMENT PRIMARY KEY,
    job_id        BIGINT NOT NULL,
    name          VARCHAR(255) NOT NULL,
    product_type  VARCHAR(100) NOT NULL DEFAULT '',
    base_data     JSON NOT NULL,
    status        VARCHAR(20) NOT NULL DEFAULT 'pending',
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_job (job_id),
    INDEX idx_status (status),
    CONSTRAINT fk_products_job FOREIGN KEY (job_id) REFERENCES jobs(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Product-Category associations
CREATE TABLE product_categories (
    product_id  BIGINT NOT NULL,
    category_id BIGINT NOT NULL,
    PRIMARY KEY (product_id, category_id),
    CONSTRAINT fk_pc_product FOREIGN KEY (product_id) REFERENCES products(id),
    CONSTRAINT fk_pc_category FOREIGN KEY (category_id) REFERENCES categories(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Variations
CREATE TABLE variations (
    id          BIGINT AUTO_INCREMENT PRIMARY KEY,
    product_id  BIGINT NOT NULL,
    name        VARCHAR(255) NOT NULL DEFAULT '',
    sku         VARCHAR(100) NOT NULL DEFAULT '',
    price       DECIMAL(10, 2),
    currency    VARCHAR(3) NOT NULL DEFAULT 'EUR',
    weight      DECIMAL(10, 3),
    weight_unit VARCHAR(10) NOT NULL DEFAULT 'kg',
    barcode     VARCHAR(100) NOT NULL DEFAULT '',
    status      VARCHAR(20) NOT NULL DEFAULT 'pending',
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_product (product_id),
    INDEX idx_sku (sku),
    CONSTRAINT fk_variations_product FOREIGN KEY (product_id) REFERENCES products(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Variation-Attribute links
CREATE TABLE variation_attributes (
    variation_id       BIGINT NOT NULL,
    attribute_id       BIGINT NOT NULL,
    attribute_value_id BIGINT NOT NULL,
    PRIMARY KEY (variation_id, attribute_id),
    CONSTRAINT fk_va_variation FOREIGN KEY (variation_id) REFERENCES variations(id),
    CONSTRAINT fk_va_attribute FOREIGN KEY (attribute_id) REFERENCES attributes(id),
    CONSTRAINT fk_va_value FOREIGN KEY (attribute_value_id) REFERENCES attribute_values(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Variation-Property values
CREATE TABLE variation_properties (
    id           BIGINT AUTO_INCREMENT PRIMARY KEY,
    variation_id BIGINT NOT NULL,
    property_id  BIGINT NOT NULL,
    value_text   TEXT,
    value_int    BIGINT,
    value_float  DECIMAL(10, 4),
    UNIQUE KEY idx_var_prop (variation_id, property_id),
    CONSTRAINT fk_vp_variation FOREIGN KEY (variation_id) REFERENCES variations(id),
    CONSTRAINT fk_vp_property FOREIGN KEY (property_id) REFERENCES properties(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Images
CREATE TABLE images (
    id          BIGINT AUTO_INCREMENT PRIMARY KEY,
    product_id  BIGINT NOT NULL,
    source_url  VARCHAR(2048) NOT NULL DEFAULT '',
    local_path  VARCHAR(512) NOT NULL DEFAULT '',
    position    INT NOT NULL DEFAULT 0,
    source_type VARCHAR(50) NOT NULL DEFAULT '',
    attribution TEXT,
    status      VARCHAR(20) NOT NULL DEFAULT 'pending',
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_product (product_id),
    CONSTRAINT fk_images_product FOREIGN KEY (product_id) REFERENCES products(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Multilingual texts (all text fields for all entity types)
CREATE TABLE texts (
    id          BIGINT AUTO_INCREMENT PRIMARY KEY,
    product_id  BIGINT NOT NULL,
    field       VARCHAR(50) NOT NULL,
    lang        VARCHAR(5) NOT NULL,
    content     TEXT NOT NULL,
    status      VARCHAR(20) NOT NULL DEFAULT 'pending',
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY idx_product_field_lang (product_id, field, lang),
    INDEX idx_product (product_id),
    CONSTRAINT fk_texts_product FOREIGN KEY (product_id) REFERENCES products(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Property multilingual texts (for text-type properties that need translations)
CREATE TABLE property_texts (
    id            BIGINT AUTO_INCREMENT PRIMARY KEY,
    variation_id  BIGINT NOT NULL,
    property_id   BIGINT NOT NULL,
    lang          VARCHAR(5) NOT NULL,
    content       TEXT NOT NULL,
    UNIQUE KEY idx_var_prop_lang (variation_id, property_id, lang),
    CONSTRAINT fk_pt_variation FOREIGN KEY (variation_id) REFERENCES variations(id),
    CONSTRAINT fk_pt_property FOREIGN KEY (property_id) REFERENCES properties(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
