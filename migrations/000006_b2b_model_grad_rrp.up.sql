-- Add B2B prices and model to variations, add RRP to graduated prices.
ALTER TABLE variations ADD COLUMN b2b_price DECIMAL(10, 2) AFTER rrp;
ALTER TABLE variations ADD COLUMN b2b_rrp DECIMAL(10, 2) AFTER b2b_price;
ALTER TABLE variations ADD COLUMN model VARCHAR(100) NOT NULL DEFAULT '' AFTER barcode;
ALTER TABLE variation_graduated_prices ADD COLUMN rrp DECIMAL(10, 2) NOT NULL DEFAULT 0 AFTER price;
