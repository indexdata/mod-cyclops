SRC=main.go server.go
TESTSRC=#config-file_test.go
TARGET=../target/mod-cyclops

$(TARGET): $(SRC)
	go build -o $@

run: $(TARGET)
	env LOGCAT=hello,listen,path,error $(TARGET)

lint:
	-go vet ./...
	-go vet -vettool=/Users/mike/go/bin/shadow ./...
	-! egrep -n '([ 	]+$$|if +\(|;[ 	]*$$)' *.go | grep -v ':[A-Z][A-Z][A-Z][A-Z]'
	-staticcheck ./... | (grep -v '^/usr/local/go/src/runtime/' || true)
	-errcheck -exclude .errcheck-exclude ./...
	-ineffassign ./...
	-deadcode ./...
	-govulncheck ./...

test:
	go test -v -coverprofile=coverage.out ./...
	go test -json -coverprofile=coverage.out ./... > coverage.json
	@echo "go tool cover -func=coverage.out | sed 's/^github.com\/folio-org\/mod-cyclops\/src\///'"

cover: coverage.out
	go tool cover -html=coverage.out

fmt:
	go fmt ./...

clean:
	rm -f $(TARGET) coverage.out coverage.json

