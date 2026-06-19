// Package authmw verifies JWTs forwarded by the gateway (ADR-0009, ADR-0010).
// The gateway has already validated the token; the service re-checks the
// signature against Hydra's JWKS as a defense-in-depth measure.
package authmw

import (
	"context"
	"net/http"
	"os"

	"github.com/tabmadi/microservices-monorepo-template/libs/go/apierr"

	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

type ctxKey int

const claimsKey ctxKey = 1

// Claims is the subset of JWT claims services consume.
type Claims struct {
	UserID  string
	OrgID   string
	Roles   []string
	Subject string
}

// FromContext returns the claims attached by Verify, or nil if absent.
func FromContext(ctx context.Context) (*Claims, bool) {
	c, ok := ctx.Value(claimsKey).(*Claims)
	return c, ok
}

// Verify returns middleware that requires a valid JWT on every request.
// The JWKS URL is read from $HYDRA_JWKS_URL; absent it defaults to Hydra's
// in-cluster public endpoint.
func Verify() func(http.Handler) http.Handler {
	jwksURL := os.Getenv("HYDRA_JWKS_URL")
	if jwksURL == "" {
		jwksURL = "http://hydra-public.platform.svc.cluster.local/.well-known/jwks.json"
	}
	cache := jwk.NewCache(context.Background())
	_ = cache.Register(jwksURL)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := bearer(r)
			if raw == "" {
				apierr.Unauthorized().Write(w)
				return
			}
			set, err := cache.Get(r.Context(), jwksURL)
			if err != nil {
				apierr.Internal("jwks fetch").Write(w)
				return
			}
			tok, err := jwt.Parse([]byte(raw), jwt.WithKeySet(set), jwt.WithValidate(true))
			if err != nil {
				apierr.Unauthorized().Write(w)
				return
			}
			c := &Claims{Subject: tok.Subject()}
			v, ok := tok.Get("user_id")
			if ok {
				c.UserID, _ = v.(string)
			}
			v, ok = tok.Get("org_id")
			if ok {
				c.OrgID, _ = v.(string)
			}
			v, ok = tok.Get("roles")
			if ok {
				arr, ok := v.([]any)
				if ok {
					for _, x := range arr {
						s, ok := x.(string)
						if ok {
							c.Roles = append(c.Roles, s)
						}
					}
				}
			}
			ctx := context.WithValue(r.Context(), claimsKey, c)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const p = "Bearer "
	if len(h) > len(p) && h[:len(p)] == p {
		return h[len(p):]
	}
	return ""
}
