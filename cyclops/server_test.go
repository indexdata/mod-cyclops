package cyclops

import "encoding/json"
import "net/http"
import "net/http/httptest"
import "reflect"
import "testing"
import "github.com/indexdata/ccms"

// serve drives a request through the router the constructor installed on the
// server's http.Server, returning the recorder. This exercises MakeModCyclops-
// Server's routing table rather than calling handlers directly.
func serve(server *ModCyclopsServer, req *http.Request) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(rr, req)
	return rr
}

func TestMakeModCyclopsServerWiring(t *testing.T) {
	fake := &fakeCCMS{}
	server := MakeModCyclopsServer(nil, fake, ".", 60)

	if server == nil {
		t.Fatal("MakeModCyclopsServer returned nil")
	}
	if server.ccmsClient != fake {
		t.Error("ccmsClient was not stored on the server")
	}

	// The timeout argument is converted from seconds to a Duration on both
	// read and write deadlines.
	if server.httpServer.ReadTimeout != 60*1e9 {
		t.Errorf("ReadTimeout: got %v want 60s", server.httpServer.ReadTimeout)
	}
	if server.httpServer.WriteTimeout != 60*1e9 {
		t.Errorf("WriteTimeout: got %v want 60s", server.httpServer.WriteTimeout)
	}
	if server.httpServer.Handler == nil {
		t.Error("constructor did not install an HTTP handler")
	}
}

// The static routes are self-contained: they touch neither CCMS nor the
// logger, so we can assert their responses directly.
func TestMakeModCyclopsServerStaticRoutes(t *testing.T) {
	server := newTestServer(&fakeCCMS{})

	t.Run("health", func(t *testing.T) {
		rr := serve(server, httptest.NewRequest(http.MethodGet, "/admin/health", nil))
		assertStatus(t, rr, http.StatusOK)
		assertEqual(t, "health body", rr.Body.String(), "Behold! I live!!\n")
	})

	t.Run("root", func(t *testing.T) {
		rr := serve(server, httptest.NewRequest(http.MethodGet, "/", nil))
		assertStatus(t, rr, http.StatusOK)
		assertEqual(t, "root content type", rr.Header().Get("Content-Type"), "text/html; charset=utf-8")
	})

	t.Run("unknown path yields 404", func(t *testing.T) {
		rr := serve(server, httptest.NewRequest(http.MethodGet, "/no/such/thing", nil))
		assertStatus(t, rr, http.StatusNotFound)
		assertEqual(t, "not-found body", rr.Body.String(), "Not Found\n")
	})
}

// A CCMS-backed route, driven end to end through the router, confirms the
// constructor connects the path to the right handler and that the handler in
// turn talks to the wired-in CCMS client.
func TestMakeModCyclopsServerRoutesToHandler(t *testing.T) {
	fake := &fakeCCMS{resp: listResponse("vip", "staff")}
	server := newTestServer(fake)

	rr := serve(server, httptest.NewRequest(http.MethodGet, "/cyclops/tags", nil))

	assertStatus(t, rr, http.StatusOK)
	assertEqual(t, "command sent to CCMS", fake.lastCmd, "show tags;")

	var got TagList
	err := json.Unmarshal(rr.Body.Bytes(), &got)
	if err != nil {
		t.Fatalf("could not decode response body %q: %v", rr.Body.String(), err)
	}
	want := TagList{Tags: []any{"vip", "staff"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("translated response:\n got %+v\nwant %+v", got, want)
	}
}

// A registered path that is hit with the wrong method should not fall through
// to the generic NotFound handler; chi answers 405 itself.
func TestMakeModCyclopsServerMethodNotAllowed(t *testing.T) {
	server := newTestServer(&fakeCCMS{resp: ccms.NewResponse()})

	rr := serve(server, httptest.NewRequest(http.MethodPatch, "/cyclops/tags", nil))
	assertStatus(t, rr, http.StatusMethodNotAllowed)
}
