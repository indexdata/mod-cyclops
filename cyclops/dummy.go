package cyclops

import "os"
import "fmt"
import "net/http"

/*
 * This is a bad source file. It exists to provide dummy data until
 * CCMS is working properly. Until then, if the MOD_CYCLOPS_DUMMY_DATA
 * environment variable is set, then a premade response is sent back
 * to the client. Otherwise, an error is returned.
 *
 * It's bad because:
 * 1. It calls Getenv directly, which is otherwise done only in main
 * 2. It knows about data-types otherwise private to handlers.go
 * But all that will go away soon.
 */

func (server *ModCyclopsServer) sendDummyResponse(w http.ResponseWriter, label string) (bool, error) {
	sendDummy := os.Getenv("MOD_CYCLOPS_DUMMY_DATA")
	if sendDummy == "" {
		return false, nil
	}

	if label == "RETRIEVE" {
		resp := makeDummyRetrieveResponse()
		return true, respondWithJSON(w, resp, label)
	}

	return false, fmt.Errorf("'%s' dummy not yet implemented", label)
}

func makeDummyRetrieveResponse() *RetrieveResponse {
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
	return &rr
}
