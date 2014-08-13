NO_COLOR=\033[0m
OK_COLOR=\033[0;32m
KITE_HOME=/tmp/test_kite_home
ULIMIT=9000


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

	@echo "$(OK_COLOR)==> Creating openssl keys $(NO_COLOR)"
	@openssl genrsa -out /tmp/privateKey.pem 2048
	@openssl rsa -in /tmp/privateKey.pem -pubout > /tmp/publicKey.pem

	@echo "$(OK_COLOR)==> Creating test kite key $(NO_COLOR)"
	@`which go` run kontrol/kontrol/main.go -public-key /tmp/publicKey.pem -private-key /tmp/privateKey.pem -init -username kite -kontrol-url "http://localhost:4444/kite"

	@echo "$(OK_COLOR)==> Running Kontrol $(NO_COLOR)"
	@`which go` run kontrol/kontrol/main.go -public-key /tmp/publicKey.pem -private-key /tmp/privateKey.pem -port 4444

install:
	@echo "$(OK_COLOR)==> Downloading dependencies$(NO_COLOR)"
	@`which go` get -d -v ./...
	@`which go` get github.com/fatih/color

	@echo "$(OK_COLOR)==> Installing test binaries $(NO_COLOR)"
	@`which go` install -v ./kitectl
	@`which go` install -v ./kontrol/kontrol
	@`which go` install -v ./reverseproxy/reverseproxy
	@`which go` install -v ./tunnelproxy/tunnelproxy

test:
	@echo "$(OK_COLOR)==> Preparing test environment $(NO_COLOR)"
	@echo "Cleaning $(KITE_HOME) directory"
	@rm -rf $(KITE_HOME)

	@echo "Setting ulimit to $(ULIMIT) for multiple client tests"
	@ulimit -n $(ULIMIT) #needed for multiple kontrol tests

	@echo "Killing previous etcd instance"
	@killall etcd ||:

	@echo "Installing etcd"
	test -d "_etcd" || git clone https://github.com/coreos/etcd _etcd
	@rm -rf _etcd/kontrol_test ||: #remove previous folder
	@cd _etcd; ./build; ./bin/etcd --name=kontrol --data-dir=kontrol_test &

	@echo "Creating test key"
	@`which go` run ./testutil/writekey/main.go

	@echo "$(OK_COLOR)==> Downloading dependencies$(NO_COLOR)"
	@`which go` get -d -v ./...
	@`which go` get github.com/fatih/color

	@echo "$(OK_COLOR)==> Testing packages $(NO_COLOR)"
	@`which go` test -race $(VERBOSE) ./dnode
	@`which go` test -race $(VERBOSE) ./kitectl
	@`which go` test -race $(VERBOSE) ./systeminfo
	@`which go` test -race $(VERBOSE) ./
	@`which go` test -race $(VERBOSE) ./test
	@`which go` test -race $(VERBOSE) ./kontrol
	@`which go` test -race $(VERBOSE) ./tunnelproxy
	@`which go` test -race $(VERBOSE) ./reverseproxy
	@`which go` test -race $(VERBOSE) ./pool

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
