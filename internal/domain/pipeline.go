package domain

import "time"

// Job represents a generation or push job that triggers a pipeline run.
type Job struct {
	ID          int64        `json:"id"`
	Type        JobType      `json:"type"`         // generate, push
	ProductType string       `json:"product_type"` // e.g., "food", "electronics"
	Count       int          `json:"count"`        // Number of products to generate/push
	Status      EntityStatus `json:"status"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// PipelineRun represents a single execution of the 6-stage pipeline.
type PipelineRun struct {
	ID          int64          `json:"id"`
	JobID       int64          `json:"job_id"`
	Status      PipelineStatus `json:"status"`
	CurrentStage StageName     `json:"current_stage"`
	StartedAt   time.Time      `json:"started_at"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// StageState tracks the progress of a single stage within a pipeline run.
type StageState struct {
	ID          int64       `json:"id"`
	RunID       int64       `json:"run_id"`
	Stage       StageName   `json:"stage"`
	Status      StageStatus `json:"status"`
	TotalItems  int         `json:"total_items"`
	DoneItems   int         `json:"done_items"`
	FailedItems int         `json:"failed_items"`
	StartedAt   *time.Time  `json:"started_at,omitempty"`
	CompletedAt *time.Time  `json:"completed_at,omitempty"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}
