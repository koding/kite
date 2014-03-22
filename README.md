Kite Micro-Service Framework
============================

Kite is a framework for developing micro-services in Go.

[![GoDoc](https://godoc.org/github.com/koding/kite?status.png)](https://godoc.org/github.com/koding/kite)
[![Build Status](https://travis-ci.org/koding/kite.png)](https://travis-ci.org/koding/kite)

![Kite](http://i.imgur.com/iNcltPN.png)

What is Kite?
-------------

Kite is both the name of the framework and the micro-service that is written by using this framework.
Basically, it is an RPC server and it can do anything you want.
They can discover other kites using a service called Kontrol, and communicate with them.
The communication protocol uses a websocket as transport in order to allow web applications to connect directly to kites.

Kites can talk with each other by sending [dnode](https://github.com/substack/dnode-protocol/blob/master/doc/protocol.markdown) messages over a websocket.
If the client knows the URL of the server kite it can connect to it directly.
If the URL is not known, client can ask for it from Kontrol.

What is *Kontrol*?
------------------

Kontrol is the service registry and authentication service used by Kites.
It is itself a kite, although we are planning to separate it into two separate kites.

When a kite starts to run, it registers itself to Kontrol.
Then, others can find it by querying Kontrol.
There is also a Proxy Kite for giving public URLs to registered kites.

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

* Import `kite/simple` package.
* Create a new instance with `simple.New()`.
* Add your method handlers with `k.HandleFunc()`.
* Call `k.Run()`

See [an example](https://github.com/koding/kite/blob/master/examples/math_simple/math_simple.go)
for the code of an example kite.
