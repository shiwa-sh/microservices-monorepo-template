package workflows_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/converter"
	"go.temporal.io/sdk/testsuite"

	"github.com/tabmadi/microservices-monorepo-template/services/platform/internal/workflows"
)

const testIdentity = "019a3f8c-6d21-7c4b-8e55-0f27f7f0b001"

func erasureEnv(ts *testsuite.WorkflowTestSuite) *testsuite.TestWorkflowEnvironment {
	env := ts.NewTestWorkflowEnvironment()
	env.RegisterActivityWithOptions(
		func(context.Context, string, string) error { return nil },
		activity.RegisterOptions{Name: "EraseServiceDataActivity"},
	)
	env.RegisterActivityWithOptions(
		func(context.Context, string) error { return nil },
		activity.RegisterOptions{Name: "EraseIdentityActivity"},
	)
	env.RegisterActivityWithOptions(
		func(context.Context, string) error { return nil },
		activity.RegisterOptions{Name: "EraseAuthzTuplesActivity"},
	)
	return env
}

// The ordering is the property worth pinning: while the authz tuples exist the
// services can still answer questions about the subject, which is what makes a
// failed run safe to retry. A change that removed them first would leave rows
// nothing can reach — erased in effect, invisible to the retry meant to erase them.
func TestEraseSubjectRemovesTuplesLast(t *testing.T) {
	t.Parallel()
	var ts testsuite.WorkflowTestSuite
	env := erasureEnv(&ts)

	var order []string
	env.SetOnActivityStartedListener(
		func(info *activity.Info, _ context.Context, _ converter.EncodedValues) {
			order = append(order, info.ActivityType.Name)
		},
	)

	env.ExecuteWorkflow(workflows.EraseSubject, workflows.EraseSubjectInput{IdentityID: testIdentity})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	require.NotEmpty(t, order)
	require.Equal(t, "EraseAuthzTuplesActivity", order[len(order)-1])
}

// A store that fails must stop the run rather than let it continue: a partial
// erasure that reports success is the failure ADR-0301 names as worse than an
// unstarted one.
func TestEraseSubjectStopsOnFailure(t *testing.T) {
	t.Parallel()
	var ts testsuite.WorkflowTestSuite
	env := erasureEnv(&ts)
	env.OnActivity("EraseServiceDataActivity", mock.Anything, mock.Anything, mock.Anything).
		Return(storeError{})

	env.ExecuteWorkflow(workflows.EraseSubject, workflows.EraseSubjectInput{IdentityID: testIdentity})

	require.True(t, env.IsWorkflowCompleted())
	require.Error(t, env.GetWorkflowError())
	env.AssertNotCalled(t, "EraseAuthzTuplesActivity", mock.Anything, mock.Anything)
}

// storeError stands in for a store that is unreachable, which is the failure the
// workflow must stop on rather than continue past.
type storeError struct{}

func (storeError) Error() string { return "store unavailable" }
