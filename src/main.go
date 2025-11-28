package main

import "os"
import "strconv"
import "fmt"
import "regexp"
import "github.com/MikeTaylor/catlogger"

func makeConfiguredLogger() *catlogger.Logger {
	// catlogger.MakeLogger handes the category environment variables on its own
	prefix := os.Getenv("LOGGING_PREFIX")
	timestamp := false
	tsString := os.Getenv("LOGGING_TIMESTAMP")
	if tsString != "" {
		timestamp = true
	}

	logger := catlogger.MakeLogger("", prefix, timestamp)

	// We do not need this transformation yet, but will need something like it when we have authentication
	logger.AddTransformation(regexp.MustCompile(`\\"pass\\":\\"[^"]*\\"`), `\"pass\":\"********\"`)
	return logger
}

func main() {
	if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "Usage:", os.Args[0])
		os.Exit(1)
	}

	logger := makeConfiguredLogger()
	logger.Log("hello", "Hello, world!")

	var timeout int
	timeoutString := os.Getenv("MOD_CYCLOPS_QUERY_TIMEOUT")
	if timeoutString != "" {
		timeout, _ = strconv.Atoi(timeoutString)
	} else {
		timeout = 60
	}

	server := MakeModCyclopsServer(logger, ".", timeout)
	err := server.launch()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: cannot launch server: %s\n", os.Args[0], err)
		os.Exit(3)
	}
}
