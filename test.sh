#!/bin/sh
set -e

# Do not owerwrite the user's key when running tests
KITE_HOME=/tmp/kite_home
rm -rf $KITE_HOME
export KITE_HOME=$KITE_HOME

go test -v ./dnode
go test -v ./dnode/rpc
go test -v ./cmd/cli
go test -v ./systeminfo
go test -v
go test -v ./regserv
go test -v ./kontrol
go test -v ./pool
go test -v ./proxy
