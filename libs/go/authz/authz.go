// Package authz is the shared SpiceDB client wrapper (ADR-0010). Services
// never import authzed-go directly; they call Checker.Allowed(...).
package authz

import (
	"context"
	"errors"
	"fmt"
	"os"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/authzed-go/v1"
	"github.com/authzed/grpcutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Checker is the only authz surface service code uses.
type Checker interface {
	Allowed(ctx context.Context, subject, permission, resource string) (bool, error)
}

type spice struct{ c *authzed.Client }

// New dials the cluster SpiceDB.
//
// SPICEDB_ENDPOINT and SPICEDB_PRESHARED_KEY are required env vars
// (typically envFrom-mounted from a SOPS Secret — ADR-0005).
func New() (Checker, error) {
	endpoint := os.Getenv("SPICEDB_ENDPOINT")
	if endpoint == "" {
		endpoint = "spicedb.platform.svc.cluster.local:50051"
	}
	psk := os.Getenv("SPICEDB_PRESHARED_KEY")
	if psk == "" {
		return nil, errors.New("SPICEDB_PRESHARED_KEY not set")
	}
	c, err := authzed.NewClient(
		endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpcutil.WithInsecureBearerToken(psk),
	)
	if err != nil {
		return nil, fmt.Errorf("dial spicedb: %w", err)
	}
	return &spice{c: c}, nil
}

// Allowed runs a CheckPermission against SpiceDB.
// subject  = "user:alice"
// resource = "order:o1"
// permission = "read".
func (s *spice) Allowed(ctx context.Context, subject, permission, resource string) (bool, error) {
	subT, subID, err := split(subject)
	if err != nil {
		return false, err
	}
	resT, resID, err := split(resource)
	if err != nil {
		return false, err
	}

	r, err := s.c.CheckPermission(
		ctx,
		&v1.CheckPermissionRequest{
			Resource:   &v1.ObjectReference{ObjectType: resT, ObjectId: resID},
			Permission: permission,
			Subject:    &v1.SubjectReference{Object: &v1.ObjectReference{ObjectType: subT, ObjectId: subID}},
		},
	)
	if err != nil {
		return false, fmt.Errorf("authz: check permission: %w", err)
	}
	return r.GetPermissionship() == v1.CheckPermissionResponse_PERMISSIONSHIP_HAS_PERMISSION, nil
}

// split converts "type:id" → ("type", "id").
func split(s string) (string, string, error) {
	for i := range len(s) {
		if s[i] == ':' {
			return s[:i], s[i+1:], nil
		}
	}
	return "", "", fmt.Errorf("authz: invalid id %q (expected type:id)", s)
}
