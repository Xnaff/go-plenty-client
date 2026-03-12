package domain

import "time"

// Attribute represents a product attribute (e.g., Color, Size).
type Attribute struct {
	ID        int64            `json:"id"`
	JobID     int64            `json:"job_id"`
	Name      string           `json:"name"`
	Values    []AttributeValue `json:"values"`
	Status    EntityStatus     `json:"status"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
}

// AttributeValue represents a single value for an attribute (e.g., "Red" for Color).
type AttributeValue struct {
	ID          int64        `json:"id"`
	AttributeID int64        `json:"attribute_id"`
	Name        string       `json:"name"`
	Status      EntityStatus `json:"status"`
	CreatedAt   time.Time    `json:"created_at"`
}
