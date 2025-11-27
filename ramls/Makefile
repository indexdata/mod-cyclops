# To make "z-schema" available:
#	yarn global add z-schema
#
# To make "api_lint.py" and "api_doc.py" available:
#	git clone git@github.com:folio-org/folio-tools
# and follow the instructions in its "api-lint" and "api-doc" directories

# This VENV setting is for Mike's laptop, and ought to be parameterized
VENV=/Users/mike/git/folio/other/folio-tools/api-lint/.venv

lint: schemalint examplelint apilint docgen

schemalint:
	z-schema tag-schema.json
	z-schema tags-schema.json
	z-schema filter-schema.json
	z-schema filters-schema.json
	z-schema set-schema.json
	z-schema sets-schema.json
	z-schema retrieve-schema.json
	z-schema addremove-schema.json

examplelint:
	z-schema tag-schema.json examples/tag-example.json
	z-schema tags-schema.json examples/tags-example.json
	z-schema filter-schema.json examples/filter-example.json
	z-schema filters-schema.json examples/filters-example.json
	z-schema set-schema.json examples/set-example.json
	z-schema sets-schema.json examples/sets-example.json
	z-schema retrieve-schema.json examples/retrieve-example.json
	z-schema addremove-schema.json examples/add-example.json
	z-schema addremove-schema.json examples/remove-example.json

apilint: cyclops.raml
	env PATH=$(VENV)/bin:$$PATH api_lint.py -t RAML -d .

docgen:
	env PATH=$(VENV)/bin:$$PATH api_doc.py -o doc -t RAML -d .

clean:
	rm -rf doc

