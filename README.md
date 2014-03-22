Kite Micro-Service Framework
============================

Kite is a framework for developing micro-services in Go.

[![GoDoc](https://godoc.org/github.com/koding/kite?status.png)](https://godoc.org/github.com/koding/kite)
[![Build Status](https://travis-ci.org/koding/kite.png)](https://travis-ci.org/koding/kite)

![Kite](http://i.imgur.com/iNcltPN.png)

What is Kite?
-------------

Kite is both the name of the framework and the micro-service that is written by using this framework.
Basicly, it is an RPC server and can do anything you want.
They can discover other kites by using a service called Kontrol and communicate with them.
The communication protocol uses Websocket as transport in order to allow web applications to connect directly to kites.

Kites talk with each other by sending [dnode](https://github.com/substack/dnode-protocol/blob/master/doc/protocol.markdown) messages over Websocket.
If the client knows the URL of the server kite it can connect to it directly.
If the URL is not known, client can ask it to Kontrol.

What is *Kontrol*?
------------------

Kontrol is the service registry and authentication service.
We are planning to seperate these into 2 seperate kites.

When a kite starts to run, it registers itself to Kontrol.
Then, ohters can find it by querying Kontrol.
There is also a Proxy Kite for giving public URLs to registered kites.

Query has 7 fields:

    /<username>/<environment>/<name>/<version>/<region>/<hotname>/<id>

* You must at least give the username.
* The order of the fields is from general to specific.
* Query cannot contains empty parts between fields.

How can I use Kites from Browser?
---------------------------------

Browser also acts like a Kite. It has it's own methods ("log" for logging a
message to the console, "alert" for displaying alert to the user, etc.). A
connected Kite can call methods of the browser.

See [kite.js library](https://github.com/koding/kite.js) for more information.

How can I write a new Kite?
---------------------------

* Import `kite/simple` package.
* Create a new instance with `simple.New()`.
* Add your method handlers with `k.HandleFunc()`.
* Call `k.Run()`

See [an example](https://github.com/koding/kite/blob/master/examples/math_simple/math_simple.go)
for an example Kite code.
