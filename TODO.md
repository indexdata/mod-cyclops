# CYCLOPS: Things To Do

## Updates to CCMS needing corresponding work in mod-cyclops

### Chunk 1 (DONE)

* **DONE** added create fund and show funds commands
* **DONE** added drop project command
* in alter project
  * **DONE** name values used as identifiers are no longer enclosed in quotation marks
  * **N/A** values of the action property are now restricted to preset names (listed in the documentation)
  * **DONE** property locations has been superseded by new properties origins and destinations, and will be removed in the future
  * **DONE** added the action drop all
  * **N/A** an error is no longer returned when adding a subvalue to a composite property that already contains the subvalue
* all documentation updated
* continuing work on spectres and authorization

The special `reserve` set is being superseded by project-specific sets named _project_.object, for example `palci_slavic.object` for the project `palci_slavic`.  These new `object` sets are like `reserve` but with the addition of spectre attributes.  They are created automatically by `create project`.

In Mod-Cyclops, for the moment it would be sufficient to replace `reserve` in the from clause with `palci_slavic.object`.


### Unit tests (DONE)

* **DONE** Unit tests for handlers
* **DONE** Unit tests for server/router


### Chunk 2 (DONE)

* **DONE** The `reserve` set has been superseded by the `object` set in each project.
* **DONE** Added the `update` command for changing `object` attributes.
* **DONE** Added the `show sets in project` command.
* **DONE** The project property `locations` has been removed.
* **DONE** Added the `archive project` command, to be used instead of `drop project`.
* **N/A** The `drop project` command now only drops archived projects.
* **N/A** Added the `show projects archived` command.

There is a `create property fund` command implemented but it isn't completely enabled yet.

The main change above is the `update` command which allows updating spectre attributes (`fund` for the moment).


### For me

* **DONE** Protect most generated SQL-like commands from injection
* **DONE** JSON Schemas and examples need to be brought into alignment with reality.
* **DONE** Update project should not delete all funds then re-add those included, but generate and execute diffs
* mod-cyclops should handle "id:name" strings consistently
* Protect templates in generated SQL-like commands from injection
* Protect conditions in generated SQL-like commands from injection


### Chunk 3

* The “create filter” and “show filters” commands and the “filter()” operator are available in test/demo.  See documentation for more details.


## Updates needed in CCMS

* `drop fund`
* Ability to set human-readable name of fund
* `show tracks`, `create track`, `delete track`
* `show people`, `create person`, `delete person` -- defined globally or per project?

