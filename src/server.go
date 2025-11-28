package main

import "fmt"
import "net/http"
import "time"
import "strings"
import "html"
import "github.com/MikeTaylor/catlogger"

type HTTPError struct {
	status  int
	message string
}

func (m *HTTPError) Error() string {
	return m.message
}

type ModCyclopsServer struct {
	root   string
	logger *catlogger.Logger
	server http.Server
}

type handlerFn func(w http.ResponseWriter, req *http.Request, server *ModCyclopsServer) error

func MakeModCyclopsServer(logger *catlogger.Logger, root string, timeout int) *ModCyclopsServer {
	tr := &http.Transport{}
	tr.RegisterProtocol("file", http.NewFileTransport(http.Dir(root)))

	mux := http.NewServeMux()
	var server = ModCyclopsServer{
		logger: logger,
		root:   root,
		server: http.Server{
			ReadTimeout:  time.Duration(timeout) * time.Second,
			WriteTimeout: time.Duration(timeout) * time.Second,
			Handler:      mux,
		},
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { handler(w, r, &server) })
	fs := http.FileServer(http.Dir(root + "/htdocs"))
	mux.Handle("/htdocs/", http.StripPrefix("/htdocs/", fs))
	fs2 := http.FileServer(http.Dir(root + "/htdocs"))
	mux.Handle("/favicon.ico", fs2)

	return &server
}

func (server *ModCyclopsServer) Log(cat string, args ...string) {
	server.logger.Log(cat, args...)
}

func (server *ModCyclopsServer) launch(host string, port int) error {
	hostspec := host + ":" + fmt.Sprint(port)
	server.server.Addr = hostspec
	server.Log("listen", "listening on", hostspec)
	err := server.server.ListenAndServe()
	server.Log("listen", "finished listening on", hostspec)
	return err
}

func handler(w http.ResponseWriter, req *http.Request, server *ModCyclopsServer) {
	method := req.Method
	path := req.URL.Path
	server.Log("path", method, path)

	if path == "/" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintln(w, `<a href="/htdocs/">Static area</a>`)
		return
	} else if path == "/admin/health" {
		fmt.Fprintln(w, "Behold! I live!!")
		return
	} else if method == "GET" && path == "/cyclops/tags" {
		runWithErrorHandling(w, req, server, handleListTags)
	} else if method == "POST" && path == "/cyclops/tags" {
		runWithErrorHandling(w, req, server, handleDefineTag)
	} else if method == "GET" && strings.HasPrefix(path, "/cyclops/sets/") {
		runWithErrorHandling(w, req, server, handleRetrieve)
	} else {
		status := http.StatusNotFound
		message := http.StatusText(status)
		w.WriteHeader(status)
		fmt.Fprintln(w, message)
		server.Log("error", fmt.Sprintf("%s %s: %d %s", req.Method, req.RequestURI, status, message))
	}
}

func runWithErrorHandling(w http.ResponseWriter, req *http.Request, server *ModCyclopsServer, f handlerFn) {
	err := f(w, req, server)
	if err != nil {
		var status int
		switch e := err.(type) {
		case *HTTPError:
			status = e.status
		default:
			status = http.StatusInternalServerError
		}
		w.WriteHeader(status)
		fmt.Fprintln(w, html.EscapeString(err.Error()))
		message := http.StatusText(status)
		server.Log("error", fmt.Sprintf("%s %s: %d %s: %s", req.Method, req.RequestURI, status, message, err.Error()))
	}
}

func handleListTags(w http.ResponseWriter, req *http.Request, session *ModCyclopsServer) error {
	return &HTTPError{http.StatusNotImplemented, "LIST TAGS incomplete"}
}

func handleDefineTag(w http.ResponseWriter, req *http.Request, session *ModCyclopsServer) error {
	return &HTTPError{http.StatusNotImplemented, "DEFINE TAG incomplete"}
}

func handleRetrieve(w http.ResponseWriter, req *http.Request, session *ModCyclopsServer) error {
	return &HTTPError{http.StatusNotImplemented, "RETRIEVE incomplete"}
}
