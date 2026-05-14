// orchestrator.go — Saga orchestrator: runs steps and compensates on failure.
//
// A Saga is a sequence of Steps. Each Step has:
//   - an Action:      the forward operation (charge, reserve, ship)
//   - a Compensation: the undo operation (refund, release, cancel)
//
// The Orchestrator runs Actions in order. If any Action fails, it runs
// Compensations for all previously completed steps in reverse order.
//
// Example:
//   Step 1: Action=Charge,   Compensation=Refund
//   Step 2: Action=Reserve,  Compensation=Release
//   Step 3: Action=CreateShipment, Compensation=CancelShipment
//
//   If step 3 fails:
//     Run step 2 Compensation (Release)
//     Run step 1 Compensation (Refund)

package saga

import (
	"fmt"
)

// Step is one unit of work in a saga — an action plus its undo.
type Step struct {
	Name         string       // human-readable name for logging
	Action       func() error // the forward operation
	Compensation func() error // the undo operation (run if a later step fails)
}

// Result captures what happened during the saga run.
type Result struct {
	Succeeded   bool     // true if all steps succeeded
	FailedStep  string   // which step caused the failure (empty if success)
	Error       error    // the error from the failed step
	Compensated []string // which compensations were run
	CompErrors  []error  // any errors from compensations themselves
}

// Orchestrator runs a list of saga steps in order and handles failures.
type Orchestrator struct {
	steps []Step
}

// New creates an orchestrator with the given steps.
// Steps are run in the order they appear in the slice.
func New(steps []Step) *Orchestrator {
	return &Orchestrator{steps: steps}
}

// Run executes all steps in order.
//
// If all steps succeed, returns a Result with Succeeded=true.
//
// If step N fails:
//   - Compensations for steps N-1, N-2, ... 0 are run in reverse order.
//   - Returns a Result with Succeeded=false, FailedStep, Error, and Compensated list.
//
// Compensation errors are recorded but don't stop other compensations from running
// — we try to undo as much as possible even if some compensations fail.
func (o *Orchestrator) Run() Result {
	completed := make([]int, 0, len(o.steps)) // indices of steps that succeeded

	for i, step := range o.steps {
		if err := step.Action(); err != nil {
			// This step failed — compensate all completed steps in reverse order
			return o.compensate(i, err, completed)
		}
		// Step succeeded — remember it so we can compensate it if needed
		completed = append(completed, i)
	}

	// All steps succeeded
	return Result{Succeeded: true}
}

// compensate runs compensations for all completed steps in reverse order.
func (o *Orchestrator) compensate(failedIdx int, failedErr error, completed []int) Result {
	result := Result{
		Succeeded:  false,
		FailedStep: o.steps[failedIdx].Name,
		Error:      failedErr,
	}

	// Run compensations in reverse order: last completed → first
	for i := len(completed) - 1; i >= 0; i-- {
		stepIdx := completed[i]
		step := o.steps[stepIdx]

		result.Compensated = append(result.Compensated, step.Name)

		if step.Compensation != nil {
			if err := step.Compensation(); err != nil {
				// Record the error but continue compensating other steps
				result.CompErrors = append(result.CompErrors,
					fmt.Errorf("compensation for %q failed: %w", step.Name, err),
				)
			}
		}
	}

	return result
}
