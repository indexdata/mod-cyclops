# To make "z-schema" available:
#	yarn global add z-schema
#
# To make "api_lint.py" and "api_doc.py" available:
#	git clone git@github.com:folio-org/folio-tools
# and follow the instructions in its "api-lint" and "api-doc" directories

lint: schemalint examplelint # apilint

schemalint:
	z-schema tag-schema.json

examplelint:
	z-schema tag-schema.json examples/tag-example.json

apilint: ldp.raml
	api_lint.py -t RAML -d .

doc:
	api_doc.py -o doc -t RAML -d .

