package cyclops

import "encoding/json"
import "net/http"
import "net/http/httptest"
import "os"
import "path/filepath"
import "reflect"
import "strings"
import "testing"
import "github.com/MikeTaylor/catlogger"
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

// The POST /cyclops/sets/{setName}/{recordId} route is a wildcard sibling of
// the static /add, /remove and /tag sub-routes. Confirm the constructor wires
// it to handleUpdateRecord, and that the static routes still take precedence.
func TestMakeModCyclopsServerUpdateRecordRoute(t *testing.T) {
	t.Run("dispatches to the update-record handler", func(t *testing.T) {
		fake := &fakeCCMS{resp: okResponse()}
		server := newTestServer(fake)

		body := strings.NewReader(`{"decision":true,"fund":"palci"}`)
		rr := serve(server, httptest.NewRequest(http.MethodPost, "/cyclops/sets/mike/17", body))

		assertStatus(t, rr, http.StatusNoContent)
		assertEqual(t, "command sent to CCMS", fake.lastCmd,
			"update mike set decision = true where id = 17; update mike set fund = palci where id = 17;")
	})

	t.Run("static /add still wins over {recordId}", func(t *testing.T) {
		fake := &fakeCCMS{resp: okResponse()}
		server := newTestServer(fake)

		body := strings.NewReader(`{"from":"src"}`)
		rr := serve(server, httptest.NewRequest(http.MethodPost, "/cyclops/sets/mike/add", body))

		assertStatus(t, rr, http.StatusNoContent)
		assertEqual(t, "command sent to CCMS", fake.lastCmd,
			"insert into mike select * from src;")
	})
}

// serverRootedAt builds a server whose static files are served from the given
// directory. The htdocs FileServer reads from <root>/htdocs.
func serverRootedAt(root string) *ModCyclopsServer {
	logger := catlogger.MakeLogger("", "", false)
	return MakeModCyclopsServer(logger, &fakeCCMS{}, root, 60)
}

// writeFixture creates <root>/htdocs/<name> with the given contents, making the
// htdocs directory as needed, and fails the test if anything goes wrong.
func writeFixture(t *testing.T, root, name, contents string) {
	t.Helper()
	path := filepath.Join(root, "htdocs", name)
	err := os.MkdirAll(filepath.Dir(path), 0o755)
	if err != nil {
		t.Fatalf("could not create fixture dir: %v", err)
	}
	err = os.WriteFile(path, []byte(contents), 0o644)
	if err != nil {
		t.Fatalf("could not write fixture %q: %v", path, err)
	}
}

// The /htdocs/* route strips the prefix and serves the named file from the
// htdocs directory under root.
func TestMakeModCyclopsServerHtdocs(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "hello.txt", "hello, world\n")
	server := serverRootedAt(root)

	t.Run("serves an existing file", func(t *testing.T) {
		rr := serve(server, httptest.NewRequest(http.MethodGet, "/htdocs/hello.txt", nil))
		assertStatus(t, rr, http.StatusOK)
		assertEqual(t, "file body", rr.Body.String(), "hello, world\n")
	})

	t.Run("missing file yields 404", func(t *testing.T) {
		rr := serve(server, httptest.NewRequest(http.MethodGet, "/htdocs/nope.txt", nil))
		assertStatus(t, rr, http.StatusNotFound)
	})
}

// The /favicon.ico route is handled by the same FileServer but without a
// StripPrefix, so it serves htdocs/favicon.ico directly.
func TestMakeModCyclopsServerFavicon(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "favicon.ico", "icon-bytes")
	server := serverRootedAt(root)

	rr := serve(server, httptest.NewRequest(http.MethodGet, "/favicon.ico", nil))
	assertStatus(t, rr, http.StatusOK)
	assertEqual(t, "favicon body", rr.Body.String(), "icon-bytes")
}

// A registered path that is hit with the wrong method should not fall through
// to the generic NotFound handler; chi answers 405 itself.
func TestMakeModCyclopsServerMethodNotAllowed(t *testing.T) {
	server := newTestServer(&fakeCCMS{resp: ccms.NewResponse()})

	rr := serve(server, httptest.NewRequest(http.MethodPatch, "/cyclops/tags", nil))
	assertStatus(t, rr, http.StatusMethodNotAllowed)
}
