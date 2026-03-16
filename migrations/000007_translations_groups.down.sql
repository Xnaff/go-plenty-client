ALTER TABLE properties DROP FOREIGN KEY fk_prop_group, DROP COLUMN property_group_id;
DROP TABLE IF EXISTS property_group_translations;
DROP TABLE IF EXISTS property_groups;
DROP TABLE IF EXISTS category_translations;
DROP TABLE IF EXISTS property_option_translations;
DROP TABLE IF EXISTS property_name_translations;
