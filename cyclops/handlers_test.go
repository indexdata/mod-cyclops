package cyclops

import "context"
import "encoding/json"
import "errors"
import "fmt"
import "net/http"
import "net/http/httptest"
import "net/url"
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

// -----------------------------------------------------------------------------

// assertEqual fails the test, labelled with what, when got != want.
func assertEqual(t *testing.T, what, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %q want %q", what, got, want)
	}
}

// assertErrContains fails the test when err's message does not contain want.
// Callers are expected to have already established that err is non-nil.
func assertErrContains(t *testing.T, err error, want string) {
	t.Helper()
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error %q should contain %q", err.Error(), want)
	}
}

// assertStatus fails the test when the recorded HTTP status differs from want.
func assertStatus(t *testing.T, rr *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rr.Code != want {
		t.Errorf("status code: got %d want %d", rr.Code, want)
	}
}

// -----------------------------------------------------------------------------

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
	assertErrContains(t, err, "no such set")

	// On the error path the handler returns before writing any response, so the
	// caller can render the error itself. Confirm nothing was written.
	if rr.Body.Len() != 0 {
		t.Errorf("expected no response body on error, got %q", rr.Body.String())
	}
}

// assertHTTPStatus fails the test when err is not an *HTTPError carrying the
// wanted status. A client mistake must not be reported as a server fault.
func assertHTTPStatus(t *testing.T, err error, want int) {
	t.Helper()
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("error %v is a %T, want an *HTTPError with status %d", err, err, want)
	}
	if httpErr.status != want {
		t.Errorf("status: got %d want %d", httpErr.status, want)
	}
}

// The status an *HTTPError carries must be the status the client is actually
// sent. Handlers wrap the errors they return, so this only holds if
// runWithErrorHandling unwraps rather than type-asserting; when it did not,
// every 400 below reached the client as a 500.
func TestClientSeesHTTPErrorStatus(t *testing.T) {
	jsonCond := url.QueryEscape(`{"type":"xyzzy"}`)
	cases := []struct {
		name string
		run  func(*ModCyclopsServer, *httptest.ResponseRecorder)
		want int
	}{
		{"retrieve with a bad jsonCond", func(s *ModCyclopsServer, rr *httptest.ResponseRecorder) {
			req := retrieveRequest("users", "fields=id&jsonCond="+jsonCond)
			s.runWithErrorHandling(rr, req, s.handleRetrieve, "retrieve")
		}, http.StatusBadRequest},

		{"create filter with a bad jsonCond", func(s *ModCyclopsServer, rr *httptest.ResponseRecorder) {
			req := jsonRequest(`{"name":"active","jsonCond":{"type":"xyzzy"}}`, nil)
			s.runWithErrorHandling(rr, req, s.handleCreateFilter, "create filter")
		}, http.StatusBadRequest},

		{"add objects with a bad jsonCond", func(s *ModCyclopsServer, rr *httptest.ResponseRecorder) {
			req := jsonRequest(`{"from":"src","jsonCond":{"type":"xyzzy"}}`, map[string]string{"setName": "dest"})
			s.runWithErrorHandling(rr, req, s.handleAddObjects, "add objects")
		}, http.StatusBadRequest},

		{"remove objects with a bad jsonCond", func(s *ModCyclopsServer, rr *httptest.ResponseRecorder) {
			req := jsonRequest(`{"jsonCond":{"type":"xyzzy"}}`, map[string]string{"setName": "dest"})
			s.runWithErrorHandling(rr, req, s.handleRemoveObjects, "remove objects")
		}, http.StatusBadRequest},

		{"create filter with a bad template", func(s *ModCyclopsServer, rr *httptest.ResponseRecorder) {
			req := jsonRequest(`{"name":"active","template":"x; drop filter y"}`, nil)
			s.runWithErrorHandling(rr, req, s.handleCreateFilter, "create filter")
		}, http.StatusBadRequest},

		// An error that carries no status is still a server fault.
		{"CCMS unreachable", func(s *ModCyclopsServer, rr *httptest.ResponseRecorder) {
			req := retrieveRequest("users", "fields=id")
			s.runWithErrorHandling(rr, req, s.handleRetrieve, "retrieve")
		}, http.StatusInternalServerError},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fake := &fakeCCMS{resp: okResponse(), err: errors.New("connection refused")}
			server := newTestServer(fake)
			rr := httptest.NewRecorder()
			c.run(server, rr)
			assertStatus(t, rr, c.want)
		})
	}
}

// retrieveCommandFor runs handleRetrieve over the given query string and returns
// the command that reached CCMS.
func retrieveCommandFor(t *testing.T, rawQuery string) string {
	t.Helper()
	result := ccms.NewResult("ok")
	result.AddField("id", "string")
	resp := ccms.NewResponse()
	resp.AddResult(result)

	fake := &fakeCCMS{resp: resp}
	server := newTestServer(fake)
	err := server.handleRetrieve(httptest.NewRecorder(), retrieveRequest("users", rawQuery), "retrieve")
	if err != nil {
		t.Fatalf("handleRetrieve(%q) returned error: %v", rawQuery, err)
	}
	return fake.lastCmd
}

// A condition supplied as 'jsonCond' is rendered into the command by ParseCond,
// so the values it carries can only appear as quoted literals.
func TestHandleRetrieveJSONCond(t *testing.T) {
	cases := map[string]string{
		`{"type":"term","field":"title","rel":"contains","value":"world"}`: `title ilike '%world%'`,
		`{"type":"term","field":"holdings_count","rel":"ge","value":3}`:    `holdings_count >= 3`,
		`{"type":"term","field":"decision","rel":"eq","value":false}`:      `decision = false`,
		`{"type":"filter","name":"target"}`:                                `filter(target)`,
		`{"type":"and","clauses":[{"type":"term","field":"title","rel":"contains","value":"world"},` +
			`{"type":"term","field":"author","rel":"contains","value":"O'Brien"}]}`: `(title ilike '%world%' and author ilike '%O''Brien%')`,
		// The payload that motivated the whole exercise: it must survive as
		// data, matched literally, rather than becoming a second statement.
		`{"type":"term","field":"note","rel":"contains","value":"'; drop set users; --"}`: `note ilike '%''; drop set users; --%'`,
	}
	for jsonCond, wantCond := range cases {
		t.Run(wantCond, func(t *testing.T) {
			got := retrieveCommandFor(t, "fields=id&jsonCond="+url.QueryEscape(jsonCond))
			want := "select id from users where " + wantCond + " limit 100;"
			assertEqual(t, "command sent to CCMS", got, want)
		})
	}
}

// No condition at all means an unconditional retrieval.
func TestHandleRetrieveNoCond(t *testing.T) {
	got := retrieveCommandFor(t, "fields=id")
	assertEqual(t, "command sent to CCMS", got, "select id from users limit 100;")
}

