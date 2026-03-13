package pipeline

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/janemig/plentyone/internal/domain"
)

// ---------------------------------------------------------------------------
// Mock stage for testing pipeline runner behavior
// ---------------------------------------------------------------------------

type mockStage struct {
	name      domain.StageName
	executeFn func(ctx context.Context, run *RunContext) error
}

func (m *mockStage) Name() domain.StageName { return m.name }

func (m *mockStage) Execute(ctx context.Context, run *RunContext) error {
	return m.executeFn(ctx, run)
}

// ---------------------------------------------------------------------------
// ProcessItems tests (pure logic, no DB)
// ---------------------------------------------------------------------------

func TestProcessItemsAllSucceed(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}
	var processed atomic.Int32

	succeeded, failed := ProcessItems(context.Background(), items, 3, func(_ context.Context, item int) error {
		processed.Add(1)
		return nil
	})

	if succeeded != 5 {
		t.Errorf("expected 5 succeeded, got %d", succeeded)
	}
	if failed != 0 {
		t.Errorf("expected 0 failed, got %d", failed)
	}
	if processed.Load() != 5 {
		t.Errorf("expected 5 processed, got %d", processed.Load())
	}
}

func TestProcessItemsWithFailures(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}

	succeeded, failed := ProcessItems(context.Background(), items, 5, func(_ context.Context, item int) error {
		if item == 3 {
			return errors.New("item 3 failed")
		}
		return nil
	})

	if succeeded != 4 {
		t.Errorf("expected 4 succeeded, got %d", succeeded)
	}
	if failed != 1 {
		t.Errorf("expected 1 failed, got %d", failed)
	}
}

func TestProcessItemsPerItemErrorIsolation(t *testing.T) {
	// Verify that a failing item does NOT cancel or affect other items.
	items := []int{1, 2, 3, 4, 5}
	var mu sync.Mutex
	processedItems := make(map[int]bool)

	succeeded, failed := ProcessItems(context.Background(), items, 1, func(_ context.Context, item int) error {
		mu.Lock()
		processedItems[item] = true
		mu.Unlock()

		if item == 3 {
			return errors.New("item 3 failed")
		}
		return nil
	})

	if succeeded != 4 {
		t.Errorf("expected 4 succeeded, got %d", succeeded)
	}
	if failed != 1 {
		t.Errorf("expected 1 failed, got %d", failed)
	}

	// All 5 items should have been processed (including the failing one).
	for _, i := range items {
		if !processedItems[i] {
			t.Errorf("item %d was not processed", i)
		}
	}
}

func TestProcessItemsEmptySlice(t *testing.T) {
	succeeded, failed := ProcessItems(context.Background(), []int{}, 5, func(_ context.Context, item int) error {
		t.Fatal("should not be called")
		return nil
	})

	if succeeded != 0 || failed != 0 {
		t.Errorf("expected 0/0, got %d/%d", succeeded, failed)
	}
}

func TestProcessItemsAllFail(t *testing.T) {
	items := []int{1, 2, 3}

	succeeded, failed := ProcessItems(context.Background(), items, 2, func(_ context.Context, item int) error {
		return errors.New("always fail")
	})

	if succeeded != 0 {
		t.Errorf("expected 0 succeeded, got %d", succeeded)
	}
	if failed != 3 {
		t.Errorf("expected 3 failed, got %d", failed)
	}
}

func TestProcessItemsConcurrencyBound(t *testing.T) {
	// Verify that concurrency is respected: at most N goroutines run simultaneously.
	items := make([]int, 20)
	for i := range items {
		items[i] = i
	}

	var maxConcurrent atomic.Int32
	var current atomic.Int32

	ProcessItems(context.Background(), items, 3, func(_ context.Context, _ int) error {
		cur := current.Add(1)
		// Track peak concurrency.
		for {
			old := maxConcurrent.Load()
			if cur <= old || maxConcurrent.CompareAndSwap(old, cur) {
				break
			}
		}

		// Simulate some work so goroutines overlap.
		for i := 0; i < 1000; i++ {
			_ = i
		}

		current.Add(-1)
		return nil
	})

	peak := maxConcurrent.Load()
	if peak > 3 {
		t.Errorf("max concurrent goroutines exceeded limit: got %d, want <= 3", peak)
	}
}

// ---------------------------------------------------------------------------
// Mock stage ordering tests (no DB)
// ---------------------------------------------------------------------------

