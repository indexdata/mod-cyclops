package cyclops

import "context"
import "encoding/json"
import "errors"
import "fmt"
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
// value, each a single-column string. This matches what the show* handlers read.
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

func TestHandleShowFilters(t *testing.T) {
	fake := &fakeCCMS{resp: listResponse("active", "archived")}
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
	want := FilterList{Filters: []any{"active", "archived"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("translated response:\n got %+v\nwant %+v", got, want)
	}
}

func TestHandleShowSets(t *testing.T) {
	fake := &fakeCCMS{resp: listResponse("users", "books")}
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
	want := SetList{Sets: []any{"users", "books"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("translated response:\n got %+v\nwant %+v", got, want)
	}
}

func TestHandleShowSetsInProject(t *testing.T) {
	fake := &fakeCCMS{resp: listResponse("mike.object", "mike.endangered")}
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
	want := SetList{Sets: []any{"mike.object", "mike.endangered"}}
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
		{Name: "general", Title: "General Fund"},
		{Name: "endowment", Title: "Endowment Fund"},
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
		{AltName: "alpha", Title: "Project Alpha"},
		{AltName: "beta", Title: "Project Beta"},
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

		params := map[string]string{"setName": "mike", "recordId": "rec1"}
		rr := httptest.NewRecorder()
		err := server.handleUpdateRecord(rr, jsonRequest(`{"decision":true,"fund":"palci"}`, params), "update record")
		if err != nil {
			t.Fatalf("handleUpdateRecord returned error: %v", err)
		}

		assertEqual(t, "command sent to CCMS", fake.lastCmd,
			"update mike set decision = true where id = rec1; update mike set fund = palci where id = rec1;")
		assertStatus(t, rr, http.StatusNoContent)
	})

	// A qualified set name like "foo.bar" has the part after the period replaced
	// with "object" before the command is built.
	t.Run("complex case with faceted set-name", func(t *testing.T) {
		fake := &fakeCCMS{resp: okResponse()}
		server := newTestServer(fake)

		params := map[string]string{"setName": "foo.bar", "recordId": "rec1"}
		rr := httptest.NewRecorder()
		err := server.handleUpdateRecord(rr, jsonRequest(`{"decision":true,"fund":"palci"}`, params), "update record")
		if err != nil {
			t.Fatalf("handleUpdateRecord returned error: %v", err)
		}

		assertEqual(t, "command sent to CCMS", fake.lastCmd,
			"update foo.object set decision = true where id = rec1; update foo.object set fund = palci where id = rec1;")
		assertStatus(t, rr, http.StatusNoContent)
	})
}

