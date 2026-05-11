# See also: ramls/Makefile (used only for validation and documentation)

ARTIFACTID=`sed -n 's/^module .*\/\(.*\)/\1/p' go.mod`
VERSION ?= `git describe --tags --abbrev=0 | sed 's/^v\([0-9]\)/\1/'`
DESCRIPTORS=Activate DeploymentDescriptor Discovery ModuleDescriptor
TARGET_DESCRIPTORS=$(DESCRIPTORS:%=target/%.json)
SRC=main.go cyclops/server.go cyclops/handlers.go cyclops/dummy.go cyclops/respToString.go
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
	-cd cyclops; golangci-lint run
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

run-dummy: $(TARGET)
	env MOD_CYCLOPS_DUMMY_DATA=1 ./run.sh

