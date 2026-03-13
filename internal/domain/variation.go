package domain

import "time"

// Variation represents a product variation (e.g., a specific size/color combination).
type Variation struct {
	ID         int64                `json:"id"`
	ProductID  int64                `json:"product_id"`
	Name       string               `json:"name"`
	SKU        string               `json:"sku"`
	Price      float64              `json:"price"`
	Currency   string               `json:"currency"`
	Attributes []VariationAttribute `json:"attributes"`
	Properties []VariationProperty  `json:"properties"`
	Status     EntityStatus         `json:"status"`
	CreatedAt  time.Time            `json:"created_at"`
	UpdatedAt  time.Time            `json:"updated_at"`
}

// VariationAttribute links a variation to an attribute value.
type VariationAttribute struct {
	AttributeID int64  `json:"attribute_id"`
	ValueID     int64  `json:"value_id"`
	ValueName   string `json:"value_name"`
}

// VariationProperty links a variation to a property value.
type VariationProperty struct {
	PropertyID int64  `json:"property_id"`
	Value      string `json:"value"`
}
