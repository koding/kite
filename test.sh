#!/bin/sh
set -e

export CONFIG=vagrant

go test -v ./dnode
go test -v ./dnode/rpc
go test -v ./cmd/cli
go test -v ./systeminfo
go test -v
go test -v ./pool
go test -v ./kontrol
