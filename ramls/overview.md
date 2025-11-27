This WSAPI provides a RESTish and FOLIO-idiomatic way of invoking the operations to be supported by
[the CCMS server](https://github.com/indexdata/ccms),
the data-management middleware at the heart of the CYCLOPS system.
Based on [section 2.3 (Commands)](https://d1f3dtrg62pav.cloudfront.net/ccms/#_commands) of the [CCMS Documentation](https://d1f3dtrg62pav.cloudfront.net/ccms/#_commands), the following commands will be supported, and will act on the specified kinds of object.

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

This gives us three kinds of object that we need to represent RESTfully, and one more implied object representing the set of tags associated with a set:

* tags (2 operations) at `/cyclops/tags`
* filters (2 operations) at `/cyclops/filters`
* sets (7 operations) at `/cyclops/sets` and individual sets at `/cyclops/sets/{setName}`
    * tags applied to a set at `/cyclops/sets/{setName}/tag/{tagName}`

The last of these paths is the most conceptually complex. The path `/cyclops/sets/mike/tag/dino` represents the resource "the subset of records within the set `mike` that are tagged with `dino`". POSTing to that resource can add or remove records to the subset by associating the tag with records in the wider set. (The path for this resource includes the currently redundant component `/tag` in case we need to extend the set API later on to address other kinds of object associated with sets.)

*Implementation note.*
Operations on tags do not rely on any other kind of object. Operations on filters rely only on other filters. So the dependency graph is simple: we need to implement both tags and filters (in either order) before we can fully implement sets.
