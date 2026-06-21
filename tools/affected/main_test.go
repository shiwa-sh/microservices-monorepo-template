package main

import (
	"reflect"
	"testing"
)

func TestClassify(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		files []string
		want  Manifest
	}{
		{
			name:  "empty diff",
			files: nil,
			want:  Manifest{Services: []string{}, Apps: []string{}, Libs: []string{}},
		},
		{
			name:  "single service file",
			files: []string{"services/catalog/internal/handlers/products.go"},
			want: Manifest{
				Services: []string{"catalog"},
				Apps:     []string{},
				Libs:     []string{},
			},
		},
		{
			name: "multiple services + libs",
			files: []string{
				"services/orders/openapi.yaml",
				"services/catalog/migrations/001_init.sql",
				"libs/go/observability/init.go",
			},
			want: Manifest{
				Services: []string{"catalog", "orders"},
				Apps:     []string{},
				Libs:     []string{"observability"},
			},
		},
		{
			name:  "frontend app",
			files: []string{"apps/frontend/src/app/(panel)/page.tsx"},
			want: Manifest{
				Services: []string{},
				Apps:     []string{"frontend"},
				Libs:     []string{},
			},
		},
		{
			name:  "sdk change marks underlying service",
			files: []string{"libs/go/sdks/payment/client/client.gen.go"},
			want: Manifest{
				Services: []string{"payment"},
				Apps:     []string{},
				Libs:     []string{},
			},
		},
		{
			name:  "go.mod triggers global",
			files: []string{"go.mod"},
			want: Manifest{
				Global: true, Reason: "global trigger: go.mod",
				Services: []string{}, Apps: []string{}, Libs: []string{},
			},
		},
		{
			name:  "infra change triggers global",
			files: []string{"infra/helm/platform/postgres/values.yaml"},
			want: Manifest{
				Global: true, Reason: "global trigger: infra/helm/platform/postgres/values.yaml",
				Services: []string{}, Apps: []string{}, Libs: []string{},
			},
		},
		{
			name:  "tools change triggers global",
			files: []string{"tools/affected/main.go"},
			want: Manifest{
				Global: true, Reason: "global trigger: tools/affected/main.go",
				Services: []string{}, Apps: []string{}, Libs: []string{},
			},
		},
	}

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				t.Parallel()
				got := classify(tc.files, false)
				if !reflect.DeepEqual(got, tc.want) {
					t.Errorf("classify(%v) =\n  %+v\nwant\n  %+v", tc.files, got, tc.want)
				}
			},
		)
	}
}

func TestClassifyAllFlag(t *testing.T) {
	t.Parallel()
	got := classify([]string{"services/orders/foo.go"}, true)
	if !got.Global {
		t.Errorf("--all should force Global=true, got %+v", got)
	}
}