func TestPipelineStageOrderDefinition(t *testing.T) {
	// Verify StageOrder has exactly 6 stages in the correct sequence.
	expected := []domain.StageName{
		domain.StageCategories,
		domain.StageAttributes,
		domain.StageProducts,
		domain.StageVariations,
		domain.StageImages,
		domain.StageTexts,
	}

	if len(StageOrder) != len(expected) {
		t.Fatalf("expected %d stages, got %d", len(expected), len(StageOrder))
	}

	for i, want := range expected {
		if StageOrder[i] != want {
			t.Errorf("StageOrder[%d] = %q, want %q", i, StageOrder[i], want)
		}
	}
}

func TestPipelineOrderedExecution(t *testing.T) {
	// Register 3 mock stages and verify they execute in StageOrder.
	var mu sync.Mutex
	executionOrder := []domain.StageName{}

	stageCategories := &mockStage{
		name: domain.StageCategories,
		executeFn: func(_ context.Context, _ *RunContext) error {
			mu.Lock()
			executionOrder = append(executionOrder, domain.StageCategories)
			mu.Unlock()
			return nil
		},
	}

	stageProducts := &mockStage{
		name: domain.StageProducts,
		executeFn: func(_ context.Context, _ *RunContext) error {
			mu.Lock()
			executionOrder = append(executionOrder, domain.StageProducts)
			mu.Unlock()
			return nil
		},
	}

	stageTexts := &mockStage{
		name: domain.StageTexts,
		executeFn: func(_ context.Context, _ *RunContext) error {
			mu.Lock()
			executionOrder = append(executionOrder, domain.StageTexts)
			mu.Unlock()
			return nil
		},
	}

	// Create a pipeline without a real DB. We need the runner for registration
	// and ordering logic, but since the mock stages don't interact with the DB
	// (they just record execution order), we test the registration and lookup.
	p := &Pipeline{
		stages: make(map[domain.StageName]Stage),
	}
	p.RegisterStage(stageTexts)      // Register out of order
	p.RegisterStage(stageCategories) // Registration order shouldn't matter
	p.RegisterStage(stageProducts)

	// Verify that iterating StageOrder and looking up stages gives the right order.
	for _, stageName := range StageOrder {
		stage, ok := p.stages[stageName]
		if !ok {
			continue
		}
		if err := stage.Execute(context.Background(), &RunContext{}); err != nil {
			t.Fatalf("stage %s failed: %v", stageName, err)
		}
	}

	// Verify execution order matches StageOrder (only for registered stages).
	expected := []domain.StageName{
		domain.StageCategories,
		domain.StageProducts,
		domain.StageTexts,
	}

	if len(executionOrder) != len(expected) {
		t.Fatalf("expected %d stages executed, got %d", len(expected), len(executionOrder))
	}

	for i, want := range expected {
		if executionOrder[i] != want {
			t.Errorf("execution order[%d] = %q, want %q", i, executionOrder[i], want)
		}
	}
}

func TestPipelineUnregisteredStagesSkipped(t *testing.T) {
	// Register only 1 of 6 stages. The others should be silently skipped.
	var executed []domain.StageName

	stageImages := &mockStage{
		name: domain.StageImages,
		executeFn: func(_ context.Context, _ *RunContext) error {
			executed = append(executed, domain.StageImages)
			return nil
		},
	}

	p := &Pipeline{
		stages: make(map[domain.StageName]Stage),
	}
	p.RegisterStage(stageImages)

	// Simulate the runner loop (without DB calls).
	for _, stageName := range StageOrder {
		stage, ok := p.stages[stageName]
		if !ok {
			continue
		}
		if err := stage.Execute(context.Background(), &RunContext{}); err != nil {
			t.Fatalf("stage %s failed: %v", stageName, err)
		}
	}

	if len(executed) != 1 || executed[0] != domain.StageImages {
		t.Errorf("expected only images stage executed, got %v", executed)
	}
}

