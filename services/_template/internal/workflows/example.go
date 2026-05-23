//go:build _template

// One workflow per file, named after the business process (ADR-0006).
package workflows

import (
	"time"

	"go.temporal.io/sdk/workflow"
)

// Example is the template workflow. Replace with the service's real workflow.
func Example(ctx workflow.Context, input string) (string, error) {
	ao := workflow.ActivityOptions{ScheduleToCloseTimeout: 30 * time.Second}
	ctx = workflow.WithActivityOptions(ctx, ao)
	var out string
	if err := workflow.ExecuteActivity(ctx, "DoStuff", input).Get(ctx, &out); err != nil {
		return "", err
	}
	return out, nil
}
