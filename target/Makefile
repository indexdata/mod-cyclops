ARTIFACTID=`sed -n 's/^module .*\/\(.*\)/\1/p' ../go.mod`
VERSION ?= `git describe --tags --abbrev=0 | sed 's/^v\([0-9]\)/\1/'`
TARGETS=Activate.json DeploymentDescriptor.json Discovery.json ModuleDescriptor.json

**make-default**: $(TARGETS)

%.json: ../descriptors/%-template.json
	rm -f $@
	sed "s/@artifactId@/$(ARTIFACTID)/g;s/@version@/$(VERSION)/g" $< > $@
	chmod ugo-w $@

clean:
	rm -f $(TARGETS) mod-cyclops

debug:
	@echo ARTIFACTID=$(ARTIFACTID)
	@echo VERSION=$(VERSION)

