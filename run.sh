#!/bin/bash
set -e
set -x

##### WARNING!!!
#
# THIS IS A HELPER SCRIPT FOR DEVELOPMENT.
# IT IS USED TO RUN KONTROL, PROXY AND EXAMPLE KITE.
#

##### MANUAL STEPS
#
# 1. make sure that your gopath is set correctly (kite-repo:koding-repo)
# 2. cd into kite repo
#

# kill all running processes
killall main        || true
killall math-register || true

# delete existing kite.key
rm -rf $HOME/.kite

# delete existing kontrol data
rm -rf /tmp/kontrol-data

# generate rsa keys
openssl genrsa -out /tmp/privateKey.pem 2048
openssl rsa -in /tmp/privateKey.pem -pubout > /tmp/publicKey.pem

# initialize machine with new kite.key
go run kontrol/kontrol/main.go -public-key /tmp/publicKey.pem -private-key /tmp/privateKey.pem -init -username devrim -kontrol-url "ws://localhost:4000"

# run essential kites
go run kontrol/kontrol/main.go -public-key /tmp/publicKey.pem -private-key /tmp/privateKey.pem -data-dir /tmp/kontrol-data &
# go run proxy/proxy/main.go     -public-key /tmp/publicKey.pem -private-key /tmp/privateKey.pem &

# run simple math kite
go run examples/math-register/math-register.go