// 'cond' is withdrawn: a client still sending one must be told so, rather than
// have its condition ignored and be given records it did not ask for. The
// second case is the only one anywhere that pairs the two parameters, and pins
// that a 'cond' is refused even when a usable 'jsonCond' accompanies it; the
// other entry points test the refusal itself, which is the same code path.
func TestHandleRetrieveCondWithdrawn(t *testing.T) {
	for _, rawQuery := range []string{
		"fields=id&cond=" + url.QueryEscape("title = 'x'"),
		"fields=id&cond=" + url.QueryEscape("title = 'x'") +
			"&jsonCond=" + url.QueryEscape(`{"type":"filter","name":"target"}`),
	} {
		t.Run(rawQuery, func(t *testing.T) {
			fake := &fakeCCMS{resp: okResponse()}
			server := newTestServer(fake)

			err := server.handleRetrieve(httptest.NewRecorder(), retrieveRequest("users", rawQuery), "retrieve")
			if err == nil {
				t.Fatal("expected an error when 'cond' is supplied")
			}
			assertHTTPStatus(t, err, http.StatusBadRequest)
			assertErrContains(t, err, "'cond' parameter has been withdrawn")
			if fake.lastCmd != "" {
				t.Errorf("a command was sent despite the bad request: %q", fake.lastCmd)
			}
		})
	}
}

// A 'jsonCond' that does not describe a condition is the client's mistake, and
// must be reported as such rather than as a server fault.
func TestHandleRetrieveJSONCondInvalid(t *testing.T) {
	cases := map[string]string{
		`title ilike '%world%'`: `not a condition clause`,
		`{}`:                    `clause has no "type"`,
		`{"type":"xyzzy"}`:      `unknown clause type "xyzzy"`,
		`{"type":"term","field":"title","rel":"ilike","value":"x"}`:              `unknown relation "ilike"`,
		`{"type":"term","field":"title; drop set users","rel":"eq","value":"x"}`: `invalid field identifier: "title; drop set users"`,
	}
	for jsonCond, wantErr := range cases {
		t.Run(wantErr, func(t *testing.T) {
			fake := &fakeCCMS{resp: okResponse()}
			server := newTestServer(fake)

			rawQuery := "fields=id&jsonCond=" + url.QueryEscape(jsonCond)
			err := server.handleRetrieve(httptest.NewRecorder(), retrieveRequest("users", rawQuery), "retrieve")
			if err == nil {
				t.Fatalf("expected an error for jsonCond=%s", jsonCond)
			}
			assertHTTPStatus(t, err, http.StatusBadRequest)
			assertErrContains(t, err, "invalid 'jsonCond'")
			assertErrContains(t, err, wantErr)
			if fake.lastCmd != "" {
				t.Errorf("a command was sent despite the bad request: %q", fake.lastCmd)
			}
		})
	}
}

// jsonRequest builds a request carrying the given chi URL params and a JSON
// body. Pass a nil params map when no route params are needed, and an empty
// body for handlers that don't read one.
func jsonRequest(body string, params map[string]string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	return req.WithContext(contextWithChiRouteContext(req.Context(), rctx))
}

// okResponse is the minimal non-error CCMS response. Handlers that only return
// 204 still run it through sendToCCMS, which dereferences the response, so it
// must be non-nil and carry a non-error result.
func okResponse() *ccms.Response {
	resp := ccms.NewResponse()
	resp.AddResult(ccms.NewResult("ok"))
	return resp
}

// listResponse builds an "ok" response whose single result has one data row per
// value, each a single-column string. This matches what handleShowTags reads.
func listResponse(values ...string) *ccms.Response {
	result := ccms.NewResult("ok")
	for _, v := range values {
		result.AddData([]any{v})
	}
	resp := ccms.NewResponse()
	resp.AddResult(result)
	return resp
}

// -----------------------------------------------------------------------------
// Read handlers: fixed command in, JSON list out.

