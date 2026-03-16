-- Add sales unit and RRP to variations.
ALTER TABLE variations ADD COLUMN sales_unit VARCHAR(20) NOT NULL DEFAULT '' AFTER weight_unit;
ALTER TABLE variations ADD COLUMN rrp DECIMAL(10, 2) AFTER price;

-- Graduated/quantity-based pricing tiers for a variation.
CREATE TABLE variation_graduated_prices (
    id               BIGINT AUTO_INCREMENT PRIMARY KEY,
    variation_id     BIGINT NOT NULL,
    minimum_quantity INT NOT NULL,
    price            DECIMAL(10, 2) NOT NULL,
    UNIQUE KEY idx_var_qty (variation_id, minimum_quantity),
    CONSTRAINT fk_vgp_variation FOREIGN KEY (variation_id) REFERENCES variations(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
