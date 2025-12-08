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
 * It's bad because it calls Getenv directly, which is otherwise done
 * only in main. But all this will go away soon.
 */

func (server *ModCyclopsServer) sendDummyResponse(w http.ResponseWriter, label string) (bool, error) {
	sendDummy := os.Getenv("MOD_CYCLOPS_DUMMY_DATA")
	if sendDummy == "" {
		return false, nil
	}

	var data string
	if label == "RETRIEVE" {
		data = dummyRetrieveResponse
	} else {
		return false, fmt.Errorf("'%s' dummy not yet implemented", label)
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(data))
	return true, nil

}

const dummyRetrieveResponse = `
{
  "status": "retrieve",
  "fields": [
    {
      "name": "id"
    },
    {
      "name": "title"
    }
  ],
  "data": [
    {
      "values": [
        "123",
        "The Lord of the Rings"
      ]
    },
    {
      "values": [
        "456",
        "The Hitch Hiker's Guide to the Galaxy"
      ]
    },
    {
      "values": [
        "789",
        "The Man Who Was Thursday"
      ]
    }
  ],
  "message": ""
}
`;
