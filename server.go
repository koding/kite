// Package server implements a HTTP(S) server for kites.
package kite

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/rcrowley/goagain"
)

// Run is a blocking method. It runs the kite server and then accepts requests
// asynchronously. It supports graceful restart via SIGUSR2.
func (k *Kite) Run() {
	if os.Getenv("KITE_VERSION") != "" {
		fmt.Println(k.Kite().Version)
		os.Exit(0)
	}

	// An error string equivalent to net.errClosing for using with http.Serve()
	// during a graceful exit. Needed to declare here again because it is not
	// exported by "net" package.
	const errClosing = "use of closed network connection"

	err := k.listenAndServe()
	if err != nil {
		if strings.Contains(err.Error(), errClosing) {
			// The server is closed by Close() method
			k.Log.Info("Kite server is closed.")
			return
		}
		k.Log.Fatal(err.Error())
	}
}

// Close stops the server and the kontrol client instance.
func (k *Kite) Close() {
	k.Log.Info("Closing kite...")

	if k.kontrol != nil {
		k.kontrol.Close()
	}

	if k.listener != nil {
		k.listener.Close()
	}

}

func (k *Kite) Addr() string {
	return net.JoinHostPort(k.Config.IP, strconv.Itoa(k.Config.Port))
}

// listenAndServe listens on the TCP network address k.URL.Host and then
// calls Serve to handle requests on incoming connectionk.
func (k *Kite) listenAndServe() error {
	var err error

	// inerhit a net.Listener from the parent process
	k.listener, err = goagain.Listener()
	if err != nil {
		// create a new one if there doesn't exist
		k.listener, err = net.Listen("tcp4", k.Addr())
		if err != nil {
			return err
		}

		k.Log.Info("New listening: %s", k.listener.Addr().String())

		if k.TLSConfig != nil {
			if k.TLSConfig.NextProtos == nil {
				k.TLSConfig.NextProtos = []string{"http/1.1"}
			}
			k.listener = tls.NewListener(k.listener, k.TLSConfig)
		}
	} else {
		k.Log.Info("Resuming listening on: %s", k.listener.Addr().String())

		// Kill the parent, now that the child has started successfully.
		if err := goagain.Kill(); nil != err {
			k.Log.Fatal(err.Error())
		}
	}

	// listener is ready, notify waiters.
	close(k.readyC)

	go func() {
		defer close(k.closeC) // serving is finished, notify waiters.
		k.Log.Info("Serving...")
		http.Serve(k.listener, k)
	}()

	// Block the main goroutine awaiting signals. For a graceful restart we neeed SIGUSR2.
	if _, err := goagain.Wait(k.listener); nil != err {
		return err
	}

	return nil
}

func (k *Kite) UseTLS(certPEM, keyPEM string) {
	if k.TLSConfig == nil {
		k.TLSConfig = &tls.Config{}
	}

	cert, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		panic(err)
	}

	k.TLSConfig.Certificates = append(k.TLSConfig.Certificates, cert)
}

func (k *Kite) UseTLSFile(certFile, keyFile string) {
	certData, err := ioutil.ReadFile(certFile)
	if err != nil {
		k.Log.Fatal("Cannot read certificate file: %s", err.Error())
	}

	keyData, err := ioutil.ReadFile(keyFile)
	if err != nil {
		k.Log.Fatal("Cannot read certificate file: %s", err.Error())
	}

	k.UseTLS(string(certData), string(keyData))
}

func (k *Kite) ServerCloseNotify() chan bool {
	return k.closeC
}

func (k *Kite) ServerReadyNotify() chan bool {
	return k.readyC
}
