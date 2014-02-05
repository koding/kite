#!/bin/sh
set -e

export CONFIG=vagrant

go test -v ./dnode
go test -v ./dnode/rpc
go test -v ./kd/cli
go test -v 
