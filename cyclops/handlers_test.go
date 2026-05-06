package cyclops

import "context"
import "net/http"
import "net/http/httptest"
import "testing"
import "github.com/go-chi/chi/v5"

// helper to attach chi route context (since chi.URLParam depends on it)
func contextWithChiRouteContext(ctx context.Context, rctx *chi.Context) context.Context {
	return context.WithValue(ctx, chi.RouteCtxKey, rctx)
}

func TestMakeRetrieveCommand(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		setName     string
		expected    string
		expectedErr string
	}{
		{
			name:        "basic query with required fields",
			url:         "/test?fields=id,name",
			setName:     "users",
			expected:    "select id,name from users limit 100;",
			expectedErr: "",
		},
		{
			name:        "missing fields should error",
			url:         "/test",
			setName:     "users",
			expectedErr: "no 'fields' parameter supplied",
		},
		{
			name:        "with condition and filter",
			url:         "/test?fields=id&cond=age>18&filter=active",
			setName:     "users",
			expected:    "select id from users where age>18 filter active limit 100;",
			expectedErr: "",
		},
		{
			name:        "with tag",
			url:         "/test?fields=id&tag=vip",
			setName:     "users",
			expected:    "select id from users tag vip limit 100;",
			expectedErr: "",
		},
		{
			name:        "with omitTag",
			url:         "/test?fields=id&omitTag=vip",
			setName:     "users",
			expected:    "select id from users tag not vip limit 100;",
			expectedErr: "",
		},
		{
			name:        "both tag and omitTag should error",
			url:         "/test?fields=id&tag=vip&omitTag=vip",
			setName:     "users",
			expectedErr: "both 'tag' and 'omitTag' parameters supplied",
		},
		{
			name:        "with sort, limit and offset",
			url:         "/test?fields=id&sort=name&limit=10&offset=5",
			setName:     "users",
			expected:    "select id from users order by name limit 10 offset 5;",
			expectedErr: "",
		},
		{
			name:        "default limit applied",
			url:         "/test?fields=id",
			setName:     "users",
			expected:    "select id from users limit 100;",
			expectedErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)

			// Inject chi route param
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("setName", tt.setName)
			req = req.WithContext(
				contextWithChiRouteContext(req.Context(), rctx),
			)

			got, err := makeRetrieveCommand(req, false)

			if tt.expectedErr != "" {
				if err == nil {
					t.Fatalf("expected error but got none")
				} else if err.Error() != tt.expectedErr {
					t.Fatalf("expected error %q but got %q", tt.expectedErr, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tt.expected {
				t.Errorf("expected %q but got %q", tt.expected, got)
			}
		})
	}
}
