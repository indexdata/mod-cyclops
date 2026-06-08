# To make "z-schema" available:
#	yarn global add z-schema
#
# To make "api_lint.py" and "api_doc.py" available:
#	git clone git@github.com:folio-org/folio-tools
# and follow the instructions in its "api-lint" and "api-doc" directories

# This VENV setting is for Mike's laptop, and ought to be parameterized
#VENV=/Users/mike/git/folio/other/folio-tools/api-lint/.venv
VENV=/Users/mike/.local/share/virtualenvs/api-lint-dv5iwVpa

lint: schemalint examplelint apilint docgen

schemalint:
	z-schema tag-schema.json
	z-schema tags-schema.json
	z-schema filter-schema.json
	z-schema filters-schema.json
	z-schema set-schema.json
	z-schema sets-schema.json
	z-schema retrieve-schema.json
	z-schema addobjects-schema.json
	z-schema removeobjects-schema.json
	z-schema addremovetag-schema.json
	z-schema project-schema.json
	z-schema projects-schema.json
	z-schema fund-schema.json
	z-schema funds-schema.json

examplelint:
	z-schema tag-schema.json examples/tag-example.json
	z-schema tags-schema.json examples/show-tags-example.json
	z-schema filter-schema.json examples/filter-example.json
	z-schema filters-schema.json examples/show-filters-example.json
	z-schema set-schema.json examples/set-example.json
	z-schema sets-schema.json examples/show-sets-example.json
	z-schema retrieve-schema.json examples/retrieve-example.json
	z-schema addobjects-schema.json examples/addobject-example.json
	z-schema removeobjects-schema.json examples/removeobject-example.json
	z-schema addremovetag-schema.json examples/addtag-example.json
	z-schema addremovetag-schema.json examples/removetag-example.json
	z-schema project-schema.json examples/project-example.json
	z-schema projects-schema.json examples/show-projects-example.json
	z-schema fund-schema.json examples/fund-example.json
	z-schema funds-schema.json examples/show-funds-example.json

apilint: cyclops.raml
	env PATH=$(VENV)/bin:$$PATH api_lint.py -t RAML -d .

docgen:
	env PATH=$(VENV)/bin:$$PATH api_doc.py -o doc -t RAML -d .

clean:
	rm -rf doc

