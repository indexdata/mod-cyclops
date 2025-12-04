package cyclops

import "fmt"
import "net/http"
import "encoding/json"

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

func (server *ModCyclopsServer) handleDefineTag(w http.ResponseWriter, req *http.Request) error {
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
		return fmt.Errorf("could not fetch show-filters response: %w", err)
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
