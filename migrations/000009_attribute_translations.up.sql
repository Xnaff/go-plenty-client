-- Attribute name translations (multilingual attribute names).
CREATE TABLE attribute_name_translations (
    id           BIGINT AUTO_INCREMENT PRIMARY KEY,
    attribute_id BIGINT NOT NULL,
    lang         VARCHAR(5) NOT NULL,
    name         TEXT NOT NULL,
    UNIQUE KEY idx_attr_lang (attribute_id, lang),
    CONSTRAINT fk_ant_attr FOREIGN KEY (attribute_id) REFERENCES attributes(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Attribute value translations (multilingual attribute value names).
CREATE TABLE attribute_value_translations (
    id                 BIGINT AUTO_INCREMENT PRIMARY KEY,
    attribute_value_id BIGINT NOT NULL,
    lang               VARCHAR(5) NOT NULL,
    name               TEXT NOT NULL,
    UNIQUE KEY idx_attrval_lang (attribute_value_id, lang),
    CONSTRAINT fk_avt_val FOREIGN KEY (attribute_value_id) REFERENCES attribute_values(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
