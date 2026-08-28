# Test cases for cond-schema.json.
#
# To make "z-schema" available:
#	yarn global add z-schema
#
# Every file in valid/ must satisfy the schema, and every file in invalid/
# must be rejected by it.

SCHEMA=../cond-schema.json
EXAMPLE=../examples/cond.json
VALID=$(wildcard valid/*.json)
INVALID=$(wildcard invalid/*.json)

test: schemalint validtest invalidtest
	@echo "All condition-schema tests passed"

# The schema itself must be a well-formed JSON Schema.
schemalint:
	z-schema $(SCHEMA)

# Documents that the schema must accept, including the RAML example itself.
validtest:
	@for f in $(EXAMPLE) $(VALID); do \
		if z-schema $(SCHEMA) $$f > /dev/null 2>&1; then \
			echo "ok       accepted $$f"; \
		else \
			echo "NOT OK   wrongly rejected $$f"; \
			z-schema $(SCHEMA) $$f; \
			exit 1; \
		fi; \
	done

# Documents that the schema must reject. A case that starts being accepted
# is a hole in the validation, so this target fails when z-schema succeeds.
invalidtest:
	@for f in $(INVALID); do \
		if z-schema $(SCHEMA) $$f > /dev/null 2>&1; then \
			echo "NOT OK   wrongly accepted $$f"; \
			exit 1; \
		else \
			echo "ok       rejected $$f"; \
		fi; \
	done

.PHONY: test schemalint validtest invalidtest