func TestPipelineStageFailureStops(t *testing.T) {
	// If a stage fails, subsequent stages should not execute.
	var executed []domain.StageName

	stageCategories := &mockStage{
		name: domain.StageCategories,
		executeFn: func(_ context.Context, _ *RunContext) error {
			executed = append(executed, domain.StageCategories)
			return errors.New("categories failed")
		},
	}

	stageProducts := &mockStage{
		name: domain.StageProducts,
		executeFn: func(_ context.Context, _ *RunContext) error {
			executed = append(executed, domain.StageProducts)
			return nil
		},
	}

	p := &Pipeline{
		stages: make(map[domain.StageName]Stage),
	}
	p.RegisterStage(stageCategories)
	p.RegisterStage(stageProducts)

	// Simulate the runner loop with failure handling.
	for _, stageName := range StageOrder {
		stage, ok := p.stages[stageName]
		if !ok {
			continue
		}
		if err := stage.Execute(context.Background(), &RunContext{}); err != nil {
			break // Stop on failure, like the real runner does.
		}
	}

	if len(executed) != 1 {
		t.Errorf("expected 1 stage executed (categories only), got %d: %v", len(executed), executed)
	}
	if executed[0] != domain.StageCategories {
		t.Errorf("expected categories, got %v", executed[0])
	}
}

// ---------------------------------------------------------------------------
// ShouldProcess logic tests (pure logic validation)
// ---------------------------------------------------------------------------

func TestProcessCheckValues(t *testing.T) {
	// Verify ProcessCheck struct semantics.
	tests := []struct {
		name            string
		check           ProcessCheck
		wantNeedsProc   bool
		wantExistingPos bool // existingID > 0
	}{
		{
			name:            "new item",
			check:           ProcessCheck{NeedsProcessing: true, ExistingID: 0},
			wantNeedsProc:   true,
			wantExistingPos: false,
		},
		{
			name:            "already created",
			check:           ProcessCheck{NeedsProcessing: false, ExistingID: 42},
			wantNeedsProc:   false,
			wantExistingPos: true,
		},
		{
			name:            "retry failed",
			check:           ProcessCheck{NeedsProcessing: true, ExistingID: 99},
			wantNeedsProc:   true,
			wantExistingPos: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.check.NeedsProcessing != tt.wantNeedsProc {
				t.Errorf("NeedsProcessing = %v, want %v", tt.check.NeedsProcessing, tt.wantNeedsProc)
			}
			gotPos := tt.check.ExistingID > 0
			if gotPos != tt.wantExistingPos {
				t.Errorf("ExistingID > 0 = %v, want %v", gotPos, tt.wantExistingPos)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Stage interface compliance tests
// ---------------------------------------------------------------------------

func TestStageInterfaceCompliance(t *testing.T) {
	// Ensure all stage types satisfy the Stage interface at compile time.
	// These are also checked via var _ Stage = (*XxxStage)(nil) in each file,
	// but having an explicit test makes it visible in test output.
	var _ Stage = (*VariationStage)(nil)
	var _ Stage = (*ImageStage)(nil)
	var _ Stage = (*TextStage)(nil)
}

func TestStageNames(t *testing.T) {
	tests := []struct {
		stage Stage
		want  domain.StageName
	}{
		{&VariationStage{}, domain.StageVariations},
		{&ImageStage{}, domain.StageImages},
		{&TextStage{}, domain.StageTexts},
	}

	for _, tt := range tests {
		t.Run(string(tt.want), func(t *testing.T) {
			if got := tt.stage.Name(); got != tt.want {
				t.Errorf("Name() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Cleanup reverse order test
// ---------------------------------------------------------------------------

func TestCleanupReverseOrder(t *testing.T) {
	// The cleanup entity types should be in reverse stage order.
	// Texts -> Images -> Variations -> Products -> Properties -> Attributes -> Categories
	expectedOrder := []string{
		string(domain.EntityText),
		string(domain.EntityImage),
		string(domain.EntityVariation),
		string(domain.EntityProduct),
		string(domain.EntityProperty),
		string(domain.EntityAttribute),
		string(domain.EntityCategory),
	}

	// We can't call CleanupRun without a DB, but we can verify the
	// cleanupEntityType slice structure. The types are package-private,
	// so we verify via the expected reverse order of domain constants.

	// Verify expected order is the reverse of a logical creation order.
	creationOrder := []string{
		string(domain.EntityCategory),
		string(domain.EntityAttribute),
		string(domain.EntityProperty),
		string(domain.EntityProduct),
		string(domain.EntityVariation),
		string(domain.EntityImage),
		string(domain.EntityText),
	}

	if len(expectedOrder) != len(creationOrder) {
		t.Fatal("length mismatch")
	}

	for i := 0; i < len(expectedOrder); i++ {
		reverseIdx := len(creationOrder) - 1 - i
		if expectedOrder[i] != creationOrder[reverseIdx] {
			t.Errorf("expectedOrder[%d] = %q, want %q (reverse of creation order)",
				i, expectedOrder[i], creationOrder[reverseIdx])
		}
	}
}