func TestHandleShowTags(t *testing.T) {
	fake := &fakeCCMS{resp: listResponse("vip", "staff")}
	server := newTestServer(fake)

	rr := httptest.NewRecorder()
	err := server.handleShowTags(rr, jsonRequest("", nil), "show tags")
	if err != nil {
		t.Fatalf("handleShowTags returned error: %v", err)
	}

	assertEqual(t, "command sent to CCMS", fake.lastCmd, "show tags;")

	var got TagList
	err = json.Unmarshal(rr.Body.Bytes(), &got)
	if err != nil {
		t.Fatalf("could not decode response body %q: %v", rr.Body.String(), err)
	}
	want := TagList{Tags: []any{"vip", "staff"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("translated response:\n got %+v\nwant %+v", got, want)
	}
}

// namedResponse builds an "ok" response whose single result carries the named
// text fields and one data row per set of values. Tests that use it name the
// fields in an order other than that of the structure being built, since the
// handlers key the columns by name rather than by position.
func namedResponse(fields []string, rows ...[]any) *ccms.Response {
	result := ccms.NewResult("ok")
	for _, field := range fields {
		result.AddField(field, "text")
	}
	for _, row := range rows {
		result.AddData(row)
	}
	resp := ccms.NewResponse()
	resp.AddResult(result)
	return resp
}

func TestHandleShowFilters(t *testing.T) {
	fake := &fakeCCMS{resp: namedResponse(
		[]string{"filter", "definition", "project"},
		[]any{"active", "age > 18", "PROJ"},
		[]any{"archived", "status = 'old'", "OTHER"},
	)}
	server := newTestServer(fake)

	rr := httptest.NewRecorder()
	err := server.handleShowFilters(rr, jsonRequest("", nil), "show filters")
	if err != nil {
		t.Fatalf("handleShowFilters returned error: %v", err)
	}

	assertEqual(t, "command sent to CCMS", fake.lastCmd, "show filters;")

	var got FilterList
	err = json.Unmarshal(rr.Body.Bytes(), &got)
	if err != nil {
		t.Fatalf("could not decode response body %q: %v", rr.Body.String(), err)
	}
	want := FilterList{Filters: []FilterSummary{
		{Project: "PROJ", Filter: "active", Definition: "age > 18"},
		{Project: "OTHER", Filter: "archived", Definition: "status = 'old'"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("translated response:\n got %+v\nwant %+v", got, want)
	}
}

func TestHandleShowFiltersInProject(t *testing.T) {
	fake := &fakeCCMS{resp: namedResponse(
		[]string{"filter", "definition", "project"},
		[]any{"jurassic", "age > 18", "korea_lit"},
	)}
	server := newTestServer(fake)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test?project=korea_lit", nil)
	err := server.handleShowFilters(rr, req, "show filters")
	if err != nil {
		t.Fatalf("handleShowFilters returned error: %v", err)
	}

	assertEqual(t, "command sent to CCMS", fake.lastCmd, "show filters in project korea_lit;")

	var got FilterList
	err = json.Unmarshal(rr.Body.Bytes(), &got)
	if err != nil {
		t.Fatalf("could not decode response body %q: %v", rr.Body.String(), err)
	}
	want := FilterList{Filters: []FilterSummary{
		{Project: "korea_lit", Filter: "jurassic", Definition: "age > 18"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("translated response:\n got %+v\nwant %+v", got, want)
	}
}

func TestHandleShowFiltersInProjectBadIdentifier(t *testing.T) {
	fake := &fakeCCMS{resp: namedResponse([]string{"project", "filter", "definition"})}
	server := newTestServer(fake)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test?project=korea_lit%3B+drop+set+users", nil)
	err := server.handleShowFilters(rr, req, "show filters")
	if err == nil {
		t.Fatal("expected an error for an invalid project identifier, got nil")
	}
	assertErrContains(t, err, "invalid project identifier")
}

func TestHandleShowFiltersMissingField(t *testing.T) {
	fake := &fakeCCMS{resp: namedResponse(
		[]string{"project", "filter"},
		[]any{"PROJ", "active"},
	)}
	server := newTestServer(fake)

	rr := httptest.NewRecorder()
	err := server.handleShowFilters(rr, jsonRequest("", nil), "show filters")
	if err == nil {
		t.Fatal("expected an error for a response with no 'definition' field, got nil")
	}
	assertErrContains(t, err, "no 'definition' field")
}

func TestHandleShowSets(t *testing.T) {
	fake := &fakeCCMS{resp: namedResponse(
		[]string{"set", "title", "project"},
		[]any{"users", "All our users", "PROJ"},
		[]any{"books", "Books in the collection", "OTHER"},
	)}
	server := newTestServer(fake)

	rr := httptest.NewRecorder()
	err := server.handleShowSets(rr, jsonRequest("", nil), "show sets")
	if err != nil {
		t.Fatalf("handleShowSets returned error: %v", err)
	}

	assertEqual(t, "command sent to CCMS", fake.lastCmd, "show sets;")

	var got SetList
	err = json.Unmarshal(rr.Body.Bytes(), &got)
	if err != nil {
		t.Fatalf("could not decode response body %q: %v", rr.Body.String(), err)
	}
	want := SetList{Sets: []SetSummary{
		{Project: "PROJ", Set: "users", Title: "All our users"},
		{Project: "OTHER", Set: "books", Title: "Books in the collection"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("translated response:\n got %+v\nwant %+v", got, want)
	}
}

func TestHandleShowSetsMissingField(t *testing.T) {
	fake := &fakeCCMS{resp: namedResponse(
		[]string{"project", "set"},
		[]any{"PROJ", "users"},
	)}
	server := newTestServer(fake)

	rr := httptest.NewRecorder()
	err := server.handleShowSets(rr, jsonRequest("", nil), "show sets")
	if err == nil {
		t.Fatal("expected an error for a response with no 'title' field, got nil")
	}
	assertErrContains(t, err, "no 'title' field")
}

func TestHandleShowSetsInProject(t *testing.T) {
	fake := &fakeCCMS{resp: namedResponse(
		[]string{"set", "title", "project"},
		[]any{"object", "Objects of interest", "mike"},
		[]any{"endangered", "Endangered species", "mike"},
	)}
	server := newTestServer(fake)

	rr := httptest.NewRecorder()
	err := server.handleShowSetsInProject(rr, jsonRequest("", map[string]string{"projectId": "mike"}), "show sets in project")
	if err != nil {
		t.Fatalf("handleShowSets returned error: %v", err)
	}

	assertEqual(t, "command sent to CCMS", fake.lastCmd, "show sets in project mike;")

	var got SetList
	err = json.Unmarshal(rr.Body.Bytes(), &got)
	if err != nil {
		t.Fatalf("could not decode response body %q: %v", rr.Body.String(), err)
	}
	want := SetList{Sets: []SetSummary{
		{Project: "mike", Set: "object", Title: "Objects of interest"},
		{Project: "mike", Set: "endangered", Title: "Endangered species"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("translated response:\n got %+v\nwant %+v", got, want)
	}
}

func TestHandleShowFunds(t *testing.T) {
	result := ccms.NewResult("ok")
	result.AddData([]any{"general", "General Fund"})
	result.AddData([]any{"endowment", "Endowment Fund"})
	resp := ccms.NewResponse()
	resp.AddResult(result)
	fake := &fakeCCMS{resp: resp}
	server := newTestServer(fake)

	rr := httptest.NewRecorder()
	err := server.handleShowFunds(rr, jsonRequest("", nil), "show funds")
	if err != nil {
		t.Fatalf("handleShowFunds returned error: %v", err)
	}

	assertEqual(t, "command sent to CCMS", fake.lastCmd, "show funds;")

	var got FundList
	err = json.Unmarshal(rr.Body.Bytes(), &got)
	if err != nil {
		t.Fatalf("could not decode response body %q: %v", rr.Body.String(), err)
	}
	want := FundList{Funds: []Fund{
		{Id: "general", Name: "General Fund"},
		{Id: "endowment", Name: "Endowment Fund"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("translated response:\n got %+v\nwant %+v", got, want)
	}
}

func TestHandleShowProjects(t *testing.T) {
	result := ccms.NewResult("ok")
	result.AddData([]any{"alpha", "Project Alpha"})
	result.AddData([]any{"beta", "Project Beta"})
	resp := ccms.NewResponse()
	resp.AddResult(result)
	fake := &fakeCCMS{resp: resp}
	server := newTestServer(fake)

	rr := httptest.NewRecorder()
	err := server.handleShowProjects(rr, jsonRequest("", nil), "show projects")
	if err != nil {
		t.Fatalf("handleShowProjects returned error: %v", err)
	}

	assertEqual(t, "command sent to CCMS", fake.lastCmd, "show projects;")

	var got ProjectList
	err = json.Unmarshal(rr.Body.Bytes(), &got)
	if err != nil {
		t.Fatalf("could not decode response body %q: %v", rr.Body.String(), err)
	}
	want := ProjectList{Projects: []BriefProject{
		{Id: "alpha", Name: "Project Alpha"},
		{Id: "beta", Name: "Project Beta"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("translated response:\n got %+v\nwant %+v", got, want)
	}
}

// -----------------------------------------------------------------------------
// Body-driven handlers that return 204 No Content.

func TestHandleDefineTag(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		fake := &fakeCCMS{resp: okResponse()}
		server := newTestServer(fake)

		rr := httptest.NewRecorder()
		err := server.handleDefineTag(rr, jsonRequest(`{"name":"vip"}`, nil), "define tag")
		if err != nil {
			t.Fatalf("handleDefineTag returned error: %v", err)
		}

		assertEqual(t, "command sent to CCMS", fake.lastCmd, "define tag vip")
		assertStatus(t, rr, http.StatusNoContent)
	})

	t.Run("malformed JSON body", func(t *testing.T) {
		fake := &fakeCCMS{resp: okResponse()}
		server := newTestServer(fake)

		rr := httptest.NewRecorder()
		err := server.handleDefineTag(rr, jsonRequest(`{"name":`, nil), "define tag")
		if err == nil {
			t.Fatal("expected an error for malformed JSON, got nil")
		}
		assertErrContains(t, err, "deserialize JSON")

		// The handler must bail before sending anything to CCMS.
		assertEqual(t, "command sent to CCMS", fake.lastCmd, "")
	})

	t.Run("CCMS error status", func(t *testing.T) {
		result := ccms.NewResult("error")
		result.AddMessage("tag already exists")
		resp := ccms.NewResponse()
		resp.AddResult(result)

		fake := &fakeCCMS{resp: resp}
		server := newTestServer(fake)

		rr := httptest.NewRecorder()
		err := server.handleDefineTag(rr, jsonRequest(`{"name":"vip"}`, nil), "define tag")
		if err == nil {
			t.Fatal("expected an error for a CCMS error status, got nil")
		}
		assertErrContains(t, err, "tag already exists")

		// The command is sent before CCMS reports the error, but no 204 follows.
		assertEqual(t, "command sent to CCMS", fake.lastCmd, "define tag vip")
		if rr.Body.Len() != 0 {
			t.Errorf("expected no response body on error, got %q", rr.Body.String())
		}
	})

	t.Run("CCMS unreachable", func(t *testing.T) {
		fake := &fakeCCMS{err: errors.New("connection refused")}
		server := newTestServer(fake)

		rr := httptest.NewRecorder()
		err := server.handleDefineTag(rr, jsonRequest(`{"name":"vip"}`, nil), "define tag")
		if err == nil {
			t.Fatal("expected an error when CCMS is unreachable, got nil")
		}
		assertErrContains(t, err, "connection refused")

		assertEqual(t, "command sent to CCMS", fake.lastCmd, "define tag vip")
	})
}

func TestHandleUpdateRecord(t *testing.T) {
	t.Run("simple case with non-faceted set-name", func(t *testing.T) {
		fake := &fakeCCMS{resp: okResponse()}
		server := newTestServer(fake)

		params := map[string]string{"setName": "mike", "recordId": "17"}
		rr := httptest.NewRecorder()
		err := server.handleUpdateRecord(rr, jsonRequest(`{"decision":true,"fund":"palci"}`, params), "update record")
		if err != nil {
			t.Fatalf("handleUpdateRecord returned error: %v", err)
		}

		assertEqual(t, "command sent to CCMS", fake.lastCmd,
			"update mike set decision = true, fund = palci where id = 17;")
		assertStatus(t, rr, http.StatusNoContent)
	})

	// A qualified set name like "foo.bar" has the part after the period replaced
	// with "object" before the command is built.
	t.Run("complex case with faceted set-name", func(t *testing.T) {
		fake := &fakeCCMS{resp: okResponse()}
		server := newTestServer(fake)

		params := map[string]string{"setName": "foo.bar", "recordId": "42"}
		rr := httptest.NewRecorder()
		err := server.handleUpdateRecord(rr, jsonRequest(`{"decision":true,"fund":"palci"}`, params), "update record")
		if err != nil {
			t.Fatalf("handleUpdateRecord returned error: %v", err)
		}

		assertEqual(t, "command sent to CCMS", fake.lastCmd,
			"update foo.object set decision = true, fund = palci where id = 42;")
		assertStatus(t, rr, http.StatusNoContent)
	})
}

func TestHandleBatchUpdate(t *testing.T) {
	t.Run("decision only", func(t *testing.T) {
		fake := &fakeCCMS{resp: okResponse()}
		server := newTestServer(fake)

		params := map[string]string{"setName": "mike"}
		rr := httptest.NewRecorder()
		err := server.handleBatchUpdate(rr, jsonRequest(`{"ids":["7","8","9"],"changes":{"decision":true}}`, params), "batch update")
		if err != nil {
			t.Fatalf("handleBatchUpdate returned error: %v", err)
		}

		assertEqual(t, "command sent to CCMS", fake.lastCmd,
			"update mike set decision = true where id IN (7, 8, 9);")
		assertStatus(t, rr, http.StatusNoContent)
	})

	// All the changed fields and all the ids go into a single update statement.
	t.Run("decision and fund", func(t *testing.T) {
		fake := &fakeCCMS{resp: okResponse()}
		server := newTestServer(fake)

		params := map[string]string{"setName": "mike"}
		rr := httptest.NewRecorder()
		err := server.handleBatchUpdate(rr, jsonRequest(`{"ids":["7","8"],"changes":{"decision":false,"fund":"palci"}}`, params), "batch update")
		if err != nil {
			t.Fatalf("handleBatchUpdate returned error: %v", err)
		}

		assertEqual(t, "command sent to CCMS", fake.lastCmd,
			"update mike set decision = false, fund = palci where id IN (7, 8);")
		assertStatus(t, rr, http.StatusNoContent)
	})

	t.Run("empty ids list is rejected", func(t *testing.T) {
		fake := &fakeCCMS{resp: okResponse()}
		server := newTestServer(fake)

		rr := httptest.NewRecorder()
		err := server.handleBatchUpdate(rr, jsonRequest(`{"ids":[],"changes":{"decision":true}}`, map[string]string{"setName": "mike"}), "batch update")
		if err == nil {
			t.Fatal("expected an error for an empty ids list, got nil")
		}
		assertErrContains(t, err, "no ids specified")

		// The handler must bail before sending anything to CCMS.
		assertEqual(t, "command sent to CCMS", fake.lastCmd, "")
	})

	t.Run("no changes is rejected", func(t *testing.T) {
		fake := &fakeCCMS{resp: okResponse()}
		server := newTestServer(fake)

		rr := httptest.NewRecorder()
		err := server.handleBatchUpdate(rr, jsonRequest(`{"ids":["7"],"changes":{}}`, map[string]string{"setName": "mike"}), "batch update")
		if err == nil {
			t.Fatal("expected an error when no changes are specified, got nil")
		}
		assertErrContains(t, err, "no changes specified")

		assertEqual(t, "command sent to CCMS", fake.lastCmd, "")
	})
}

func TestHandleCreateFilterNameOnly(t *testing.T) {
	fake := &fakeCCMS{resp: okResponse()}
	server := newTestServer(fake)

	rr := httptest.NewRecorder()
	err := server.handleCreateFilter(rr, jsonRequest(`{"name":"active"}`, nil), "create filter")
	if err != nil {
		t.Fatalf("handleCreateFilter returned error: %v", err)
	}

	assertEqual(t, "command sent to CCMS", fake.lastCmd, "create filter active;")
	assertStatus(t, rr, http.StatusNoContent)
}

// A filter's condition may be supplied structurally instead. Because the body
// is already JSON, 'jsonCond' is a nested object rather than a string of JSON.
func TestHandleCreateFilterJSONCond(t *testing.T) {
	cases := map[string]string{
		`{"type":"term","field":"age","rel":"gt","value":18}`: `create filter active where age > 18;`,
		`{"type":"and","clauses":[{"type":"term","field":"age","rel":"ge","value":143100000},` +
			`{"type":"term","field":"age","rel":"le","value":201400000}]}`: `create filter active where (age >= 143100000 and age <= 201400000);`,
		`{"type":"term","field":"title","rel":"contains","value":"'; drop filter x; --"}`: `create filter active where title ilike '%''; drop filter x; --%';`,
	}
	for jsonCond, want := range cases {
		t.Run(want, func(t *testing.T) {
			fake := &fakeCCMS{resp: okResponse()}
			server := newTestServer(fake)

			body := `{"name":"active","jsonCond":` + jsonCond + `}`
			rr := httptest.NewRecorder()
			err := server.handleCreateFilter(rr, jsonRequest(body, nil), "create filter")
			if err != nil {
				t.Fatalf("handleCreateFilter returned error: %v", err)
			}
			assertEqual(t, "command sent to CCMS", fake.lastCmd, want)
			assertStatus(t, rr, http.StatusNoContent)
		})
	}
}

// As for retrieval, a withdrawn 'cond' is refused, and a bad structure is the
// client's mistake rather than a server fault.
func TestHandleCreateFilterJSONCondErrors(t *testing.T) {
	cases := map[string]string{
		`{"name":"active","cond":"age>18"}`:                                                               `'cond' parameter has been withdrawn`,
		`{"name":"active","jsonCond":{"type":"xyzzy"}}`:                                                   `unknown clause type "xyzzy"`,
		`{"name":"active","jsonCond":{"type":"term","field":"age; drop filter x","rel":"gt","value":18}}`: `invalid field identifier`,
	}
	for body, wantErr := range cases {
		t.Run(wantErr, func(t *testing.T) {
			fake := &fakeCCMS{resp: okResponse()}
			server := newTestServer(fake)

			err := server.handleCreateFilter(httptest.NewRecorder(), jsonRequest(body, nil), "create filter")
			if err == nil {
				t.Fatalf("expected an error for body %s", body)
			}
			assertHTTPStatus(t, err, http.StatusBadRequest)
			assertErrContains(t, err, wantErr)
			if fake.lastCmd != "" {
				t.Errorf("a command was sent despite the bad request: %q", fake.lastCmd)
			}
		})
	}
}

// The template names another filter, so it must be an identifier and cannot be
// used to append anything else to the command.
func TestHandleCreateFilterTemplate(t *testing.T) {
	t.Run("qualified name survives intact", func(t *testing.T) {
		fake := &fakeCCMS{resp: okResponse()}
		server := newTestServer(fake)

		body := `{"name":"korea_lit.jurassic","jsonCond":{"type":"term","field":"age","rel":"gt","value":18},` +
			`"template":"korea_lit.mesozoic"}`
		err := server.handleCreateFilter(httptest.NewRecorder(), jsonRequest(body, nil), "create filter")
		if err != nil {
			t.Fatalf("handleCreateFilter returned error: %v", err)
		}
		assertEqual(t, "command sent to CCMS", fake.lastCmd,
			"create filter korea_lit.jurassic where age > 18 template korea_lit.mesozoic;")
	})

	for _, template := range []string{
		"mesozoic; drop filter x",
		"mesozoic where 1=1",
		"mesozoic'",
		"1mesozoic",
		"meso zoic",
	} {
		t.Run(template, func(t *testing.T) {
			fake := &fakeCCMS{resp: okResponse()}
			server := newTestServer(fake)

			body, mErr := json.Marshal(CreateFilter{
				Name:     "active",
				JSONCond: json.RawMessage(`{"type":"term","field":"age","rel":"gt","value":18}`),
				Template: template,
			})
			if mErr != nil {
				t.Fatal(mErr)
			}
			err := server.handleCreateFilter(httptest.NewRecorder(), jsonRequest(string(body), nil), "create filter")
			if err == nil {
				t.Fatalf("template %q should have been rejected", template)
			}
			assertHTTPStatus(t, err, http.StatusBadRequest)
			assertErrContains(t, err, fmt.Sprintf("invalid filter template identifier: %q", template))
			if fake.lastCmd != "" {
				t.Errorf("a command was sent despite the bad request: %q", fake.lastCmd)
			}
		})
	}
}

// An explicit null is not a condition, and must not be mistaken for one.
func TestHandleCreateFilterNullJSONCond(t *testing.T) {
	fake := &fakeCCMS{resp: okResponse()}
	server := newTestServer(fake)

	rr := httptest.NewRecorder()
	body := `{"name":"active","jsonCond":null}`
	err := server.handleCreateFilter(rr, jsonRequest(body, nil), "create filter")
	if err != nil {
		t.Fatalf("handleCreateFilter returned error: %v", err)
	}
	assertEqual(t, "command sent to CCMS", fake.lastCmd, "create filter active;")
}

func TestHandleDeleteFilter(t *testing.T) {
	// The identifier is the project-qualified name the filter was created
	// under, so the '.' must survive validation and reach the command intact.
	fake := &fakeCCMS{resp: okResponse()}
	server := newTestServer(fake)

	rr := httptest.NewRecorder()
	err := server.handleDeleteFilter(rr, jsonRequest("", map[string]string{"filterId": "korea_lit.jurassic"}), "delete filter")
	if err != nil {
		t.Fatalf("handleDeleteFilter returned error: %v", err)
	}

	assertEqual(t, "command sent to CCMS", fake.lastCmd, "drop filter korea_lit.jurassic;")
	assertStatus(t, rr, http.StatusNoContent)
}

func TestHandleDeleteFilterBadIdentifier(t *testing.T) {
	fake := &fakeCCMS{resp: okResponse()}
	server := newTestServer(fake)

	rr := httptest.NewRecorder()
	err := server.handleDeleteFilter(rr, jsonRequest("", map[string]string{"filterId": "korea_lit.jurassic; drop set users"}), "delete filter")
	if err == nil {
		t.Fatal("expected an error for an identifier carrying a second statement, got nil")
	}
	assertErrContains(t, err, "invalid filter identifier")
	assertEqual(t, "command sent to CCMS", fake.lastCmd, "")
}

func TestHandleCreateSet(t *testing.T) {
	fake := &fakeCCMS{resp: okResponse()}
	server := newTestServer(fake)

	rr := httptest.NewRecorder()
	err := server.handleCreateSet(rr, jsonRequest(`{"name":"users"}`, nil), "create set")
	if err != nil {
		t.Fatalf("handleCreateSet returned error: %v", err)
	}

	assertEqual(t, "command sent to CCMS", fake.lastCmd, "create set users;")
	assertStatus(t, rr, http.StatusNoContent)
}

// A supplied title is applied by a second command, since "create set"
// itself takes no title.
func TestHandleCreateSetWithTitle(t *testing.T) {
	fake := &fakeCCMS{resp: okResponse()}
	server := newTestServer(fake)

	body := `{"name":"users","title":"Registered users"}`
	rr := httptest.NewRecorder()
	err := server.handleCreateSet(rr, jsonRequest(body, nil), "create set")
	if err != nil {
		t.Fatalf("handleCreateSet returned error: %v", err)
	}

	assertEqual(t, "command sent to CCMS", fake.lastCmd, "create set users;\nalter set users alter property title set 'Registered users';")
	assertStatus(t, rr, http.StatusNoContent)
}

func TestHandleAlterSet(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		fake := &fakeCCMS{resp: okResponse()}
		server := newTestServer(fake)

		body := `{"name":"users","title":"New title"}`
		rr := httptest.NewRecorder()
		err := server.handleAlterSet(rr, jsonRequest(body, map[string]string{"setName": "users"}), "alter set")
		if err != nil {
			t.Fatalf("handleAlterSet returned error: %v", err)
		}

		// The URL's set name drives the command; the body's "name" is ignored.
		assertEqual(t, "command sent to CCMS", fake.lastCmd,
			"alter set users alter property title set 'New title';")
		assertStatus(t, rr, http.StatusNoContent)
	})

	// A title containing a single quote must be escaped by doubling it.
	t.Run("title with apostrophe is escaped", func(t *testing.T) {
		fake := &fakeCCMS{resp: okResponse()}
		server := newTestServer(fake)

		body := `{"title":"Mike's set"}`
		rr := httptest.NewRecorder()
		err := server.handleAlterSet(rr, jsonRequest(body, map[string]string{"setName": "mike"}), "alter set")
		if err != nil {
			t.Fatalf("handleAlterSet returned error: %v", err)
		}

		assertEqual(t, "command sent to CCMS", fake.lastCmd,
			"alter set mike alter property title set 'Mike''s set';")
		assertStatus(t, rr, http.StatusNoContent)
	})

	t.Run("invalid set name", func(t *testing.T) {
		fake := &fakeCCMS{resp: okResponse()}
		server := newTestServer(fake)

		body := `{"title":"New title"}`
		rr := httptest.NewRecorder()
		err := server.handleAlterSet(rr, jsonRequest(body, map[string]string{"setName": "users; drop set mike"}), "alter set")
		if err == nil {
			t.Fatal("expected an error for an invalid set name, got nil")
		}
		assertErrContains(t, err, "invalid set identifier")
		assertEqual(t, "command sent to CCMS", fake.lastCmd, "")
	})

	t.Run("malformed JSON body", func(t *testing.T) {
		fake := &fakeCCMS{resp: okResponse()}
		server := newTestServer(fake)

		rr := httptest.NewRecorder()
		err := server.handleAlterSet(rr, jsonRequest(`{"title":`, map[string]string{"setName": "users"}), "alter set")
		if err == nil {
			t.Fatal("expected an error for malformed JSON, got nil")
		}
		assertErrContains(t, err, "deserialize JSON")
	})
}

func TestHandleDropSet(t *testing.T) {
	fake := &fakeCCMS{resp: okResponse()}
	server := newTestServer(fake)

	rr := httptest.NewRecorder()
	err := server.handleDropSet(rr, jsonRequest("", map[string]string{"setName": "users"}), "drop set")
	if err != nil {
		t.Fatalf("handleDropSet returned error: %v", err)
	}

	assertEqual(t, "command sent to CCMS", fake.lastCmd, "drop set users;")
	assertStatus(t, rr, http.StatusNoContent)
}

func TestHandleCreateFund(t *testing.T) {
	fake := &fakeCCMS{resp: okResponse()}
	server := newTestServer(fake)

	rr := httptest.NewRecorder()
	err := server.handleCreateFund(rr, jsonRequest(`{"id":"endowment"}`, nil), "create fund")
	if err != nil {
		t.Fatalf("handleCreateFund returned error: %v", err)
	}

	assertEqual(t, "command sent to CCMS", fake.lastCmd, "create fund endowment;")
	assertStatus(t, rr, http.StatusNoContent)
}

func TestHandleUpdateFund(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		fake := &fakeCCMS{resp: okResponse()}
		server := newTestServer(fake)

		body := `{"id":"endowment","name":"Endowment Fund"}`
		rr := httptest.NewRecorder()
		err := server.handleUpdateFund(rr, jsonRequest(body, map[string]string{"fundId": "endowment"}), "update fund")
		if err != nil {
			t.Fatalf("handleUpdateFund returned error: %v", err)
		}

		// The URL's fund id drives the command; the body's "id" is ignored.
		assertEqual(t, "command sent to CCMS", fake.lastCmd,
			"alter fund endowment alter property title set 'Endowment Fund';")
		assertStatus(t, rr, http.StatusNoContent)
	})

	// A name containing a single quote must be escaped by doubling it.
	t.Run("name with apostrophe is escaped", func(t *testing.T) {
		fake := &fakeCCMS{resp: okResponse()}
		server := newTestServer(fake)

		body := `{"name":"Founder's Fund"}`
		rr := httptest.NewRecorder()
		err := server.handleUpdateFund(rr, jsonRequest(body, map[string]string{"fundId": "founders"}), "update fund")
		if err != nil {
			t.Fatalf("handleUpdateFund returned error: %v", err)
		}

		assertEqual(t, "command sent to CCMS", fake.lastCmd,
			"alter fund founders alter property title set 'Founder''s Fund';")
		assertStatus(t, rr, http.StatusNoContent)
	})

	t.Run("malformed JSON body", func(t *testing.T) {
		fake := &fakeCCMS{resp: okResponse()}
		server := newTestServer(fake)

		rr := httptest.NewRecorder()
		err := server.handleUpdateFund(rr, jsonRequest(`{"name":`, map[string]string{"fundId": "endowment"}), "update fund")
		if err == nil {
			t.Fatal("expected an error for malformed JSON, got nil")
		}
		assertErrContains(t, err, "deserialize JSON")

		// The handler must bail before sending anything to CCMS.
		assertEqual(t, "command sent to CCMS", fake.lastCmd, "")
	})
}

func TestHandleDeleteFund(t *testing.T) {
	fake := &fakeCCMS{resp: okResponse()}
	server := newTestServer(fake)

	rr := httptest.NewRecorder()
	err := server.handleDeleteFund(rr, jsonRequest("", map[string]string{"fundId": "endowment"}), "delete fund")
	if err != nil {
		t.Fatalf("handleDeleteFund returned error: %v", err)
	}

	assertEqual(t, "command sent to CCMS", fake.lastCmd, "drop fund endowment;")
	assertStatus(t, rr, http.StatusNoContent)
}

// The condition itself is covered by TestHandleObjectsJSONCond below; what is
// under test here is the limit, which the command carries only when the request
// asked for one.
func TestHandleAddObjects(t *testing.T) {
	cases := map[string]string{
		`,"limit":"5"`: "insert into dest select * from src where age > 18 limit 5;",
		``:             "insert into dest select * from src where age > 18;",
	}
	for limit, want := range cases {
		t.Run(want, func(t *testing.T) {
			fake := &fakeCCMS{resp: okResponse()}
			server := newTestServer(fake)

			body := `{"from":"src","jsonCond":{"type":"term","field":"age","rel":"gt","value":18}` + limit + `}`
			rr := httptest.NewRecorder()
			err := server.handleAddObjects(rr, jsonRequest(body, map[string]string{"setName": "dest"}), "add objects")
			if err != nil {
				t.Fatalf("handleAddObjects returned error: %v", err)
			}

			assertEqual(t, "command sent to CCMS", fake.lastCmd, want)
			assertStatus(t, rr, http.StatusNoContent)
		})
	}
}

// handlerUnderTest is one of the body-carrying handlers that accepts a
// condition, named so failures say which one.
type condHandler struct {
	name    string
	call    func(*ModCyclopsServer, *httptest.ResponseRecorder, *http.Request) error
	params  map[string]string
	prefix  string // the body members that precede the condition
	wantFmt string // the whole command, with %s where the condition goes
}

var condHandlers = []condHandler{
	{
		name: "add objects",
		call: func(s *ModCyclopsServer, rr *httptest.ResponseRecorder, req *http.Request) error {
			return s.handleAddObjects(rr, req, "add objects")
		},
		params:  map[string]string{"setName": "dest"},
		prefix:  `"from":"src",`,
		wantFmt: "insert into dest select * from src where %s;",
	},
	{
		name: "remove objects",
		call: func(s *ModCyclopsServer, rr *httptest.ResponseRecorder, req *http.Request) error {
			return s.handleRemoveObjects(rr, req, "remove objects")
		},
		params: map[string]string{"setName": "dest"},
		// Note the double space: the command is "delete from <set> " + clause,
		// and the conditional clause itself begins with a leading space.
		wantFmt: "delete from dest  where %s;",
	},
}

// Both handlers take the condition as a structure, rendered through the same
// code the other entry points use.
func TestHandleObjectsJSONCond(t *testing.T) {
	cases := map[string]string{
		`{"type":"term","field":"age","rel":"gt","value":18}`:                             `age > 18`,
		`{"type":"term","field":"author","rel":"eq","value":"Adams, John"}`:               `author = 'Adams, John'`,
		`{"type":"term","field":"title","rel":"contains","value":"'; drop set dest; --"}`: `title ilike '%''; drop set dest; --%'`,
	}
	for _, h := range condHandlers {
		for jsonCond, wantCond := range cases {
			t.Run(h.name+"/"+wantCond, func(t *testing.T) {
				fake := &fakeCCMS{resp: okResponse()}
				server := newTestServer(fake)

				body := `{` + h.prefix + `"jsonCond":` + jsonCond + `}`
				rr := httptest.NewRecorder()
				if err := h.call(server, rr, jsonRequest(body, h.params)); err != nil {
					t.Fatalf("%s returned error: %v", h.name, err)
				}
				assertEqual(t, "command sent to CCMS", fake.lastCmd, fmt.Sprintf(h.wantFmt, wantCond))
				assertStatus(t, rr, http.StatusNoContent)
			})
		}
	}
}

// And both reject the same client mistakes, without sending a command.
func TestHandleObjectsJSONCondErrors(t *testing.T) {
	cases := map[string]string{
		`"cond":"age>18"`:             `'cond' parameter has been withdrawn`,
		`"jsonCond":{"type":"xyzzy"}`: `unknown clause type "xyzzy"`,
		`"jsonCond":{"type":"term","field":"age; drop set x","rel":"gt","value":18}`: `invalid field identifier`,
	}
	for _, h := range condHandlers {
		for members, wantErr := range cases {
			t.Run(h.name+"/"+wantErr, func(t *testing.T) {
				fake := &fakeCCMS{resp: okResponse()}
				server := newTestServer(fake)

				body := `{` + h.prefix + members + `}`
				err := h.call(server, httptest.NewRecorder(), jsonRequest(body, h.params))
				if err == nil {
					t.Fatalf("%s: expected an error for body %s", h.name, body)
				}
				assertHTTPStatus(t, err, http.StatusBadRequest)
				assertErrContains(t, err, wantErr)
				if fake.lastCmd != "" {
					t.Errorf("a command was sent despite the bad request: %q", fake.lastCmd)
				}
			})
		}
	}
}

func TestHandleDeleteProject(t *testing.T) {
	fake := &fakeCCMS{resp: okResponse()}
	server := newTestServer(fake)

	rr := httptest.NewRecorder()
	err := server.handleDeleteProject(rr, jsonRequest("", map[string]string{"projectId": "p1"}), "delete project")
	if err != nil {
		t.Fatalf("handleDeleteProject returned error: %v", err)
	}

	assertEqual(t, "command sent to CCMS", fake.lastCmd, "drop project p1;")
	assertStatus(t, rr, http.StatusNoContent)
}

func TestHandleCreateProject(t *testing.T) {
	fake := &fakeCCMS{resp: okResponse()}
	server := newTestServer(fake)

	body := `{"id":"p1","name":"T","action":{"id":"approve"},"mou_link":"m",` +
		`"funds":[{"id":"f1"},{"id":"f2"}],"origins":[{"id":"o1"},{"id":"o2"}],` +
		`"destinations":[{"id":"d1"}]}`
	rr := httptest.NewRecorder()
	err := server.handleCreateProject(rr, jsonRequest(body, nil), "create project")
	if err != nil {
		t.Fatalf("handleCreateProject returned error: %v", err)
	}

	want := "create project p1;\n" +
		"alter project p1 alter property title set 'T';\n" +
		"alter project p1 alter property action set approve;\n" +
		"alter project p1 alter property mou_link set 'm';\n" +
		"alter project p1 alter property funds add f1;\n" +
		"alter project p1 alter property funds add f2;\n" +
		"alter project p1 alter property origins drop all;\n" +
		"alter project p1 alter property origins add o1;\n" +
		"alter project p1 alter property origins add o2;\n" +
		"alter project p1 alter property destinations drop all;\n" +
		"alter project p1 alter property destinations add d1;\n"
	assertEqual(t, "command sent to CCMS", fake.lastCmd, want)
	assertStatus(t, rr, http.StatusNoContent)
}

func TestHandleCreateProjectNoId(t *testing.T) {
	fake := &fakeCCMS{resp: okResponse()}
	server := newTestServer(fake)

	rr := httptest.NewRecorder()
	err := server.handleCreateProject(rr, jsonRequest(`{"name":"T"}`, nil), "create project")
	if err == nil {
		t.Fatal("expected an error when id is missing, got nil")
	}
	assertErrContains(t, err, "no id specified")

	// The handler must bail before sending anything to CCMS.
	assertEqual(t, "command sent to CCMS", fake.lastCmd, "")
}

func TestHandleUpdateProject(t *testing.T) {
	fake := &fakeCCMS{resp: okResponse()}
	server := newTestServer(fake)

	body := `{"name":"T","action":{"id":"a"}}`
	rr := httptest.NewRecorder()
	err := server.handleUpdateProject(rr, jsonRequest(body, map[string]string{"projectId": "p1"}), "update project")
	if err != nil {
		t.Fatalf("handleUpdateProject returned error: %v", err)
	}

	// With no existing funds and no new funds, no fund commands are emitted.
	want := "alter project p1 alter property title set 'T';\n" +
		"alter project p1 alter property action set a;\n" +
		"alter project p1 alter property mou_link set '';\n" +
		"alter project p1 alter property origins drop all;\n" +
		"alter project p1 alter property destinations drop all;\n"
	assertEqual(t, "command sent to CCMS", fake.lastCmd, want)
	assertStatus(t, rr, http.StatusNoContent)
}

// TestHandleUpdateProjectFundDiff checks that updating a project's funds emits
// the minimal set of add/drop commands by comparing the existing funds against
// the new ones: only genuinely added or removed funds produce commands, and
// funds present in both lists are left untouched.
func TestHandleUpdateProjectFundDiff(t *testing.T) {
	tests := []struct {
		name      string
		existing  string   // CCMS-style "id:desc|id:desc" list of current funds
		newFunds  []string // fund ids in the update request
		fundLines []string // expected fund commands, in order
	}{
		{
			name:      "no funds at all",
			existing:  "",
			newFunds:  nil,
			fundLines: nil,
		},
		{
			name:      "unchanged funds emit nothing",
			existing:  "f1:Fund One|f2:Fund Two",
			newFunds:  []string{"f1", "f2"},
			fundLines: nil,
		},
		{
			name:      "add only",
			existing:  "",
			newFunds:  []string{"f1", "f2"},
			fundLines: []string{"funds add f1", "funds add f2"},
		},
		{
			name:      "drop only",
			existing:  "f1:Fund One|f2:Fund Two",
			newFunds:  nil,
			fundLines: []string{"funds drop f1", "funds drop f2"},
		},
		{
			name:      "add and drop, keeping overlap",
			existing:  "f1:Fund One|f2:Fund Two",
			newFunds:  []string{"f2", "f3"},
			fundLines: []string{"funds add f3", "funds drop f1"},
		},
		{
			name:      "complete replacement",
			existing:  "f1:Fund One",
			newFunds:  []string{"f2"},
			fundLines: []string{"funds add f2", "funds drop f1"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ccms.NewResult("ok")
			result.AddData([]any{"funds", tc.existing})
			resp := ccms.NewResponse()
			resp.AddResult(result)
			fake := &fakeCCMS{resp: resp}
			server := newTestServer(fake)

			funds := make([]string, len(tc.newFunds))
			for i, id := range tc.newFunds {
				funds[i] = fmt.Sprintf("{\"id\":%q}", id)
			}
			body := fmt.Sprintf(`{"name":"T","action":{"id":"a"},"funds":[%s]}`,
				strings.Join(funds, ","))

			rr := httptest.NewRecorder()
			err := server.handleUpdateProject(rr, jsonRequest(body, map[string]string{"projectId": "p1"}), "update project")
			if err != nil {
				t.Fatalf("handleUpdateProject returned error: %v", err)
			}

			want := "alter project p1 alter property title set 'T';\n" +
				"alter project p1 alter property action set a;\n" +
				"alter project p1 alter property mou_link set '';\n"
			for _, line := range tc.fundLines {
				want += "alter project p1 alter property " + line + ";\n"
			}
			want += "alter project p1 alter property origins drop all;\n" +
				"alter project p1 alter property destinations drop all;\n"

			assertEqual(t, "command sent to CCMS", fake.lastCmd, want)
			assertStatus(t, rr, http.StatusNoContent)
		})
	}
}

// -----------------------------------------------------------------------------
// Handlers with their own response shapes.

func TestHandleFetchProject(t *testing.T) {
	result := ccms.NewResult("ok")
	result.AddData([]any{"title", "My Title"})
	result.AddData([]any{"action", "approve:Approve"})
	result.AddData([]any{"funds", "f1:Fund One|f2:Fund Two"})
	result.AddData([]any{"origins", "seoul:Seoul"})
	result.AddData([]any{"destinations", ""})
	result.AddData([]any{"bogus", "ignored"}) // exercises the default branch
	resp := ccms.NewResponse()
	resp.AddResult(result)

	fake := &fakeCCMS{resp: resp}
	server := newTestServer(fake)

	rr := httptest.NewRecorder()
	err := server.handleFetchProject(rr, jsonRequest("", map[string]string{"projectId": "p1"}), "fetch project")
	if err != nil {
		t.Fatalf("handleFetchProject returned error: %v", err)
	}

	assertEqual(t, "command sent to CCMS", fake.lastCmd, "show project p1;")

	var got Project
	err = json.Unmarshal(rr.Body.Bytes(), &got)
	if err != nil {
		t.Fatalf("could not decode response body %q: %v", rr.Body.String(), err)
	}
	want := Project{
		Id:           "p1",
		Name:         "My Title",
		Action:       ProjectAction{Id: "approve", Name: "Approve"},
		Funds:        []ProjectFund{{Id: "f1", Name: "Fund One"}, {Id: "f2", Name: "Fund Two"}},
		Origins:      []ProjectLocation{{Id: "seoul", Name: "Seoul"}},
		Destinations: []ProjectLocation{},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("translated response:\n got %+v\nwant %+v", got, want)
	}
}

func TestHandleFetchFund(t *testing.T) {
	result := ccms.NewResult("ok")
	result.AddData([]any{"title", "Endowment Fund"})
	result.AddData([]any{"bogus", "ignored"}) // exercises the default branch
	resp := ccms.NewResponse()
	resp.AddResult(result)

	fake := &fakeCCMS{resp: resp}
	server := newTestServer(fake)

	rr := httptest.NewRecorder()
	err := server.handleFetchFund(rr, jsonRequest("", map[string]string{"fundId": "endowment"}), "fetch fund")
	if err != nil {
		t.Fatalf("handleFetchFund returned error: %v", err)
	}

	assertEqual(t, "command sent to CCMS", fake.lastCmd, "show fund endowment;")

	var got Fund
	err = json.Unmarshal(rr.Body.Bytes(), &got)
	if err != nil {
		t.Fatalf("could not decode response body %q: %v", rr.Body.String(), err)
	}
	// The id comes from the URL param, the name from the CCMS response.
	want := Fund{Id: "endowment", Name: "Endowment Fund"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("translated response:\n got %+v\nwant %+v", got, want)
	}
}

// -----------------------------------------------------------------------------
// The tag add/remove endpoint is a no-op: it returns 204 and talks to nobody.

func TestHandleAddRemoveTags(t *testing.T) {
	fake := &fakeCCMS{resp: okResponse()}
	server := newTestServer(fake)

	rr := httptest.NewRecorder()
	err := server.handleAddRemoveTags(rr, jsonRequest(`{"whatever":true}`, nil), "add/remove tags")
	if err != nil {
		t.Fatalf("handleAddRemoveTags returned error: %v", err)
	}

	assertStatus(t, rr, http.StatusNoContent)
	assertEqual(t, "command sent to CCMS", fake.lastCmd, "")
}

// -----------------------------------------------------------------------------

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
			name: "with condition and filter",
			url: "/test?fields=id&filter=active&jsonCond=" +
				url.QueryEscape(`{"type":"term","field":"age","rel":"gt","value":18}`),
			setName:     "users",
			expected:    "select id from users where age > 18 filter active limit 100;",
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
