# mod-cyclops

Copyright (C) 2025 Index Data.

This software is distributed under the terms of the GNU Affero General Public License version 3. See the file "[LICENSE](LICENSE)" for more information.


## Overview

This is a [FOLIO](https://folio.org/) module providing access to CCD, the server of [the CCMS software](https://pkg.go.dev/github.com/indexdata/ccms) that powers CYCLOPS. It exists so that we can create a FOLIO-based UI for CYCLOPS. It presents a somewhat RESTful WSAPI, in contrast with the command-based API of the underlying software, largely so that FOLIO permissions can be applied to the various endpoints.


## Building

Run `make` in the top-level directory to generate the binary, `mod-cyclops`, and the various descriptors required to run it as a FOLIO module under Okapi. Both the binary and the generated descriptors are placed in the `target` directory. `make lint` will run a selection of code-quality tools over the source code, and `make clean` will remove all generated fields. There are presently no unit tests.


## Invocation

`mod-cyclops` takes no command-line arguments and has no configuration file. It is configured entirely by environment variables:

* Configuring the service provided:
  * `SERVER_HOST`, `SERVER_PORT` -- the host and port on which `mod-cyclops` listens. Host defaults to `0.0.0.0`, which listens on all interfaces; port defaults to 12370.
  * `MOD_CYCLOPS_QUERY_TIMEOUT` -- The number of seconds that the server will hold open a client connection before timing out a long-running request. Defaults to 60 seconds.
* Configuring the CCMS service used:
  * `CCMS_HOST` -- The hostname of a CCMS service to connect to. Must be defined.
  * `CCMS_PORT` -- The port number to connect to on that host. Defaults to 8504.
  * `CCMS_USER`, `CCMS_PASSWORD` -- The name of the user to authenticate as on the CCMS service and the corresponding password.
* Configuring logging (see [`catlogger`](https://pkg.go.dev/github.com/MikeTaylor/catlogger) for overview):
  * `LOGGING_CATEGORIES` or `LOGCAT` -- A comma-separated list of categories in which messages will be logged: see below for available categories.
  * `LOGGING_PREFIX` -- If specified, a string that will be included at the start of each logging line. Can be helpful for disambiguating logging output from other kinds of output.
  * `LOGGING_TIMESTAMP` -- If specified, then each line of logging contains a timestamp.
* Configuring dummy data:
  * `MOD_CYCLOPS_DUMMY_DATA` -- If specified, then the CCMS server is never contacted, but dummy responses are provided to each WSAPI call. This facility is provided to enable UI development to begin before CCMS is functional, and **may be removed in future**.

Logging messages are emitted under the following categories, if those categories are included in the comma-separated list in the `LOGGING_CATEGORIES` or `LOGCAT` environment variable:

* `listen` -- Log when the server has been started up and begun to listen, noting the listening host and port.
* `path` -- Log each incoming request with its method (GET, POST, etc.) and path.
* `command` -- Log each complex generated CCMS command, e.g. Retrieve commands.
* `error` -- Log errors that are returned to the client.

So, for example, if the server is run as `env LOGCAT=listen,error target/mod-cyclops`, then a message will be emitted at startup, when the server has begun to listen, but it will otherwise remain quiet apart from logging any errors that might occur.


## API

[API documentation](https://s3.amazonaws.com/indexdata-docs/api/mod-cyclops/cyclops.html)
is automatically generated from
[the RAML and JSON Schemas](ramls).


## Author

Mike Taylor, [Index Data ApS](https://www.indexdata.com/).
mike@indexdata.com


