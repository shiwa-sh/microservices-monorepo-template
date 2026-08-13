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

const testID = "id_1"

// registerEnv registers stub activities under the names RegisterUser executes, so
// the test env resolves and mocks them without the real orgs DB / OpenFGA writer.
func registerEnv(ts *testsuite.WorkflowTestSuite) *testsuite.TestWorkflowEnvironment {
	env := ts.NewTestWorkflowEnvironment()
	env.RegisterActivityWithOptions(
		func(context.Context, string) (string, error) { return "", nil },
		activity.RegisterOptions{Name: "CreatePersonalOrgActivity"},
	)
	env.RegisterActivityWithOptions(
		func(context.Context, string, string) error { return nil },
		activity.RegisterOptions{Name: "GrantOrgAdminActivity"},
	)
	env.RegisterActivityWithOptions(
		func(context.Context, string, string) error { return nil },
		activity.RegisterOptions{Name: "SetIdentityOrgActivity"},
	)
	return env
}

func TestRegisterUserWorkflow(t *testing.T) {
	t.Parallel()

	t.Run(
		"creates the personal org then grants admin on it",
		func(t *testing.T) {
			t.Parallel()
			var ts testsuite.WorkflowTestSuite
			env := registerEnv(&ts)
			env.OnActivity("CreatePersonalOrgActivity", mock.Anything, testID).
				Return("org_1", nil).Once()
			env.OnActivity("GrantOrgAdminActivity", mock.Anything, "org_1", testID).
				Return(nil).Once()
			env.OnActivity("SetIdentityOrgActivity", mock.Anything, testID, "org_1").
				Return(nil).Once()

			env.ExecuteWorkflow(RegisterUser, RegisterInput{IdentityID: testID})

			require.True(t, env.IsWorkflowCompleted())
			require.NoError(t, env.GetWorkflowError())
			env.AssertExpectations(t)
		},
	)

	t.Run(
		"org creation failure fails the workflow and never grants admin",
		func(t *testing.T) {
			t.Parallel()
			var ts testsuite.WorkflowTestSuite
			env := registerEnv(&ts)
			env.OnActivity("CreatePersonalOrgActivity", mock.Anything, mock.Anything).
				Return("", errors.New("db down"))

			env.ExecuteWorkflow(RegisterUser, RegisterInput{IdentityID: testID})

			require.True(t, env.IsWorkflowCompleted())
			require.Error(t, env.GetWorkflowError())
			env.AssertNotCalled(t, "GrantOrgAdminActivity", mock.Anything, mock.Anything, mock.Anything)
		},
	)

	// The identity is where X-Org-Id comes from (ADR-0304), so an org the edge never
	// learns about is the same as no org at all — the workflow has to fail on it.
	t.Run(
		"identity metadata failure fails the workflow",
		func(t *testing.T) {
			t.Parallel()
			var ts testsuite.WorkflowTestSuite
			env := registerEnv(&ts)
			env.OnActivity("CreatePersonalOrgActivity", mock.Anything, mock.Anything).
				Return("org_1", nil).Once()
			env.OnActivity("GrantOrgAdminActivity", mock.Anything, "org_1", testID).
				Return(nil).Once()
			env.OnActivity("SetIdentityOrgActivity", mock.Anything, testID, "org_1").
				Return(errors.New("kratos down"))

			env.ExecuteWorkflow(RegisterUser, RegisterInput{IdentityID: testID})

			require.True(t, env.IsWorkflowCompleted())
			require.Error(t, env.GetWorkflowError())
		},
	)

	t.Run(
		"OpenFGA grant failure fails the workflow after the org is created",
		func(t *testing.T) {
			t.Parallel()
			var ts testsuite.WorkflowTestSuite
			env := registerEnv(&ts)
			env.OnActivity("CreatePersonalOrgActivity", mock.Anything, mock.Anything).
				Return("org_1", nil).Once()
			env.OnActivity("GrantOrgAdminActivity", mock.Anything, "org_1", testID).
				Return(errors.New("openfga down"))

			env.ExecuteWorkflow(RegisterUser, RegisterInput{IdentityID: testID})

			require.True(t, env.IsWorkflowCompleted())
			require.Error(t, env.GetWorkflowError())
		},
	)
}
