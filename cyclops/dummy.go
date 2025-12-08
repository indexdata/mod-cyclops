package cyclops

import "os"
import "strings"
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

	if caption == "show tags" ||
		caption == "show filters" ||
		caption == "show sets" ||
		caption == "retrieve" {
	} else {
		return false, fmt.Errorf("'%s' dummy not yet implemented", caption)
	}

	frag := strings.ReplaceAll(caption, " ", "-")
	path := "ramls/examples/" + frag + "-example.json"
	data, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("'%s' dummy data file: %w", caption, err)
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
	return true, nil

}
