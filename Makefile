# See also: ramls/Makefile (used only for validation and documentation)

**default**: target/ModuleDescriptor.json target/mod-cyclops

target/ModuleDescriptor.json:
	(cd target; make)

target/mod-cyclops:
	(cd src; make)

clean:
	(cd target; make clean)

