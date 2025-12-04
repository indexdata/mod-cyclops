# See also: ramls/Makefile (used only for validation and documentation)

ARTIFACTID=`sed -n 's/^module .*\/\(.*\)/\1/p' go.mod`
VERSION ?= `git describe --tags --abbrev=0 | sed 's/^v\([0-9]\)/\1/'`
DESCRIPTORS=Activate DeploymentDescriptor Discovery ModuleDescriptor
TARGET_DESCRIPTORS=$(DESCRIPTORS:%=target/%.json)
SRC=main.go cyclops/server.go cyclops/handlers.go
TARGET=target/mod-cyclops

**default**: $(TARGET_DESCRIPTORS) $(TARGET) 

debug:
	@echo ARTIFACTID=$(ARTIFACTID)
	@echo VERSION=$(VERSION)
	@echo TARGET_DESCRIPTORS=$(TARGET_DESCRIPTORS)

target/%.json: descriptors/%-template.json
	rm -f $@
	sed "s/@artifactId@/$(ARTIFACTID)/g;s/@version@/$(VERSION)/g" $< > $@
	chmod ugo-w $@

$(TARGET): $(SRC)
	go build -o $@

# Lint is usually run from within the cyclops directory where the
# majority of the source code is, so we run the individual parts of
# the linting script from there so that relative paths are correct
#
lint:
	-cd cyclops; go vet ../...
	-cd cyclops; go vet -vettool=/Users/mike/go/bin/shadow ../...
	-cd cyclops; ! egrep -n '([ 	]+$$|if +\(|;[ 	]*$$)' ../main.go *.go | grep -v ':[A-Z][A-Z][A-Z][A-Z]'
	-cd cyclops; staticcheck ../... | (grep -v '^/usr/local/go/src/runtime/' || true)
	-cd cyclops; errcheck -exclude .errcheck-exclude ../...
	-cd cyclops; ineffassign ../...
	-cd cyclops; deadcode ../...
	-cd cyclops; govulncheck ../...

test:
	go test -v -coverprofile=coverage.out ./...
	go test -json -coverprofile=coverage.out ./... > coverage.json
	@echo "go tool cover -func=coverage.out | sed 's/^github.com\/folio-org\/mod-cyclops\/src\///'"

cover: coverage.out
	go tool cover -html=coverage.out

fmt:
	go fmt ./...

clean:
	rm -f $(TARGET_DESCRIPTORS) $(TARGET) coverage.out coverage.json

run: $(TARGET)
	./run.sh

