package domain

import "time"

// EntityMapping tracks the relationship between a local entity and its
// PlentyONE counterpart. This is the core audit trail for the pipeline.
type EntityMapping struct {
	ID           int64        `json:"id"`
	RunID        int64        `json:"run_id"`         // Which pipeline run created this
	LocalID      int64        `json:"local_id"`       // Our internal ID
	PlentyID     int64        `json:"plenty_id"`      // PlentyONE's ID (0 if not yet created)
	EntityType   EntityType   `json:"entity_type"`    // category, product, variation, etc.
	Stage        StageName    `json:"stage"`          // Which stage created/manages this
	Status       EntityStatus `json:"status"`         // pending, created, failed, orphaned, etc.
	ErrorMessage string       `json:"error_message"`  // Last error if failed
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
}
