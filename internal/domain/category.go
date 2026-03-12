package domain

import "time"

// Category represents a product category with tree structure support.
type Category struct {
	ID        int64        `json:"id"`
	JobID     int64        `json:"job_id"`
	ParentID  int64        `json:"parent_id"`
	Name      string       `json:"name"`
	Level     int          `json:"level"`
	SortOrder int          `json:"sort_order"`
	Status    EntityStatus `json:"status"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}
