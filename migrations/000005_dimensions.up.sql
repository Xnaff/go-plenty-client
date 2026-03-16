-- Add product dimensions (stored in millimeters) to variations.
ALTER TABLE variations ADD COLUMN length_mm INT NOT NULL DEFAULT 0 AFTER sales_unit;
ALTER TABLE variations ADD COLUMN width_mm INT NOT NULL DEFAULT 0 AFTER length_mm;
ALTER TABLE variations ADD COLUMN height_mm INT NOT NULL DEFAULT 0 AFTER width_mm;
