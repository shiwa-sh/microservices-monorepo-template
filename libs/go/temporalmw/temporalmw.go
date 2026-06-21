// Package temporalmw is the platform-default Temporal client/worker wiring
// (ADR-0006). Every server and worker calls NewClient / NewWorker; tracing,
// data converters, and identity all come pre-configured.
package temporalmw

import (
	"fmt"
	"os"

	"go.temporal.io/sdk/client"
	temporaloteltracer "go.temporal.io/sdk/contrib/opentelemetry"
	"go.temporal.io/sdk/interceptor"
	"go.temporal.io/sdk/worker"
)

// Address resolves the Temporal frontend host from $TEMPORAL_HOST_PORT.
func Address() string {
	v := os.Getenv("TEMPORAL_HOST_PORT")
	if v != "" {
		return v
	}
	return "temporal-frontend.platform.svc.cluster.local:7233"
}

// Namespace resolves $TEMPORAL_NAMESPACE, defaulting to "default".
func Namespace() string {
	v := os.Getenv("TEMPORAL_NAMESPACE")
	if v != "" {
		return v
	}
	return "default"
}

// NewClient dials Temporal with the platform interceptors attached.
func NewClient(serviceName string) (client.Client, error) {
	tracingInterceptor, err := temporaloteltracer.NewTracingInterceptor(temporaloteltracer.TracerOptions{})
	if err != nil {
		return nil, fmt.Errorf("temporalmw: new tracing interceptor: %w", err)
	}
	c, err := client.Dial(
		client.Options{
			HostPort:     Address(),
			Namespace:    Namespace(),
			Identity:     serviceName,
			Interceptors: []interceptor.ClientInterceptor{tracingInterceptor},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("temporalmw: dial: %w", err)
	}
	return c, nil
}

// NewWorker constructs a Temporal worker with the platform-default options.
func NewWorker(c client.Client, taskQueue string) worker.Worker {
	return worker.New(
		c,
		taskQueue,
		worker.Options{
			EnableSessionWorker:                true,
			MaxConcurrentActivityExecutionSize: 50,
		},
	)
}
