#!/bin/sh
set -e

go install ./cmd/kite
go install ./regserv/regserv
go install ./kontrol/kontrol
go install ./proxy/proxy
