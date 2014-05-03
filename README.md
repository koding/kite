Kite Micro-Service Framework
============================

Kite is a framework for developing micro-services in Go.

[![GoDoc](http://img.shields.io/badge/godoc-Reference-brightgreen.svg?style=flat)](https://godoc.org/github.com/koding/kite)
[![Build Status](http://img.shields.io/travis/koding/kite/master.svg?style=flat)](https://travis-ci.org/koding/kite)

![Kite](http://i.imgur.com/iNcltPN.png)


Kite is both the name of the framework and the micro-service that is written by
using this framework.  Basically, Kite is a RPC server as well as a client. It
connects to other kites and peers to communicate with each other. They can
discover other kites using a service called Kontrol, and communicate with them.
The communication protocol uses a websocket as transport in order to allow web
applications to connect directly to kites.

Kites can talk with each other by sending
[dnode](https://github.com/substack/dnode-protocol/blob/master/doc/protocol.markdown)
messages over a websocket.  If the client knows the URL of the server kite it
can connect to it directly.  If the URL is not known, client can ask for it
from Kontrol.

Install and Usage
-----------------

Install the package with:

```bash
go get github.com/koding/kite
```

Import it with:

```go
import "github.com/koding/kite"
```

and use `kite` as the package name inside the code. Check out the
[examples](https://github.com/koding/kite/tree/master/examples) folder for a
simple usage case.


What is *Kontrol*?
------------------

Kontrol is the service registry and authentication service used by Kites.  It
is itself a kite too.

When a kite starts to run, it can registers itself to Kontrol with the
`Register()` method if wished.  That enables others to find it by querying
Kontrol. There is also a Proxy Kite for giving public URLs to registered
kites.

Query has 7 fields:

    /<username>/<environment>/<name>/<version>/<region>/<hotname>/<id>

* You must at least give the username.
* The order of the fields is from general to specific.
* Query cannot contains empty parts between fields.

How can I use kites from a browser?
---------------------------------

A browser can also be a Kite. It has it's own methods ("log" for logging a
message to the console, "alert" for displaying alert to the user, etc.). A
connected kite can call methods defined on the webpage.

See [kite.js library](https://github.com/koding/kite.js) for more information.

How can I write a new kite?
---------------------------

* Import `kite` package.
* Create a new instance with `kite.New()`.
* Add your method handlers with `k.HandleFunc()`.
* Call `k.Run()`

See [an example](https://github.com/koding/kite/blob/master/examples/math/math.go)
for the code of an example kite.
