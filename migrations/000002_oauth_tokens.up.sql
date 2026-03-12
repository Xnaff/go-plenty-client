CREATE TABLE oauth_tokens (
    id             BIGINT AUTO_INCREMENT PRIMARY KEY,
    shop_url       VARCHAR(255) NOT NULL,
    access_token   TEXT NOT NULL,
    refresh_token  TEXT NOT NULL,
    token_type     VARCHAR(20) NOT NULL DEFAULT 'Bearer',
    expires_at     TIMESTAMP NOT NULL,
    created_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY idx_shop (shop_url)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
