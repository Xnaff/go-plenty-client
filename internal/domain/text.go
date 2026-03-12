package domain

import "time"

// Text represents a multilingual text entry for a product field.
type Text struct {
	ID        int64        `json:"id"`
	ProductID int64        `json:"product_id"`
	Field     string       `json:"field"`   // e.g., "name", "description", "short_description"
	Lang      string       `json:"lang"`    // e.g., "en", "de", "es", "fr", "it"
	Content   string       `json:"content"`
	Status    EntityStatus `json:"status"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

// PropertyText represents a multilingual text entry for a property value.
type PropertyText struct {
	ID         int64     `json:"id"`
	PropertyID int64     `json:"property_id"`
	ProductID  int64     `json:"product_id"`
	Lang       string    `json:"lang"`
	Value      string    `json:"value"`
	CreatedAt  time.Time `json:"created_at"`
}
