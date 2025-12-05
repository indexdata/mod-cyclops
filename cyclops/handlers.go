package cyclops

import "errors"
import "strings"
import "io"
import "fmt"
import "net/http"
import "encoding/json"
import "github.com/go-chi/chi/v5"
import "github.com/indexdata/ccms"

type TagList struct {
	Tags []string `json:"tags"`
	// No other elements yet, but use a structure for future expansion
}

func (server *ModCyclopsServer) handleShowTags(w http.ResponseWriter, req *http.Request) error {
	tags := []string{"foo", "bar", "baz"}
	tagList := TagList{Tags: tags}
	return sendJSON(w, tagList, "SHOW TAGS")
}

// -----------------------------------------------------------------------------

type DefineTag struct {
	Name string `json:"name"`
}

func (server *ModCyclopsServer) handleDefineTag(w http.ResponseWriter, req *http.Request) error {
	var tag DefineTag
	err := unmarshalBody(req, &tag)
	if err != nil {
		return fmt.Errorf("define tag: %w", err)
	}

	setName := tag.Name
	command := "define tag " + setName
	server.Log("command", command)

	resp, err := server.ccmsClient.Send(command)
	if err != nil {
		return fmt.Errorf("could not define tag %s: %w", setName, err)
	}
	if resp.Status == "error" {
		return fmt.Errorf("define tag %s failed: %s", setName, resp.Message)
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

// -----------------------------------------------------------------------------

type FilterList struct {
	Filters []string `json:"filters"`
	// No other elements yet, but use a structure for future expansion
}

func (server *ModCyclopsServer) handleShowFilters(w http.ResponseWriter, req *http.Request) error {
	resp, err := server.ccmsClient.Send("show filters")
	if err != nil {
		return fmt.Errorf("could not show filters: %w", err)
	}
	if resp.Status == "error" {
		return fmt.Errorf("show filters failed: %s", resp.Message)
	}

	filters := make([]string, len(resp.Data))
	for i, val := range resp.Data {
		filters[i] = val.Values[0]
	}
	filterList := FilterList{Filters: filters}
	return sendJSON(w, filterList, "SHOW FILTERS")
}

// -----------------------------------------------------------------------------

func (server *ModCyclopsServer) handleDefineFilter(w http.ResponseWriter, req *http.Request) error {
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// -----------------------------------------------------------------------------

type SetList struct {
	Sets []string `json:"sets"`
	// No other elements yet, but use a structure for future expansion
}

func (server *ModCyclopsServer) handleShowSets(w http.ResponseWriter, req *http.Request) error {
	resp, err := server.ccmsClient.Send("show sets")
	if err != nil {
		return fmt.Errorf("could not fetch show-sets response: %w", err)
	}

	sets := make([]string, len(resp.Data))
	for i, val := range resp.Data {
		sets[i] = val.Values[0]
	}
	setList := SetList{Sets: sets}
	return sendJSON(w, setList, "SHOW SETS")
}

// -----------------------------------------------------------------------------

func (server *ModCyclopsServer) handleCreateSet(w http.ResponseWriter, req *http.Request) error {
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// -----------------------------------------------------------------------------

func makeRetrieveCommand(req *http.Request) (string, error) {
	var b strings.Builder

	fields := req.URL.Query().Get("fields")
	if fields == "" {
		return "", errors.New("no 'fields' parameter supplied")
	}
	b.WriteString("retrieve ")
	b.WriteString(fields)

	setName := chi.URLParam(req, "setName")
	b.WriteString(" from ")
	b.WriteString(setName)

	cond := req.URL.Query().Get("cond")
	if cond != "" {
		b.WriteString(" where ")
		b.WriteString(cond)
	}

	filter := req.URL.Query().Get("filter")
	if filter != "" {
		b.WriteString(" filter ")
		b.WriteString(filter)
	}

	tag := req.URL.Query().Get("tag")
	omitTag := req.URL.Query().Get("omitTag")
	if tag != "" && omitTag != "" {
		return "", errors.New("both 'tag' and 'omitTag' parameters supplied")
	}

	if tag != "" {
		b.WriteString(" tag ")
		b.WriteString(tag)
	} else if omitTag != "" {
		b.WriteString(" tag not ")
		b.WriteString(omitTag)
	}

	sort := req.URL.Query().Get("sort")
	if sort != "" {
		b.WriteString(" sort ")
		b.WriteString(sort)
	}

	offset := req.URL.Query().Get("offset")
	if offset != "" {
		b.WriteString(" offset ")
		b.WriteString(offset)
	}

	limit := req.URL.Query().Get("limit")
	if limit != "" {
		b.WriteString(" limit ")
		b.WriteString(limit)
	}

	return b.String(), nil
}

// It's annoying that we have to make these descriptions that are
// parallel to those in the ccms package, but it seems the only way to
// specify the JSON encoding.

type FieldDescription struct {
	Name string `json:"name"`
	// No other elements yet, but use a structure for future expansion
}

type DataRow struct {
	Values []string `json:"values"`
	// No other elements yet, but use a structure for future expansion
}

type RetrieveResponse struct {
	Status  string             `json:"status"`
	Fields  []FieldDescription `json:"fields"`
	Data    []DataRow          `json:"data"`
	Message string             `json:"message"`
}

func makeDummyResponse() *ccms.Response {
	field1 := ccms.FieldDescription{Name: "id"}
	field2 := ccms.FieldDescription{Name: "title"}
	fields := []ccms.FieldDescription{field1, field2}
	datum1 := ccms.DataRow{Values: []string{"123", "The Lord of the Rings"}}
	datum2 := ccms.DataRow{Values: []string{"456", "The Hitch Hiker's Guide to the Galaxy"}}
	datum3 := ccms.DataRow{Values: []string{"789", "The Man Who Was Thursday"}}
	data := []ccms.DataRow{datum1, datum2, datum3}
	rr := ccms.Response{
		Status:  "retrieve",
		Fields:  fields,
		Data:    data,
		Message: "",
	}
	return &rr
}

// Translate from CCMS's structure into an identical one with JSON encoding instructions
// It feels like there has to be a better way to do this
func ccms2local(rr *ccms.Response) RetrieveResponse {
	localFields := make([]FieldDescription, len(rr.Fields))
	for i, val := range rr.Fields {
		localFields[i].Name = val.Name
	}

	localData := make([]DataRow, len(rr.Data))
	for i, val := range rr.Data {
		localData[i] = DataRow{
			Values: make([]string, len(val.Values)),
		}
		copy(localData[i].Values, val.Values)
	}

	return RetrieveResponse{
		Status:  rr.Status,
		Fields:  localFields,
		Data:    localData,
		Message: rr.Message,
	}
}

func (server *ModCyclopsServer) handleRetrieve(w http.ResponseWriter, req *http.Request) error {
	command, err := makeRetrieveCommand(req)
	if err != nil {
		return fmt.Errorf("could not make retrieve command: %w", err)
	}
	server.Log("command", command)

	// Use the dummy until CCMS can provide a response
	var resp *ccms.Response
	if false {
		resp, err = server.ccmsClient.Send(command)
		if err != nil {
			return fmt.Errorf("could not retrieve: %w", err)
		}
		if resp.Status == "error" {
			return fmt.Errorf("retrieve failed: %s", resp.Message)
		}
	} else {
		resp = makeDummyResponse()
	}

	localrr := ccms2local(resp)

	return sendJSON(w, localrr, "RETRIEVE")
}

// -----------------------------------------------------------------------------

func (server *ModCyclopsServer) handleAddRemoveObjects(w http.ResponseWriter, req *http.Request) error {
	// It seems weird to just shrug and say "fine" for anything posted, but for now it will suffice.
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// -----------------------------------------------------------------------------

func (server *ModCyclopsServer) handleAddRemoveTags(w http.ResponseWriter, req *http.Request) error {
	// It seems weird to just shrug and say "fine" for anything posted, but for now it will suffice.
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// -----------------------------------------------------------------------------

func unmarshalBody[T any](req *http.Request, data *T) error {
	b, err := io.ReadAll(req.Body)
	if err != nil {
		return fmt.Errorf("could not read HTTP request body: %w", err)
	}

	err = json.Unmarshal(b, &data)
	if err != nil {
		return fmt.Errorf("could not deserialize JSON from body: %w", err)
	}

	return nil
}

func sendJSON(w http.ResponseWriter, data any, caption string) error {
	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("could not encode JSON for %s: %w", caption, err)
	}

	w.Header().Set("Content-Type", "application/json")

	// If w.write fails there is no way to report this to the client: see MODREP-37.
	_, _ = w.Write(b)
	return nil
}
