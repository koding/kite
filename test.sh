#!/bin/sh
set -e

# Do not owerwrite the user's key when running tests
KITE_HOME=/tmp/test_kite_home
rm -rf $KITE_HOME
export KITE_HOME=$KITE_HOME
go run ./testutil/writekey/main.go

go test -v ./dnode
go test -v ./dnode/rpc
go test -v ./cmd/cli
go test -v ./systeminfo
go test -v
go test -v ./server
go test -v ./registration/test
go test -v ./regserv
go test -v ./kontrol
go test -v ./proxy
go test -v ./simple
go test -v ./pool
