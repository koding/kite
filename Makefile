NO_COLOR=\033[0m
OK_COLOR=\033[0;32m
KITE_HOME=/tmp/test_kite_home

DEBUG?=0
ifeq ($(DEBUG), 1)
	VERBOSE="-v"
endif

all: test

format:
	@echo "$(OK_COLOR)==> Formatting the code $(NO_COLOR)"
	@gofmt -s -w *.go
	@goimports -w *.go

kontrol:
	@echo "$(OK_COLOR)==> Preparing kontrol test environment $(NO_COLOR)"
	@rm -rf $(KITE_HOME)
	@rm -rf /tmp/kontrol-data

	@echo "$(OK_COLOR)==> Creating openssl keys $(NO_COLOR)"
	@openssl genrsa -out /tmp/privateKey.pem 2048
	@openssl rsa -in /tmp/privateKey.pem -pubout > /tmp/publicKey.pem

	@echo "$(OK_COLOR)==> Creating test kite key $(NO_COLOR)"
	@`which go` run kontrol/kontrol/main.go -public-key /tmp/publicKey.pem -private-key /tmp/privateKey.pem -init -username kite -kontrol-url "http://localhost:4444/kite"

	@echo "$(OK_COLOR)==> Running Kontrol $(NO_COLOR)"
	@`which go` run kontrol/kontrol/main.go -public-key /tmp/publicKey.pem -private-key /tmp/privateKey.pem -data-dir /tmp/kontrol-data -port 4444

install:
	@echo "$(OK_COLOR)==> Installing test binaries $(NO_COLOR)"
	@`which go` install -v ./cmd/kite
	@`which go` install -v ./kontrol/kontrol
	@`which go` install -v ./proxy/proxy

test:
	@echo "$(OK_COLOR)==> Preparing test environment $(NO_COLOR)"
	@echo "Cleaning $(KITE_HOME) directory"
	@rm -rf $(KITE_HOME)

	@echo "Creating test key"
	@`which go` run ./testutil/writekey/main.go

	@echo "$(OK_COLOR)==> Building packages $(NO_COLOR)"
	@`which go` build -v ./...

	@echo "$(OK_COLOR)==> Testing packages $(NO_COLOR)"
	@`which go` test $(VERBOSE) ./dnode
	@`which go` test $(VERBOSE) ./cmd/cli
	@`which go` test $(VERBOSE) ./systeminfo
	@`which go` test $(VERBOSE) ./
	@`which go` test $(VERBOSE) ./test
	@`which go` test $(VERBOSE) ./kontrol
	@`which go` test $(VERBOSE) ./tunnelproxy
	@`which go` test $(VERBOSE) ./reverseproxy
	@`which go` test $(VERBOSE) ./pool

doc:
	@`which godoc` github.com/koding/kite | less

vet:
	@echo "$(OK_COLOR)==> Running go vet $(NO_COLOR)"
	@`which go` vet .

lint:
	@echo "$(OK_COLOR)==> Running golint $(NO_COLOR)"
	@`which golint` .

ctags:
	@ctags -R --languages=c,go

.PHONY: all install format test doc vet lint ctags kontrol
