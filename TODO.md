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


### Chunk 3 (DONE)

* **DONE** The “create filter” and “show filters” commands and the “filter()” operator are available in test/demo.  See documentation for more details.


### Chunk 4

CCMS v0.0.29 updated in test instance:
* **DONE** `show filters` now returns project name in a separate column.
* `show sets` now returns project name in a separate column.
* Filters now have a project name space.  https://d1f3dtrg62pav.cloudfront.net/ccms/doc/current/#_create_filter
* Added command `show filters in project`.  https://d1f3dtrg62pav.cloudfront.net/ccms/doc/current/#_show
* Added command `drop filter`.  https://d1f3dtrg62pav.cloudfront.net/ccms/doc/current/#_drop_filter
* Added `cascade` option to the command `drop project`.  https://d1f3dtrg62pav.cloudfront.net/ccms/doc/current/#_drop_project
* **DONE** Added the attribute `holdings_count`.  https://d1f3dtrg62pav.cloudfront.net/ccms/doc/current/#_attributes


### For me

* **DONE** Protect most generated SQL-like commands from injection
* **DONE** JSON Schemas and examples need to be brought into alignment with reality
* **DONE** Update project should not delete all funds then re-add those included, but generate and execute diffs
* **DONE** Return project names as well as IDs when listing projects
* **DONE** Return a structure of { name, title } when listing funds
* **DONE** mod-cyclops should handle "id:name" strings consistently
* **DONE** Add CRUD support for funds
* **DONE** Change project and fund structures so all `{id, name}` pairs use those fieldnames
* **DONE** Change project `altName` field to `id` (discarding old Id)
* **DONE** Support new batch-update WSAPI
* **DONE** Validation function use same rules as CCMS `Validator` object
* **DONE** Review permission names for consistency and Eureka-friendliness
* **DONE** Fix `/cyclops/sets/{setName}/tag/{tagName}` path to use plural `tags`
* **FIXED**: when inserting into a set, limit should be omitted
* **NO** Consider making Retrieve responses into regular JSON records
* **N/A** Support new `holdings_count` attribute
* Consider breaking the `fund` field in retrieve responses into `{ id, name }`
* Protect templates in generated SQL-like commands from injection
* Protect conditions in generated SQL-like commands from injection


## Updates needed in CCMS

* `show tracks`, `create track`, `alter tracks` and delete track
* `show people`, `create person`, `alter person` and `delete person` -- defined globally or per project?

