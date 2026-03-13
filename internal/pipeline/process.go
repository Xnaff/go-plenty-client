package pipeline

import (
	"context"
	"sync"

	"golang.org/x/sync/errgroup"
)

// ProcessItems runs processFn concurrently for each item with bounded parallelism.
// Failed items are counted but do not cancel processing of other items (the Go()
// callback returns nil so errgroup keeps running). This implements the
// "pause-and-flag" pattern: individual failures are recorded, not propagated.
//
// Returns the number of succeeded and failed items.
func ProcessItems[T any](ctx context.Context, items []T, concurrency int, processFn func(ctx context.Context, item T) error) (succeeded, failed int) {
	if len(items) == 0 {
		return 0, 0
	}
	if concurrency <= 0 {
		concurrency = 5
	}

	var mu sync.Mutex
	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(concurrency)

	for _, item := range items {
		g.Go(func() error {
			if err := processFn(gCtx, item); err != nil {
				mu.Lock()
				failed++
				mu.Unlock()
				// Return nil to avoid cancelling other items.
				// The caller is responsible for logging/recording the error
				// in entity_mappings via the processFn itself.
				return nil
			}
			mu.Lock()
			succeeded++
			mu.Unlock()
			return nil
		})
	}

	// g.Wait always returns nil since processFn errors are swallowed above.
	_ = g.Wait()

	return succeeded, failed
}
