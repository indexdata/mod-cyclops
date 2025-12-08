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
	sent, err := server.sendDummyResponse(w, "SHOW TAGS")
	if err != nil {
		return fmt.Errorf("could not make dummy response: %w", err)
	} else if sent {
		return nil
	}

	resp, err := server.sendToCCMS("show tags", "show tags")
	if err != nil {
		return err
	}

	tags := make([]string, len(resp.Data))
	for i, val := range resp.Data {
		tags[i] = val.Values[0]
	}
	tagList := TagList{Tags: tags}
	return respondWithJSON(w, tagList, "SHOW TAGS")
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

	command := "define tag " + tag.Name
	server.Log("command", command)

	resp, err := server.sendToCCMS("define tag "+tag.Name, command)
	if err != nil {
		return err
	}
	fmt.Printf("define tag response: %+v\n", resp)

	w.WriteHeader(http.StatusNoContent)
	return nil
}

// -----------------------------------------------------------------------------

type FilterList struct {
	Filters []string `json:"filters"`
	// No other elements yet, but use a structure for future expansion
}

func (server *ModCyclopsServer) handleShowFilters(w http.ResponseWriter, req *http.Request) error {
	resp, err := server.sendToCCMS("show filters", "show filters")
	if err != nil {
		return err
	}

	filters := make([]string, len(resp.Data))
	for i, val := range resp.Data {
		filters[i] = val.Values[0]
	}
	filterList := FilterList{Filters: filters}
	return respondWithJSON(w, filterList, "SHOW FILTERS")
}

// -----------------------------------------------------------------------------

type DefineFilter struct {
	Name     string `json:"name"`
	Cond     string `json:"cond"`
	Template string `json:"template"`
}

func (server *ModCyclopsServer) handleDefineFilter(w http.ResponseWriter, req *http.Request) error {
	var filter DefineFilter
	err := unmarshalBody(req, &filter)
	if err != nil {
		return fmt.Errorf("define filter: %w", err)
	}

	command := "define filter " + filter.Name
	if filter.Cond != "" {
		command += " where " + filter.Cond
	}
	if filter.Template != "" {
		command += " template " + filter.Template
	}
	server.Log("command", command)

	resp, err := server.sendToCCMS("define filter "+filter.Name, command)
	if err != nil {
		return err
	}
	fmt.Printf("define filter response: %+v\n", resp)

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
	return respondWithJSON(w, setList, "SHOW SETS")
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
	sent, err := server.sendDummyResponse(w, "RETRIEVE")
	if err != nil {
		return fmt.Errorf("could not make dummy response: %w", err)
	} else if sent {
		return nil
	}

	command, err := makeRetrieveCommand(req)
	if err != nil {
		return fmt.Errorf("could not make retrieve command: %w", err)
	}
	server.Log("command", command)

	resp, err := server.ccmsClient.Send(command)
	if err != nil {
		return fmt.Errorf("could not retrieve: %w", err)
	}
	if resp.Status == "error" {
		return fmt.Errorf("retrieve failed: %s", resp.Message)
	}

	localrr := ccms2local(resp)

	return respondWithJSON(w, localrr, "RETRIEVE")
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

func (server *ModCyclopsServer) sendToCCMS(caption string, command string) (*ccms.Response, error) {
	resp, err := server.ccmsClient.Send(command)
	if err != nil {
		return nil, fmt.Errorf("could not %s: %w", caption, err)
	}
	if resp.Status == "error" {
		return nil, fmt.Errorf("%s failed: %s", caption, resp.Message)
	}
	return resp, nil
}

func respondWithJSON(w http.ResponseWriter, data any, caption string) error {
	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("could not encode JSON for %s: %w", caption, err)
	}

	w.Header().Set("Content-Type", "application/json")

	// If w.write fails there is no way to report this to the client: see MODREP-37.
	_, _ = w.Write(b)
	return nil
}
