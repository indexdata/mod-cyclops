# To make "z-schema" available:
#	yarn global add z-schema
#
# To make "api_lint.py" and "api_doc.py" available:
#	git clone git@github.com:folio-org/folio-tools
# and follow the instructions in its "api-lint" and "api-doc" directories

# This VENV setting is for Mike's laptop, and ought to be parameterized
VENV=/Users/mike/git/folio/other/folio-tools/api-lint/.venv

lint: schemalint examplelint apilint

schemalint:
	z-schema tag-schema.json
	z-schema tags-schema.json

examplelint:
	z-schema tag-schema.json examples/tag-example.json
	z-schema tags-schema.json examples/tags-example.json

apilint: cyclops.raml
	env PATH=$(VENV)/bin:$$PATH api_lint.py -t RAML -d .

doc:
	env PATH=$(VENV)/bin:$$PATH api_doc.py -o doc -t RAML -d .

clean:
	rm -rf doc

