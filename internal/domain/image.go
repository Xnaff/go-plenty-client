package domain

import "time"

// Image represents a product image with source tracking.
type Image struct {
	ID          int64        `json:"id"`
	ProductID   int64        `json:"product_id"`
	SourceURL   string       `json:"source_url"`
	LocalPath   string       `json:"local_path"`
	Position    int          `json:"position"`
	SourceType  string       `json:"source_type"`  // e.g., "unsplash", "pexels", "generated"
	Attribution string       `json:"attribution"`   // Credit/license info for the image source
	Status      EntityStatus `json:"status"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}
