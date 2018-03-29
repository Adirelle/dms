dms
===

Adirelle/dms is a fork of [anacrolix/dms](https://github.com/anacrolix/dms). This is a UPNP A/V server written in [Go](https://golang.org/).

dms delegates media probing to ffprobe/ffmpeg. However, this is optional: the features that depends on them are disabled if dms cannot find them.

Features
--------

* Implements UPNP's ContentDirectory service:
	* Reproduce the directory tree.
	* No initial scan is necessary.
	* Looks for Album Art.
* Implements SSDP (announcing/querying).
* Provides a read-only RESTful API, supporting HTML, XML et JSON formats.

TODOs
-----

* Automated tests.
* Reimplements transcoding (using ffmpeg).
* Reimplements DLNA ranges (using ffmpeg too).
