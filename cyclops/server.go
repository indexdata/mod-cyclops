package cyclops

import "fmt"
import "net/http"
import "time"
import "github.com/go-chi/chi/v5"
import "github.com/MikeTaylor/catlogger"
import "github.com/indexdata/ccms"

type HTTPError struct {
	status  int
	message string
}

func (m *HTTPError) Error() string {
	return m.message
}

type ModCyclopsServer struct {
	logger     *catlogger.Logger
	ccmsClient *ccms.Client
	httpServer http.Server
}

func MakeModCyclopsServer(logger *catlogger.Logger, ccmsClient *ccms.Client, root string, timeout int) *ModCyclopsServer {
	tr := &http.Transport{}
	tr.RegisterProtocol("file", http.NewFileTransport(http.Dir(root)))

	r := chi.NewRouter()
	var server = ModCyclopsServer{
		logger:     logger,
		ccmsClient: ccmsClient,
		httpServer: http.Server{
			ReadTimeout:  time.Duration(timeout) * time.Second,
			WriteTimeout: time.Duration(timeout) * time.Second,
			Handler:      r,
		},
	}

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			server.Log("path", req.Method, req.URL.Path)
			next.ServeHTTP(w, req)
		})
	})

	fs := http.FileServer(http.Dir(root + "/htdocs"))
	r.Get("/", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintln(w, `<a href="/htdocs/">Static area</a>`)
	})
	r.Handle("/htdocs/*", http.StripPrefix("/htdocs/", fs))
	r.Handle("/favicon.ico", fs)
	r.Get("/admin/health", func(w http.ResponseWriter, req *http.Request) {
		_, _ = fmt.Fprintln(w, "Behold! I live!!")
	})
	r.Get("/cyclops/tags", func(w http.ResponseWriter, req *http.Request) {
		server.runWithErrorHandling(w, req, server.handleShowTags, "show tags")
	})
	r.Post("/cyclops/tags", func(w http.ResponseWriter, req *http.Request) {
		server.runWithErrorHandling(w, req, server.handleDefineTag, "define tag")
	})
	r.Get("/cyclops/filters", func(w http.ResponseWriter, req *http.Request) {
		server.runWithErrorHandling(w, req, server.handleShowFilters, "show filters")
	})
	r.Post("/cyclops/filters", func(w http.ResponseWriter, req *http.Request) {
		server.runWithErrorHandling(w, req, server.handleDefineFilter, "define filter")
	})
	r.Get("/cyclops/sets", func(w http.ResponseWriter, req *http.Request) {
		server.runWithErrorHandling(w, req, server.handleShowSets, "show sets")
	})
	r.Post("/cyclops/sets", func(w http.ResponseWriter, req *http.Request) {
		server.runWithErrorHandling(w, req, server.handleCreateSet, "create set")
	})
	r.Get("/cyclops/sets/{setName}", func(w http.ResponseWriter, req *http.Request) {
		server.runWithErrorHandling(w, req, server.handleRetrieve, "retrieve")
	})
	r.Delete("/cyclops/sets/{setName}", func(w http.ResponseWriter, req *http.Request) {
		server.runWithErrorHandling(w, req, server.handleDropSet, "drop set")
	})
	r.Post("/cyclops/sets/{setName}/add", func(w http.ResponseWriter, req *http.Request) {
		server.runWithErrorHandling(w, req, server.handleAddObjects, "add objects")
	})
	r.Post("/cyclops/sets/{setName}/remove", func(w http.ResponseWriter, req *http.Request) {
		server.runWithErrorHandling(w, req, server.handleRemoveObjects, "remove objects")
	})
	r.Post("/cyclops/sets/{setName}/tag/{tagName}", func(w http.ResponseWriter, req *http.Request) {
		server.runWithErrorHandling(w, req, server.handleAddRemoveTags, "add/remove tags")
	})
	r.Get("/cyclops/projects", func(w http.ResponseWriter, req *http.Request) {
		server.runWithErrorHandling(w, req, server.handleShowProjects, "show projects")
	})
	r.Get("/cyclops/projects/{projectId}", func(w http.ResponseWriter, req *http.Request) {
		server.runWithErrorHandling(w, req, server.handleFetchProject, "fetch project")
	})
	r.Post("/cyclops/projects", func(w http.ResponseWriter, req *http.Request) {
		server.runWithErrorHandling(w, req, server.handleCreateProject, "create project")
	})
	r.Delete("/cyclops/projects/{projectId}", func(w http.ResponseWriter, req *http.Request) {
		server.runWithErrorHandling(w, req, server.handleDeleteProject, "delete project")
	})
	r.Put("/cyclops/projects/{projectId}", func(w http.ResponseWriter, req *http.Request) {
		server.runWithErrorHandling(w, req, server.handleUpdateProject, "update project")
	})
	r.Get("/cyclops/funds", func(w http.ResponseWriter, req *http.Request) {
		server.runWithErrorHandling(w, req, server.handleShowFunds, "show funds")
	})
	r.Post("/cyclops/funds", func(w http.ResponseWriter, req *http.Request) {
		server.runWithErrorHandling(w, req, server.handleCreateFund, "create fund")
	})
	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		status := http.StatusNotFound
		message := http.StatusText(status)
		w.WriteHeader(status)
		_, _ = fmt.Fprintln(w, message)
		server.Log("error", fmt.Sprintf("%s %s: %d %s", req.Method, req.RequestURI, status, message))
	})

	return &server
}

func (server *ModCyclopsServer) Log(cat string, args ...string) {
	server.logger.Log(cat, args...)
}

func (server *ModCyclopsServer) Launch(host string, port int) error {
	hostspec := host + ":" + fmt.Sprint(port)
	server.httpServer.Addr = hostspec
	server.Log("listen", "listening on", hostspec)
	err := server.httpServer.ListenAndServe()
	server.Log("listen", "finished listening on", hostspec)
	return err
}

type handlerFn func(w http.ResponseWriter, req *http.Request, caption string) error

func (server *ModCyclopsServer) runWithErrorHandling(w http.ResponseWriter, req *http.Request, f handlerFn, caption string) {
	sent, err := server.respondWithDummy(w, caption)
	if sent {
		return
	} else if err != nil {
		err = fmt.Errorf("could not make dummy response: %w", err)
	} else {
		err = f(w, req, caption)
	}

	if err != nil {
		var status int
		switch e := err.(type) {
		case *HTTPError:
			status = e.status
		default:
			status = http.StatusInternalServerError
		}
		w.WriteHeader(status)
		_, _ = fmt.Fprintln(w, err.Error())
		message := http.StatusText(status)
		server.Log("error", fmt.Sprintf("%s %s: %d %s: %s", req.Method, req.RequestURI, status, message, err.Error()))
	}
}
