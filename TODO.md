# CYCLOPS: Things To Do

## Updates to CCMS needing corresponding work in mod-cyclops

### Chunk 1

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


### Unit tests

* **DONE** Unit tests for handlers
* **DONE** Unit tests for server/router


### Chunk 2

* The `reserve` set has been superseded by the `object` set in each project.
* Added the `update` command for changing `object` attributes.
* Added the `show sets in project` command.
* **DONE** The project property `locations` has been removed.
* Added the `archive project` command, to be used instead of `drop project`.
* The `drop project` command now only drops archived projects.
* Added the `show projects archived` command.

There is a `create property fund` command implemented but it isn't completely enabled yet.

The main change above is the `update` command which allows updating spectre attributes (`fund` for the moment).



## Updates needed in CCMS

* `drop fund`
* Ability to set human-readable name of fund
* `show tracks`, `create track`, `delete track`
* `show people`, `create person`, `delete person` -- defined globally or per project?

