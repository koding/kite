Kite Micro-Service Framework
============================

Kite is a framework for developing micro-services in Go.

[![GoDoc](http://img.shields.io/badge/go-documentation-brightgreen.svg?style=flat-square)](https://godoc.org/github.com/koding/kite)
[![Build Status](http://img.shields.io/travis/koding/kite/master.svg?style=flat-square)](https://travis-ci.org/koding/kite)

![Kite](http://i.imgur.com/iNcltPN.png)


Kite is both the name of the framework and the micro-service that is written by
using this framework.  Basically, Kite is a RPC server as well as a client. It
connects to other kites and peers to communicate with each other. They can
discover other kites using a service called Kontrol, and communicate with them 
bidirectionaly. The communication protocol uses a WebSocket (or XHR) as transport 
in order to allow web applications to connect directly to kites.

Kites can talk with each other by sending
[dnode](https://github.com/substack/dnode-protocol/blob/master/doc/protocol.markdown)
messages over a socket session.  If the client knows the URL of the server kite it
can connect to it directly.  If the URL is not known, client can ask for it
from Kontrol (Service Discovery).

For more info checkout the blog post at GopherAcademy which explains Kite in more detail: http://blog.gopheracademy.com/birthday-bash-2014/kite-microservice-library/

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

and use `kite` as the package name inside the code.

What is *Kontrol*?
------------------

Kontrol is the service registry and authentication service used by Kites.  It
is itself a kite too.

When a kite starts to run, it can registers itself to Kontrol with the
`Register()` method if wished.  That enables others to find it by querying
Kontrol. There is also a Proxy Kite for giving public URLs to registered
kites.

Query has 7 fields:

    /<username>/<environment>/<name>/<version>/<region>/<hostname>/<id>

* You must at least give the username.
* The order of the fields is from general to specific.
* Query cannot contains empty parts between fields.

Installing *Kontrol*
------------------

Install Kontrol:

```
go get github.com/koding/kite/kontrol/kontrol
```

Generate keys for the Kite key:

```
openssl genrsa -out key.pem 2048
openssl rsa -in key.pem -pubout > key_pub.pem
```

Set environment variables:

```
KONTROL_PORT=6000
KONTROL_USERNAME="kontrol"
KONTROL_STORAGE="etcd"
KONTROL_KONTROLURL="http://127.0.0.1:6000/kite"
KONTROL_PUBLICKEYFILE="certs/key_pub.pem"
KONTROL_PRIVATEKEYFILE="certs/key.pem"
```

Generate initial Kite key:

```
./bin/kontrol -initial
```

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
* Add your method handlers with `k.HandleFunc()` or `k.Handle()`.
* Call `k.Run()`

Below you can find an example, a math kite which calculates the square of a
received number:

```go
package main

import "github.com/koding/kite"

func main() {
	// Create a kite
	k := kite.New("math", "1.0.0")

	// Add our handler method with the name "square"
	k.HandleFunc("square", func(r *kite.Request) (interface{}, error) {
		a := r.Args.One().MustFloat64()
		result := a * a    // calculate the square
		return result, nil // send back the result
	}).DisableAuthentication()

	// Attach to a server with port 3636 and run it
	k.Config.Port = 3636
	k.Run()
}
```

Now let's connect to it and send a `4` as an argument.

```go
package main

import (
	"fmt"

	"github.com/koding/kite"
)

func main() {
	k := kite.New("exp2", "1.0.0")

	// Connect to our math kite
	mathWorker := k.NewClient("http://localhost:3636/kite")
	mathWorker.Dial()

	response, _ := mathWorker.Tell("square", 4) // call "square" method with argument 4
	fmt.Println("result:", response.MustFloat64())
}
```

Check out the [examples](https://github.com/koding/kite/tree/master/examples)
folder for more examples.