func TestHandleCreateFilter(t *testing.T) {
	fake := &fakeCCMS{resp: okResponse()}
	server := newTestServer(fake)

	body := `{"name":"active","cond":"age>18","template":"tmpl"}`
	rr := httptest.NewRecorder()
	err := server.handleCreateFilter(rr, jsonRequest(body, nil), "create filter")
	if err != nil {
		t.Fatalf("handleCreateFilter returned error: %v", err)
	}

	assertEqual(t, "command sent to CCMS", fake.lastCmd, "create filter active where age>18 template tmpl;")
	assertStatus(t, rr, http.StatusNoContent)
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
	err := server.handleCreateFund(rr, jsonRequest(`{"name":"endowment"}`, nil), "create fund")
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

		body := `{"name":"endowment","title":"Endowment Fund"}`
		rr := httptest.NewRecorder()
		err := server.handleUpdateFund(rr, jsonRequest(body, map[string]string{"fundName": "endowment"}), "update fund")
		if err != nil {
			t.Fatalf("handleUpdateFund returned error: %v", err)
		}

		// The URL's fund name drives the command; the body's "name" is ignored.
		assertEqual(t, "command sent to CCMS", fake.lastCmd,
			"alter fund endowment alter property title set 'Endowment Fund';")
		assertStatus(t, rr, http.StatusNoContent)
	})

	// A title containing a single quote must be escaped by doubling it.
	t.Run("title with apostrophe is escaped", func(t *testing.T) {
		fake := &fakeCCMS{resp: okResponse()}
		server := newTestServer(fake)

		body := `{"title":"Founder's Fund"}`
		rr := httptest.NewRecorder()
		err := server.handleUpdateFund(rr, jsonRequest(body, map[string]string{"fundName": "founders"}), "update fund")
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
		err := server.handleUpdateFund(rr, jsonRequest(`{"title":`, map[string]string{"fundName": "endowment"}), "update fund")
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
	err := server.handleDeleteFund(rr, jsonRequest("", map[string]string{"fundName": "endowment"}), "delete fund")
	if err != nil {
		t.Fatalf("handleDeleteFund returned error: %v", err)
	}

	assertEqual(t, "command sent to CCMS", fake.lastCmd, "drop fund endowment;")
	assertStatus(t, rr, http.StatusNoContent)
}

func TestHandleAddObjects(t *testing.T) {
	fake := &fakeCCMS{resp: okResponse()}
	server := newTestServer(fake)

	body := `{"from":"src","cond":"age>18","limit":"5"}`
	rr := httptest.NewRecorder()
	err := server.handleAddObjects(rr, jsonRequest(body, map[string]string{"setName": "dest"}), "add objects")
	if err != nil {
		t.Fatalf("handleAddObjects returned error: %v", err)
	}

	assertEqual(t, "command sent to CCMS", fake.lastCmd, "insert into dest select * from src where age>18 limit 5;")
	assertStatus(t, rr, http.StatusNoContent)
}

func TestHandleRemoveObjects(t *testing.T) {
	fake := &fakeCCMS{resp: okResponse()}
	server := newTestServer(fake)

	rr := httptest.NewRecorder()
	err := server.handleRemoveObjects(rr, jsonRequest(`{"cond":"age<10"}`, map[string]string{"setName": "dest"}), "remove objects")
	if err != nil {
		t.Fatalf("handleRemoveObjects returned error: %v", err)
	}

	// Note the double space: command is "delete from <set> " + clause, and the
	// conditional clause itself begins with a leading space (" where ...").
	assertEqual(t, "command sent to CCMS", fake.lastCmd, "delete from dest  where age<10;")
	assertStatus(t, rr, http.StatusNoContent)
}

func TestHandleDeleteProject(t *testing.T) {
	fake := &fakeCCMS{resp: okResponse()}
	server := newTestServer(fake)

	rr := httptest.NewRecorder()
	err := server.handleDeleteProject(rr, jsonRequest("", map[string]string{"projectId": "p1"}), "delete project")
	if err != nil {
		t.Fatalf("handleDeleteProject returned error: %v", err)
	}

	assertEqual(t, "command sent to CCMS", fake.lastCmd, "archive project p1;")
	assertStatus(t, rr, http.StatusNoContent)
}

func TestHandleCreateProject(t *testing.T) {
	fake := &fakeCCMS{resp: okResponse()}
	server := newTestServer(fake)

	body := `{"altName":"p1","title":"T","action":{"id":"approve"},"mou_link":"m",` +
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

func TestHandleCreateProjectNoAltName(t *testing.T) {
	fake := &fakeCCMS{resp: okResponse()}
	server := newTestServer(fake)

	rr := httptest.NewRecorder()
	err := server.handleCreateProject(rr, jsonRequest(`{"title":"T"}`, nil), "create project")
	if err == nil {
		t.Fatal("expected an error when altName is missing, got nil")
	}
	assertErrContains(t, err, "no altName specified")

	// The handler must bail before sending anything to CCMS.
	assertEqual(t, "command sent to CCMS", fake.lastCmd, "")
}

func TestHandleUpdateProject(t *testing.T) {
	fake := &fakeCCMS{resp: okResponse()}
	server := newTestServer(fake)

	body := `{"title":"T","action":{"id":"a"}}`
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
			body := fmt.Sprintf(`{"title":"T","action":{"id":"a"},"funds":[%s]}`,
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
		AltName:      "p1",
		Title:        "My Title",
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
	err := server.handleFetchFund(rr, jsonRequest("", map[string]string{"fundName": "endowment"}), "fetch fund")
	if err != nil {
		t.Fatalf("handleFetchFund returned error: %v", err)
	}

	assertEqual(t, "command sent to CCMS", fake.lastCmd, "show fund endowment;")

	var got Fund
	err = json.Unmarshal(rr.Body.Bytes(), &got)
	if err != nil {
		t.Fatalf("could not decode response body %q: %v", rr.Body.String(), err)
	}
	// The name comes from the URL param, the title from the CCMS response.
	want := Fund{Name: "endowment", Title: "Endowment Fund"}
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
