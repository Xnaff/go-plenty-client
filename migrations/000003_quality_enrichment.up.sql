CREATE TABLE quality_scores (
    id          BIGINT AUTO_INCREMENT PRIMARY KEY,
    product_id  BIGINT NOT NULL,
    job_id      BIGINT NOT NULL,
    overall     DECIMAL(5,4) NOT NULL,
    text_score  DECIMAL(5,4) NOT NULL,
    image_score DECIMAL(5,4) NOT NULL,
    data_score  DECIMAL(5,4) NOT NULL,
    pass        BOOLEAN NOT NULL DEFAULT TRUE,
    details     JSON NOT NULL,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY idx_qs_product (product_id),
    INDEX idx_qs_job_pass (job_id, pass),
    CONSTRAINT fk_qs_product FOREIGN KEY (product_id) REFERENCES products(id),
    CONSTRAINT fk_qs_job FOREIGN KEY (job_id) REFERENCES jobs(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE enrichment_cache (
    id          BIGINT AUTO_INCREMENT PRIMARY KEY,
    source      VARCHAR(50) NOT NULL,
    query_key   VARCHAR(512) NOT NULL,
    data        JSON NOT NULL,
    expires_at  TIMESTAMP NOT NULL,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY idx_ec_source_key (source, query_key),
    INDEX idx_ec_expires (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
