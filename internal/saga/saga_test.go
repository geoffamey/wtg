package saga

import (
	"context"
	"errors"
	"testing"
)

var ctx = context.Background()

func step(name string, doErr, undoErr error) Step {
	return Step{
		Name: name,
		Do:   func(context.Context) error { return doErr },
		Undo: func(context.Context) error { return undoErr },
	}
}

func counter(n *int) func(context.Context) error {
	return func(context.Context) error { *n++; return nil }
}

func TestRun_AllSucceed(t *testing.T) {
	var undos int
	steps := []Step{
		{Name: "a", Do: func(context.Context) error { return nil }, Undo: counter(&undos)},
		{Name: "b", Do: func(context.Context) error { return nil }, Undo: counter(&undos)},
		{Name: "c", Do: func(context.Context) error { return nil }, Undo: counter(&undos)},
	}
	if err := Run(ctx, steps); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if undos != 0 {
		t.Errorf("no undos expected on success, got %d", undos)
	}
}

func TestRun_FirstStepFails(t *testing.T) {
	boom := errors.New("boom")
	var undos int
	steps := []Step{
		{Name: "a", Do: func(context.Context) error { return boom }, Undo: counter(&undos)},
		{Name: "b", Do: func(context.Context) error { return nil }, Undo: counter(&undos)},
	}
	err := Run(ctx, steps)
	if !errors.Is(err, boom) {
		t.Fatalf("expected boom, got %v", err)
	}
	// Step "a" failed before completing, so nothing to undo.
	if undos != 0 {
		t.Errorf("expected 0 undos, got %d", undos)
	}
}

func TestRun_MiddleStepFails(t *testing.T) {
	boom := errors.New("boom")
	var undoOrder []string
	makeStep := func(name string, doErr error) Step {
		return Step{
			Name: name,
			Do:   func(context.Context) error { return doErr },
			Undo: func(context.Context) error { undoOrder = append(undoOrder, name); return nil },
		}
	}

	steps := []Step{
		makeStep("a", nil),
		makeStep("b", nil),
		makeStep("c", boom),
		makeStep("d", nil),
	}
	err := Run(ctx, steps)
	if !errors.Is(err, boom) {
		t.Fatalf("expected boom, got %v", err)
	}
	// "a" and "b" completed; they should be undone in reverse: b, a.
	if len(undoOrder) != 2 {
		t.Fatalf("expected 2 undos, got %d: %v", len(undoOrder), undoOrder)
	}
	if undoOrder[0] != "b" || undoOrder[1] != "a" {
		t.Errorf("undo order: got %v, want [b a]", undoOrder)
	}
}

func TestRun_ErrorWrapsStepName(t *testing.T) {
	boom := errors.New("boom")
	err := Run(ctx, []Step{step("my-step", boom, nil)})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, boom) {
		t.Errorf("error should wrap original: %v", err)
	}
	if got := err.Error(); got != `step "my-step": boom` {
		t.Errorf("unexpected error message: %q", got)
	}
}

func TestRun_CompensationErrorDoesNotAbortRollback(t *testing.T) {
	boom := errors.New("boom")
	undoErr := errors.New("undo-failed")
	var undoCount int

	steps := []Step{
		{
			Name: "a",
			Do:   func(context.Context) error { return nil },
			Undo: func(context.Context) error { undoCount++; return undoErr },
		},
		{
			Name: "b",
			Do:   func(context.Context) error { return nil },
			Undo: func(context.Context) error { undoCount++; return undoErr },
		},
		{
			Name: "c",
			Do:   func(context.Context) error { return boom },
			Undo: func(context.Context) error { undoCount++; return nil },
		},
	}

	err := Run(ctx, steps)
	if !errors.Is(err, boom) {
		t.Fatalf("expected boom, got %v", err)
	}
	// Both "a" and "b" should have had Undo called despite undo errors.
	if undoCount != 2 {
		t.Errorf("expected 2 undo calls, got %d", undoCount)
	}
}

func TestRun_EmptySteps(t *testing.T) {
	if err := Run(ctx, nil); err != nil {
		t.Fatalf("empty saga should not error: %v", err)
	}
}
