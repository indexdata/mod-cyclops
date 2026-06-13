# CYCLOPS: Things To Do

## Updates to CCMS needing corresponding work in mod-cyclops

* **DONE** added create fund and show funds commands
* **DONE** added drop project command
* in alter project
  * **DONE** name values used as identifiers are no longer enclosed in quotation marks
  * **N/A** values of the action property are now restricted to preset names (listed in the documentation)
  * property locations has been superseded by new properties origins and destinations, and will be removed in the future
  * **DONE** added the action drop all
  * **N/A** an error is no longer returned when adding a subvalue to a composite property that already contains the subvalue
* all documentation updated
* continuing work on spectres and authorization

The special `reserve` set is being superseded by project-specific sets named _project_.object, for example `palci_slavic.object` for the project `palci_slavic`.  These new `object` sets are like `reserve` but with the addition of spectre attributes.  They are created automatically by `create project`.

In Mod-Cyclops, for the moment it would be sufficient to replace `reserve` in the from clause with `palci_slavic.object`.


## Updates needed in CCMS

* `drop fund`
* Ability to set human-readable name of fund
* `show tracks`, `create track`, `delete track`
* `show people`, `create person`, `delete person` -- defined globally or per project?

