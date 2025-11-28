package main

import "os"
import "strconv"
import "fmt"
import "regexp"
import "github.com/MikeTaylor/catlogger"

// PRIVATE to this file
type config struct {
	loggingPrefix string
	loggingTimestamp bool
	queryTimeout int
	serverHost string
	serverPort int
}

func buildConfigFromEnv() *config {
	var cfg config

	cfg.loggingPrefix = os.Getenv("LOGGING_PREFIX")

	cfg.loggingTimestamp = false
	tsString := os.Getenv("LOGGING_TIMESTAMP")
	if tsString != "" {
		cfg.loggingTimestamp = true
	}

	timeoutString := os.Getenv("MOD_CYCLOPS_QUERY_TIMEOUT")
	if timeoutString != "" {
		cfg.queryTimeout, _ = strconv.Atoi(timeoutString)
	} else {
		cfg.queryTimeout = 60
	}

	cfg.serverHost = os.Getenv("SERVER_HOST")
	if cfg.serverHost == "" {
		cfg.serverHost = "0.0.0.0"
	}

	serverPortString := os.Getenv("SERVER_PORT")
	if serverPortString != "" {
		cfg.serverPort, _ = strconv.Atoi(serverPortString)
	} else {
		cfg.serverPort = 12370
	}

	return &cfg
}

func main() {
	if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "Usage:", os.Args[0])
		os.Exit(1)
	}

	cfg := buildConfigFromEnv()

	// catlogger.MakeLogger handes the category environment variables on its own
	logger := catlogger.MakeLogger("", cfg.loggingPrefix, cfg.loggingTimestamp)
	// We do not need this transformation yet, but will need something like it when we have authentication
	logger.AddTransformation(regexp.MustCompile(`\\"pass\\":\\"[^"]*\\"`), `\"pass\":\"********\"`)
	logger.Log("hello", "Hello, world!")

	server := MakeModCyclopsServer(logger, ".", cfg.queryTimeout)
	err := server.launch(cfg.serverHost, cfg.serverPort)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: cannot launch server: %s\n", os.Args[0], err)
		os.Exit(3)
	}
}
