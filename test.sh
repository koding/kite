#!/bin/sh
set -e

# Do not owerwrite the user's key when running tests
KITE_HOME=/tmp/test_kite_home
rm -rf $KITE_HOME
export KITE_HOME=$KITE_HOME
go run ./testutil/writekey/main.go

while getopts “v” OPTION
do
     case $OPTION in
         v)
             VERBOSE="-v"
             ;;
     esac
done

go test $VERBOSE ./dnode
go test $VERBOSE ./dnode/rpc
go test $VERBOSE ./cmd/cli
go test $VERBOSE ./systeminfo
go test $VERBOSE ./
go test $VERBOSE ./server
go test $VERBOSE ./registration/test
go test $VERBOSE ./regserv
go test $VERBOSE ./kontrol
go test $VERBOSE ./proxy
go test $VERBOSE ./simple
go test $VERBOSE ./pool
