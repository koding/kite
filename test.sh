#!/bin/sh
set -e

export CONFIG=vagrant

go test -v ./dnode
go test -v ./dnode/rpc
go test -v ./kite
go test -v ./kontrol
go test -v ./token
go test -v ./kodingkey
go test -v ./kd/cli
