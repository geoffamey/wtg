// Package saga provides a simple pre-flight + compensating-action transaction pattern.
//
// Commands that mutate state (space create, space add, space delete) build a
// slice of Steps. Run executes each Step's Do in order; if any step fails, all
// previously completed steps are unwound by calling their Undo in reverse order.
package saga

import (
	"context"
	"fmt"
	"log/slog"
)

// Step is one unit of work within a saga, paired with its compensation.
type Step struct {
	// Name is used in error messages and log output.
	Name string
	// Do performs the forward action. Returning an error triggers rollback.
	Do func(ctx context.Context) error
	// Undo reverses the forward action. Errors during Undo are logged but do
	// not prevent remaining compensations from running.
	Undo func(ctx context.Context) error
}

// Run executes each step's Do in order. On the first failure it calls Undo on
// all previously completed steps in reverse order, then returns the original
// error wrapped with the failing step's name.
//
// Compensation errors are logged via slog.Default() but do not affect the
// returned error.
func Run(ctx context.Context, steps []Step) error {
	var done []Step
	for _, step := range steps {
		if err := step.Do(ctx); err != nil {
			// Unwind completed steps in reverse order.
			for i := len(done) - 1; i >= 0; i-- {
				if cerr := done[i].Undo(ctx); cerr != nil {
					slog.Default().Error("saga compensation failed",
						"step", done[i].Name,
						"error", cerr,
					)
				}
			}
			return fmt.Errorf("step %q: %w", step.Name, err)
		}
		done = append(done, step)
	}
	return nil
}
