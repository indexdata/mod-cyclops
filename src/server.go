package main

import "fmt"
import "net/http"
import "time"
import "html"
import "encoding/json"
import "github.com/MikeTaylor/catlogger"

type HTTPError struct {
	status  int
	message string
}

func (m *HTTPError) Error() string {
	return m.message
}

type ModCyclopsServer struct {
	root       string
	logger     *catlogger.Logger
	httpServer http.Server
}

func MakeModCyclopsServer(logger *catlogger.Logger, root string, timeout int) *ModCyclopsServer {
	tr := &http.Transport{}
	tr.RegisterProtocol("file", http.NewFileTransport(http.Dir(root)))

	mux := http.NewServeMux()
	var server = ModCyclopsServer{
		logger: logger,
		root:   root,
		httpServer: http.Server{
			ReadTimeout:  time.Duration(timeout) * time.Second,
			WriteTimeout: time.Duration(timeout) * time.Second,
			Handler:      mux,
		},
	}

	fs := http.FileServer(http.Dir(root + "/htdocs"))
	mux.Handle("/htdocs/", http.StripPrefix("/htdocs/", fs))
	mux.Handle("/favicon.ico", fs)
	mux.HandleFunc("/admin/health", func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprintln(w, "Behold! I live!!")
	})
	mux.HandleFunc("/cyclops/tags", func(w http.ResponseWriter, req *http.Request) {
		server.runWithErrorHandling(w, req, server.handleListTags)
	})
	mux.HandleFunc("/cyclops/sets/", func(w http.ResponseWriter, req *http.Request) {
		server.runWithErrorHandling(w, req, server.handleRetrieve)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintln(w, `<a href="/htdocs/">Static area</a>`)
	})
	mux.HandleFunc("/cyclops/{anythingElse...}", server.handler)

	return &server
}

func (server *ModCyclopsServer) Log(cat string, args ...string) {
	server.logger.Log(cat, args...)
}

func (server *ModCyclopsServer) launch(host string, port int) error {
	hostspec := host + ":" + fmt.Sprint(port)
	server.httpServer.Addr = hostspec
	server.Log("listen", "listening on", hostspec)
	err := server.httpServer.ListenAndServe()
	server.Log("listen", "finished listening on", hostspec)
	return err
}

func (server *ModCyclopsServer) handler(w http.ResponseWriter, req *http.Request) {
	method := req.Method
	path := req.URL.Path
	server.Log("path", method, path)

	if method == "POST" && path == "/cyclops/tags" {
		server.runWithErrorHandling(w, req, server.handleDefineTag)
	} else {
		status := http.StatusNotFound
		message := http.StatusText(status)
		w.WriteHeader(status)
		fmt.Fprintln(w, message)
		server.Log("error", fmt.Sprintf("%s %s: %d %s", req.Method, req.RequestURI, status, message))
	}
}

type handlerFn func(w http.ResponseWriter, req *http.Request) error

func (server *ModCyclopsServer) runWithErrorHandling(w http.ResponseWriter, req *http.Request, f handlerFn) {
	err := f(w, req)
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

type TagList struct {
	Tags []string `json:"tags"`
	// No other elements yet, but use a structure for future expansion
}

func (server *ModCyclopsServer) handleListTags(w http.ResponseWriter, req *http.Request) error {
	tags := []string{"foo", "bar", "baz"}
	tagList := TagList{Tags: tags}
	fmt.Printf("tagList = %+v\n", tagList)
	return sendJSON(w, tagList, "LIST TAGS")
}

func (server *ModCyclopsServer) handleDefineTag(w http.ResponseWriter, req *http.Request) error {
	w.WriteHeader(http.StatusNoContent)
	return nil
}

type FieldDescription struct {
	Name string
	// No other elements yet, but use a structure for future expansion
}

type DataRow struct {
	Values []string
	// No other elements yet, but use a structure for future expansion
}

type RetrieveResponse struct {
	Status  string
	Fields  []FieldDescription
	Data    []DataRow
	Message string
}

func (server *ModCyclopsServer) handleRetrieve(w http.ResponseWriter, req *http.Request) error {
	field1 := FieldDescription{Name: "id"}
	field2 := FieldDescription{Name: "title"}
	fields := []FieldDescription{field1, field2}
	datum1 := DataRow{Values: []string{"123", "The Lord of the Rings"}}
	datum2 := DataRow{Values: []string{"456", "The Hitch Hiker's Guide to the Galaxy"}}
	datum3 := DataRow{Values: []string{"789", "The Man Who Was Thursday"}}
	data := []DataRow{datum1, datum2, datum3}
	rr := RetrieveResponse{
		Status:  "retrieve",
		Fields:  fields,
		Data:    data,
		Message: "",
	}
	return sendJSON(w, rr, "RETRIEVE")
}

func sendJSON(w http.ResponseWriter, data any, caption string) error {
	bytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("could not encode JSON for %s: %w", caption, err)
	}

	w.Header().Set("Content-Type", "application/json")

	// If w.write fails there is no way to report this to the client: see MODREP-37.
	_, _ = w.Write(bytes)
	return nil
}
