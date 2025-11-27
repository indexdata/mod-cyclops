# Operations to be supported by mod-cyclops WSAPI

Based on [section 2.3 (Commands)](https://d1f3dtrg62pav.cloudfront.net/ccms/#_commands) of the [CCMS Documentation](https://d1f3dtrg62pav.cloudfront.net/ccms/#_commands), the following commands will be supported:

* 2.3.1. `add tag` -- operates on sets (using tags and filters)
* 2.3.2. `create set` -- operates on sets
* 2.3.3. `define filter` -- operates on filters (using other filters)
* 2.3.4. `define tag` -- operates on tags
* 2.3.5. `delete` -- operates on sets (using filters and tags)
* 2.3.6. `help` -- (can be ignored)
* 2.3.7. `insert` -- operates on sets (using other sets, filters and tags)
* 2.3.8. `remove tag` -- operates on sets (using tags and filters)
* 2.3.9. `retrieve` -- operates on sets (using filters and tags)
* 2.3.10. `show filters` -- operates on filters
* 2.3.11. `show sets` -- operates on sets
* 2.3.12. `show tags` -- operates on tags

This gives us three kinds of object:

* tags (2 operations)
* filters (2 operations)
* sets (7 operations)

The operations on tags do not rely on any other kind of object. The operations on filters rely only on other filters. So the dependency graph is simple: we need to implement both tags and filters (in either order) before we can fully implement sets.

The operations are described in [the RAML](../ramls/cyclops.raml).
