package cyclops

import "context"
import "encoding/json"
import "net/http"
import "net/http/httptest"
import "reflect"
import "strings"
import "testing"
import "github.com/MikeTaylor/catlogger"
import "github.com/go-chi/chi/v5"
import "github.com/indexdata/ccms"

// helper to attach chi route context (since chi.URLParam depends on it)
func contextWithChiRouteContext(ctx context.Context, rctx *chi.Context) context.Context {
	return context.WithValue(ctx, chi.RouteCtxKey, rctx)
}

// fakeCCMS is a stand-in for *ccms.Client. It records the command it was asked
// to send and returns a pre-built response (or error), so handlers can be
// exercised without a real CCMS server.
type fakeCCMS struct {
	lastCmd string
	resp    *ccms.Response
	err     error
}

func (f *fakeCCMS) Send(cmd string) (*ccms.Response, error) {
	f.lastCmd = cmd
	return f.resp, f.err
}

// newTestServer wires a ModCyclopsServer to the given fake CCMS client.
func newTestServer(fake *fakeCCMS) *ModCyclopsServer {
	logger := catlogger.MakeLogger("", "", false)
	return MakeModCyclopsServer(logger, fake, ".", 60)
}

// retrieveRequest builds a GET request carrying the chi {setName} route param
// and the given raw query string (without leading '?').
func retrieveRequest(setName, rawQuery string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/test?"+rawQuery, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("setName", setName)
	return req.WithContext(contextWithChiRouteContext(req.Context(), rctx))
}

// assertEqual fails the test, labelled with what, when got != want.
func assertEqual(t *testing.T, what, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %q want %q", what, got, want)
	}
}

func TestHandleRetrieve(t *testing.T) {
	// Build the CCMS response the fake will return, using the ccms package's
	// own constructors.
	result := ccms.NewResult("ok")
	result.AddField("id", "string")
	result.AddField("name", "string")
	result.AddData([]any{"1", "Alice"})
	result.AddData([]any{"2", "Bob"})
	resp := ccms.NewResponse()
	resp.AddResult(result)

	fake := &fakeCCMS{resp: resp}
	server := newTestServer(fake)

	rr := httptest.NewRecorder()
	err := server.handleRetrieve(rr, retrieveRequest("users", "fields=id,name"), "retrieve")
	if err != nil {
		t.Fatalf("handleRetrieve returned error: %v", err)
	}

	// (a) the command actually sent to CCMS
	assertEqual(t, "command sent to CCMS", fake.lastCmd, "select id,name from users limit 100;")

	// (b) the translated JSON response
	var got RetrieveResponse
	err = json.Unmarshal(rr.Body.Bytes(), &got)
	if err != nil {
		t.Fatalf("could not decode response body %q: %v", rr.Body.String(), err)
	}
	want := RetrieveResponse{
		Status: "ok",
		Fields: []FieldDescription{{Name: "id"}, {Name: "name"}},
		Data:   []DataRow{{Values: []any{"1", "Alice"}}, {Values: []any{"2", "Bob"}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("translated response:\n got %+v\nwant %+v", got, want)
	}
}

func TestHandleRetrieveCountOnly(t *testing.T) {
	result := ccms.NewResult("ok")
	result.AddField("count", "bigint")
	result.AddData([]any{int64(42)})
	resp := ccms.NewResponse()
	resp.AddResult(result)

	fake := &fakeCCMS{resp: resp}
	server := newTestServer(fake)

	rr := httptest.NewRecorder()
	err := server.handleRetrieve(rr, retrieveRequest("users", "fields=id&countOnly=true"), "retrieve")
	if err != nil {
		t.Fatalf("handleRetrieve returned error: %v", err)
	}

	assertEqual(t, "command sent to CCMS", fake.lastCmd, "select COUNT(*) from users limit 100;")

	// The translated JSON response. Note the count comes back as float64:
	// encoding/json decodes JSON numbers into float64 when the target is any.
	var got RetrieveResponse
	err = json.Unmarshal(rr.Body.Bytes(), &got)
	if err != nil {
		t.Fatalf("could not decode response body %q: %v", rr.Body.String(), err)
	}
	want := RetrieveResponse{
		Status: "ok",
		Fields: []FieldDescription{{Name: "count"}},
		Data:   []DataRow{{Values: []any{float64(42)}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("translated response:\n got %+v\nwant %+v", got, want)
	}
}

func TestHandleRetrieveCCMSError(t *testing.T) {
	result := ccms.NewResult("error")
	result.AddMessage("no such set")
	resp := ccms.NewResponse()
	resp.AddResult(result)

	fake := &fakeCCMS{resp: resp}
	server := newTestServer(fake)

	rr := httptest.NewRecorder()
	err := server.handleRetrieve(rr, retrieveRequest("nope", "fields=id"), "retrieve")
	if err == nil {
		t.Fatal("expected an error for a CCMS error status, got nil")
	}
	if !strings.Contains(err.Error(), "no such set") {
		t.Errorf("error %q should mention the CCMS message %q", err.Error(), "no such set")
	}

	// On the error path the handler returns before writing any response, so the
	// caller can render the error itself. Confirm nothing was written.
	if rr.Body.Len() != 0 {
		t.Errorf("expected no response body on error, got %q", rr.Body.String())
	}
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

			assertEqual(t, "retrieve command", got, tt.expected)
		})
	}
}
