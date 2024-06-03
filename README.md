## Considerations
For this implementation, I chose to download the Firehol GitHub master branch archive and process it all in memory.
An alternative approach would have been to write to a temporary location on the local filesystem, however I was
targeting a containerized deployment with a read-only filesystem. Another option would have been to make this a
configuration parameter.

Other decisions were made to conserve CPU cycles at the expense of memory utilization; preserving backing arrays from
the moment of allocation to prevent operations such as growing and shrinking.

The bad IP list data refresh system was designed for highly concurrent reads, with infrequent writes.

## Features
Utilizes a configuration file in which any number of file name suffixes can be listed, indicating that files having a 
name that matches any listed suffix should comprise the master bad IP address list.

A health-check endpoint is included that monitors the "freshness" of the bad IP list data. If the refresh routine panics
or otherwise stops running beyond the refresh interval, this endpoint will respond with a 400 status code.

When checking for a new version of the GitHub files, a HEAD request is issued first in order to compare the current list
data's version against the current ETag header field value. This prevents unnecessary download requests of the zip
archive. If the ETag value has changed, a GET request is issued to download the archive file.

## Known Missing Features
Authentication and authorization are typically present on any service.

Observability instrumentation is something that is typically given much more thought and attention.
