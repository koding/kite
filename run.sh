#!/bin/bash
set -e
set -x

##### MANUAL STEPS
#
# 1. make sure that your gopath is set correctly (kite-repo:koding-repo)
# 2. cd into kite repo
#

# kill all running processes
killall main || true

# delete existing kite.key
rm -rf $HOME/.kite

# generate rsa keys
openssl genrsa -out privateKey.pem 2048
openssl rsa -in privateKey.pem -pubout > publicKey.pem

# initialize machine with new kite.key
go run regserv/regserv/main.go -public-key publicKey.pem -private-key privateKey.pem -init -username devrim -kontrol-url "ws://localhost:4000"

# run essential kites
go run kontrol/kontrol/main.go -public-key publicKey.pem -private-key privateKey.pem &
go run proxy/proxy/main.go -public-key publicKey.pem -private-key privateKey.pem &
sleep 5
go run regserv/regserv/main.go -public-key publicKey.pem -private-key privateKey.pem &

# run simple math kite
go run examples/math-simple.go
