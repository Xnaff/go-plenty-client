package domain

import "time"

// Property represents a product property (e.g., Material, Country of Origin).
// PropertyType determines how values are stored: text, int, float, or selection.
type Property struct {
	ID           int64        `json:"id"`
	JobID        int64        `json:"job_id"`
	Name         string       `json:"name"`
	PropertyType string       `json:"property_type"` // text, int, float, selection
	Status       EntityStatus `json:"status"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
}
