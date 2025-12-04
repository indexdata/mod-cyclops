package cyclops

import "fmt"
import "net/http"
import "encoding/json"

type TagList struct {
	Tags []string `json:"tags"`
	// No other elements yet, but use a structure for future expansion
}

func (server *ModCyclopsServer) handleListTags(w http.ResponseWriter, req *http.Request) error {
	tags := []string{"foo", "bar", "baz"}
	tagList := TagList{Tags: tags}
	return sendJSON(w, tagList, "LIST TAGS")
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

func (server *ModCyclopsServer) handleListFilters(w http.ResponseWriter, req *http.Request) error {
	filters := []string{"triassic", "jurassic", "cretaceous"}
	filterList := FilterList{Filters: filters}
	return sendJSON(w, filterList, "LIST FILTERS")
}

// -----------------------------------------------------------------------------

func (server *ModCyclopsServer) handleDefineFilter(w http.ResponseWriter, req *http.Request) error {
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
