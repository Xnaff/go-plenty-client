package pipeline

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/janemig/plentyone/internal/domain"
	"github.com/janemig/plentyone/internal/plenty"
	"github.com/janemig/plentyone/internal/storage/queries"
)

// PipelineConfig holds pipeline execution settings.
type PipelineConfig struct {
	// Concurrency controls how many items are processed in parallel per stage.
	// Defaults to 5 if zero.
	Concurrency int
}

// Pipeline orchestrates the ordered execution of pipeline stages.
// It manages stage registration, ordered execution, resume from failure,
// and state tracking in pipeline_runs and stage_states.
type Pipeline struct {
	stages map[domain.StageName]Stage
	client *plenty.Client
	db     *queries.Queries
	rawDB  *sql.DB
	logger *slog.Logger
	cfg    PipelineConfig
}

// NewPipeline creates a new Pipeline. Stages are NOT registered here --
// callers use RegisterStage to add them. This avoids import cycles and
// allows tests to register mock stages.
func NewPipeline(client *plenty.Client, db *queries.Queries, rawDB *sql.DB, cfg PipelineConfig, logger *slog.Logger) *Pipeline {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 5
	}
	if logger == nil {
		logger = slog.Default()
	}

	return &Pipeline{
		stages: make(map[domain.StageName]Stage),
		client: client,
		db:     db,
		rawDB:  rawDB,
		logger: logger,
		cfg:    cfg,
	}
}

// RegisterStage adds a stage to the pipeline. Stages are executed in StageOrder
// regardless of registration order.
func (p *Pipeline) RegisterStage(s Stage) {
	p.stages[s.Name()] = s
}

// DB returns the queries instance for use by stages.
func (p *Pipeline) DB() *queries.Queries {
	return p.db
}

// Client returns the PlentyONE client for use by stages.
func (p *Pipeline) Client() *plenty.Client {
	return p.client
}

// Config returns the pipeline configuration for use by stages.
func (p *Pipeline) Config() PipelineConfig {
	return p.cfg
}

// Run executes all registered stages in StageOrder.
// It checks stage_states for resume support -- completed stages are skipped.
// Pipeline state is tracked in pipeline_runs and stage_states tables.
func (p *Pipeline) Run(ctx context.Context, rc *RunContext) error {
	p.logger.Info("pipeline starting",
		slog.Int64("run_id", rc.RunID),
		slog.Int64("job_id", rc.JobID),
		slog.Bool("dry_run", rc.DryRun),
	)

	for _, stageName := range StageOrder {
		stage, ok := p.stages[stageName]
		if !ok {
			p.logger.Debug("stage not registered, skipping",
				slog.String("stage", string(stageName)),
			)
			continue
		}

		// Check if this stage was already completed (resume support).
		stageState, err := p.db.GetStageState(ctx, queries.GetStageStateParams{
			RunID:     rc.RunID,
			StageName: string(stageName),
		})
		if err == nil && stageState.Status == string(domain.StageCompleted) {
			p.logger.Info("stage already completed, skipping",
				slog.String("stage", string(stageName)),
			)
			continue
		}

		// Update pipeline_runs.current_stage.
		if err := p.db.UpdatePipelineRunStatus(ctx, queries.UpdatePipelineRunStatusParams{
			Status:       string(domain.PipelineRunning),
			CurrentStage: sql.NullString{String: string(stageName), Valid: true},
			ErrorMessage: sql.NullString{},
			ID:           rc.RunID,
		}); err != nil {
			return fmt.Errorf("updating pipeline run current stage: %w", err)
		}

		// Get or create stage_state.
		ssID, err := p.ensureStageState(ctx, rc.RunID, stageName)
		if err != nil {
			return fmt.Errorf("ensuring stage state for %s: %w", stageName, err)
		}

		// Mark stage as running with started_at.
		now := time.Now()
		if err := p.db.UpdateStageStateTimestamps(ctx, queries.UpdateStageStateTimestampsParams{
			Status:    string(domain.StageRunning),
			StartedAt: sql.NullTime{Time: now, Valid: true},
			ID:        ssID,
		}); err != nil {
			return fmt.Errorf("marking stage %s as running: %w", stageName, err)
		}

		p.logger.Info("executing stage",
			slog.String("stage", string(stageName)),
			slog.Int64("run_id", rc.RunID),
		)

		// Execute the stage.
		stageErr := stage.Execute(ctx, rc)

		// Update stage_state with completion info.
		completedAt := time.Now()
		status := domain.StageCompleted
		if stageErr != nil {
			status = domain.StageFailed
		}

		if err := p.db.UpdateStageStateTimestamps(ctx, queries.UpdateStageStateTimestampsParams{
			Status:      string(status),
			CompletedAt: sql.NullTime{Time: completedAt, Valid: true},
			StartedAt:   sql.NullTime{Time: now, Valid: true},
			ID:          ssID,
		}); err != nil {
			p.logger.Error("failed to update stage state after execution",
				slog.String("stage", string(stageName)),
				slog.Any("error", err),
			)
		}

		if stageErr != nil {
			// Mark pipeline as failed.
			_ = p.db.UpdatePipelineRunStatus(ctx, queries.UpdatePipelineRunStatusParams{
				Status:       string(domain.PipelineFailed),
				CurrentStage: sql.NullString{String: string(stageName), Valid: true},
				ErrorMessage: sql.NullString{String: stageErr.Error(), Valid: true},
				ID:           rc.RunID,
			})
			return fmt.Errorf("stage %s failed: %w", stageName, stageErr)
		}

		p.logger.Info("stage completed",
			slog.String("stage", string(stageName)),
			slog.Duration("duration", completedAt.Sub(now)),
		)
	}

	// All stages completed successfully.
	if err := p.db.UpdatePipelineRunCompleted(ctx, queries.UpdatePipelineRunCompletedParams{
		Status:       string(domain.PipelineCompleted),
		CurrentStage: sql.NullString{},
		ID:           rc.RunID,
	}); err != nil {
		return fmt.Errorf("marking pipeline run as completed: %w", err)
	}

	p.logger.Info("pipeline completed successfully",
		slog.Int64("run_id", rc.RunID),
	)
	return nil
}

// Resume loads an existing pipeline run and re-executes it.
// Completed stages are skipped; failed mappings are optionally reset for retry.
func (p *Pipeline) Resume(ctx context.Context, runID int64, resetFailed bool) error {
	run, err := p.db.GetPipelineRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("loading pipeline run %d: %w", runID, err)
	}

	if resetFailed {
		if err := p.db.ResetFailedMappingsForRetry(ctx, runID); err != nil {
			return fmt.Errorf("resetting failed mappings for run %d: %w", runID, err)
		}
		p.logger.Info("reset failed mappings for retry",
			slog.Int64("run_id", runID),
		)
	}

	rc := &RunContext{
		RunID:  runID,
		JobID:  run.JobID,
		DryRun: false,
		Logger: p.logger,
	}

	return p.Run(ctx, rc)
}

// ensureStageState returns the stage_state ID, creating one if it doesn't exist.
func (p *Pipeline) ensureStageState(ctx context.Context, runID int64, stageName domain.StageName) (int64, error) {
	ss, err := p.db.GetStageState(ctx, queries.GetStageStateParams{
		RunID:     runID,
		StageName: string(stageName),
	})
	if err == nil {
		return ss.ID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}

	// Create a new stage_state.
	id, err := p.db.CreateStageState(ctx, queries.CreateStageStateParams{
		RunID:     runID,
		StageName: string(stageName),
		Status:    string(domain.StagePending),
	})
	if err != nil {
		return 0, err
	}
	return id, nil
}
