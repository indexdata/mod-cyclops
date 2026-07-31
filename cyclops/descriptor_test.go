package cyclops

import "encoding/json"
import "net/http"
import "os"
import "sort"
import "strings"
import "testing"
import "github.com/go-chi/chi/v5"

// The ModuleDescriptor tells Okapi which paths this module serves and which
// permissions guard them, but nothing at build time ties it to the routes the
// constructor actually installs. The two have drifted before: the descriptor
// declared /cyclops/sets/{setName}/tag/{tagName} while the router served the
// plural form, so the declared permissions guarded a path that did not exist.
// This test fails on any such divergence, in either direction.

const moduleDescriptorPath = "../descriptors/ModuleDescriptor-template.json"

type moduleDescriptor struct {
	Provides []struct {
		Handlers []struct {
			Methods     []string `json:"methods"`
			PathPattern string   `json:"pathPattern"`
		} `json:"handlers"`
	} `json:"provides"`
}

// routedPaths walks the router the constructor installed, returning the set of
// "METHOD /path" strings it serves under /cyclops. Paths outside that prefix
// (the static area, /admin/health) are deliberately not declared to Okapi.
func routedPaths(t *testing.T) map[string]bool {
	t.Helper()

	server := newTestServer(&fakeCCMS{})
	routes, ok := server.httpServer.Handler.(chi.Routes)
	if !ok {
		t.Fatal("the installed handler is not a chi router")
	}

	paths := map[string]bool{}
	err := chi.Walk(routes, func(method string, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		if strings.HasPrefix(route, "/cyclops/") {
			paths[method+" "+route] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("could not walk the router: %v", err)
	}
	return paths
}

// declaredPaths reads the same set of strings out of the ModuleDescriptor.
func declaredPaths(t *testing.T) map[string]bool {
	t.Helper()

	bytes, err := os.ReadFile(moduleDescriptorPath)
	if err != nil {
		t.Fatalf("could not read %s: %v", moduleDescriptorPath, err)
	}

	var descriptor moduleDescriptor
	err = json.Unmarshal(bytes, &descriptor)
	if err != nil {
		t.Fatalf("could not parse %s: %v", moduleDescriptorPath, err)
	}

	paths := map[string]bool{}
	for _, provided := range descriptor.Provides {
		for _, handler := range provided.Handlers {
			for _, method := range handler.Methods {
				paths[method+" "+handler.PathPattern] = true
			}
		}
	}
	return paths
}

// missingFrom returns the members of want that have absent from, sorted so the
// failure output is stable.
func missingFrom(want map[string]bool, have map[string]bool) []string {
	var missing []string
	for path := range want {
		if !have[path] {
			missing = append(missing, path)
		}
	}
	sort.Strings(missing)
	return missing
}

func TestModuleDescriptorMatchesRoutes(t *testing.T) {
	routed := routedPaths(t)
	declared := declaredPaths(t)

	if len(routed) == 0 {
		t.Fatal("walked the router but found no /cyclops paths")
	}

	// Note that this compares the path patterns verbatim, so the parameter
	// names must agree too: the router's {projectId} must not be the
	// descriptor's {id}. Okapi itself treats any {...} as a wildcard, so this
	// is stricter than it strictly needs to be -- but the descriptor doubles as
	// documentation, and a name that disagrees with the code misleads readers.
	for _, path := range missingFrom(declared, routed) {
		t.Errorf("declared in the ModuleDescriptor but not routed: %s", path)
	}
	for _, path := range missingFrom(routed, declared) {
		t.Errorf("routed but not declared in the ModuleDescriptor: %s", path)
	}
}
