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

func (server *ModCyclopsServer) respondWithDummy(w http.ResponseWriter, caption string) (bool, error) {
	sendDummy := os.Getenv("MOD_CYCLOPS_DUMMY_DATA")
	if sendDummy == "" {
		return false, nil
	}

	var data string
	if caption == "show tags" {
		data = dummyShowTagsResponse
	} else if caption == "retrieve" {
		data = dummyRetrieveResponse
	} else {
		return false, fmt.Errorf("'%s' dummy not yet implemented", caption)
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(data))
	return true, nil

}

const dummyShowTagsResponse = `
[
  "foo",
  "bar",
  "baz"
]
`

const dummyRetrieveResponse = `
{
  "status": "retrieve",
  "fields": [
    {
      "name": "id"
    },
    {
      "name": "author"
    },
    {
      "name": "title"
    }
  ],
  "data": [
    {
      "values": [
        "123",
        "J. R. R. Tolkien",
        "The Lord of the Rings"
      ]
    },
    {
      "values": [
        "456",
        "Douglas Adams",
        "The Hitch Hiker's Guide to the Galaxy"
      ]
    },
    {
      "values": [
        "789",
        "G. K. Chesterton",
        "The Man Who Was Thursday"
      ]
    }
  ],
  "message": ""
}
`
