package workflows

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
)

const (
	testEmail    = "ops@example.com"
	testPassword = "correct horse battery staple"
	testID       = "019a3f8c-6d21-7c4b-8e55-0f27f7f0b001"
)

// registerEnv registers stub activities under the names RegisterOperator executes,
// so the test env resolves and mocks them without a real Kratos or OpenFGA.
func registerEnv(ts *testsuite.WorkflowTestSuite) *testsuite.TestWorkflowEnvironment {
	env := ts.NewTestWorkflowEnvironment()
	env.RegisterActivityWithOptions(
		func(context.Context, string, string) (string, error) { return "", nil },
		activity.RegisterOptions{Name: "CreateOperatorIdentityActivity"},
	)
	env.RegisterActivityWithOptions(
		func(context.Context, string) error { return nil },
		activity.RegisterOptions{Name: "GrantOperatorRoleActivity"},
	)
	return env
}

func TestRegisterOperatorWorkflow(t *testing.T) {
	t.Parallel()

	t.Run(
		"creates the identity then grants it the operator role",
		func(t *testing.T) {
			t.Parallel()
			var ts testsuite.WorkflowTestSuite
			env := registerEnv(&ts)
			env.OnActivity("CreateOperatorIdentityActivity", mock.Anything, testEmail, testPassword).
				Return(testID, nil).Once()
			env.OnActivity("GrantOperatorRoleActivity", mock.Anything, testID).
				Return(nil).Once()

			env.ExecuteWorkflow(RegisterOperator, RegisterOperatorInput{Email: testEmail, Password: testPassword})

			require.True(t, env.IsWorkflowCompleted())
			require.NoError(t, env.GetWorkflowError())
			env.AssertExpectations(t)
		},
	)

	t.Run(
		"identity failure fails the workflow and never grants the role",
		func(t *testing.T) {
			t.Parallel()
			var ts testsuite.WorkflowTestSuite
			env := registerEnv(&ts)
			env.OnActivity("CreateOperatorIdentityActivity", mock.Anything, mock.Anything, mock.Anything).
				Return("", errors.New("kratos down"))

			env.ExecuteWorkflow(RegisterOperator, RegisterOperatorInput{Email: testEmail, Password: testPassword})

			require.True(t, env.IsWorkflowCompleted())
			require.Error(t, env.GetWorkflowError())
			env.AssertNotCalled(t, "GrantOperatorRoleActivity", mock.Anything, mock.Anything)
		},
	)

	t.Run(
		// The state the workflow exists to prevent: an identity that can sign in
		// and is refused by every ops tool. A grant failure must fail the run so
		// it is visible, rather than returning success with half the write done.
		"a grant failure fails the run rather than leaving a half-registered operator",
		func(t *testing.T) {
			t.Parallel()
			var ts testsuite.WorkflowTestSuite
			env := registerEnv(&ts)
			env.OnActivity("CreateOperatorIdentityActivity", mock.Anything, mock.Anything, mock.Anything).
				Return(testID, nil).Once()
			env.OnActivity("GrantOperatorRoleActivity", mock.Anything, testID).
				Return(errors.New("openfga down"))

			env.ExecuteWorkflow(RegisterOperator, RegisterOperatorInput{Email: testEmail, Password: testPassword})

			require.True(t, env.IsWorkflowCompleted())
			require.Error(t, env.GetWorkflowError())
		},
	)
}
