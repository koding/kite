NO_COLOR=\033[0m
OK_COLOR=\033[0;32m
KITE_HOME=/tmp/test_kite_home
ULIMIT=9000


DEBUG?=0
ifeq ($(DEBUG), 1)
	VERBOSE="-v"
endif

# Default to etcd
ifndef KONTROL_STORAGE
	KONTROL_STORAGE=etcd
endif

ifndef KITE_TRANSPORT
	KITE_TRANSPORT=WebSocket
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
	@`which go` run kontrol/kontrol/main.go -publickeyfile /tmp/publicKey.pem -privatekeyfile /tmp/privateKey.pem -initial -username kite -kontrolurl "http://localhost:4444/kite"

	@echo "$(OK_COLOR)==> Running Kontrol $(NO_COLOR)"
	@`which go` run kontrol/kontrol/main.go -publickeyfile /tmp/publicKey.pem -privatekeyfile /tmp/privateKey.pem -port 4444 

install:
	@echo "$(OK_COLOR)==> Downloading dependencies$(NO_COLOR)"
	@`which go` get -d -v -t ./...

	@echo "$(OK_COLOR)==> Installing test binaries $(NO_COLOR)"
	@`which go` install -v ./kitectl
	@`which go` install -v ./kontrol/kontrol
	@`which go` install -v ./reverseproxy/reverseproxy
	@`which go` install -v ./tunnelproxy/tunnelproxy

kontroltest:
	@echo "$(OK_COLOR)==> Preparing test environment $(NO_COLOR)"
	@echo "Cleaning $(KITE_HOME) directory"
	@rm -rf $(KITE_HOME)


	@echo "Using as storage: $(KONTROL_STORAGE)"
ifeq ($(KONTROL_STORAGE), "etcd")
	@echo "Killing previous etcd instance"
	@killall etcd ||:

	@echo "Installing etcd"
	test -d "_etcd" || git clone https://github.com/coreos/etcd _etcd
	@rm -rf _etcd/default.etcd ||: #remove previous folder
	@cd _etcd; ./build; ./bin/etcd &
endif

	@echo "Creating test key"
	@`which go` run ./testutil/writekey/main.go

	@echo "$(OK_COLOR)==> Downloading dependencies$(NO_COLOR)"
	@`which go` get -d -v -t ./...

	@echo "$(OK_COLOR)==> Starting kontrol test $(NO_COLOR)"
	@`which go` test -race $(VERBOSE) ./kontrol

test: 
	@echo "$(OK_COLOR)==> Preparing test environment $(NO_COLOR)"
	@echo "Using $(KITE_TRANSPORT) transport"
	@echo "Cleaning $(KITE_HOME) directory"
	@rm -rf $(KITE_HOME)

	@echo "Setting ulimit to $(ULIMIT) for multiple client tests"
	@ulimit -n $(ULIMIT) #needed for multiple kontrol tests

	@echo "$(OK_COLOR)==> Using kontrol storage: '$(KONTROL_STORAGE)'$(NO_COLOR)"


ifeq ($(KONTROL_STORAGE), etcd)
	@echo "Killing previous etcd instance"
	@killall etcd ||:

	@echo "Installing etcd"
	test -d "_etcd" || git clone https://github.com/coreos/etcd _etcd
	@rm -rf _etcd/default.etcd ||: #remove previous folder
	@cd _etcd; ./build; ./bin/etcd &
endif

ifeq ($(KONTROL_STORAGE), postgres)

ifndef KONTROL_POSTGRES_USERNAME
    $(error KONTROL_POSTGRES_USERNAME is not set)
endif

ifndef KONTROL_POSTGRES_DBNAME
    $(error KONTROL_POSTGRES_DBNAME is not set)
endif

endif

	@echo "Creating test key"
	@`which go` run ./testutil/writekey/main.go

	@echo "$(OK_COLOR)==> Downloading dependencies$(NO_COLOR)"
	@`which go` get -d -v -t ./...

	@echo "$(OK_COLOR)==> Testing packages $(NO_COLOR)"
	@`which go` test -race $(VERBOSE) ./dnode
	@`which go` test -race $(VERBOSE) ./kitectl
	@`which go` test -race $(VERBOSE) ./systeminfo
	@`which go` test -race $(VERBOSE) ./
	@`which go` test -race $(VERBOSE) ./test
	@`which go` test -race $(VERBOSE) ./kontrol
	@`which go` test -race $(VERBOSE) ./tunnelproxy
	@`which go` test -race $(VERBOSE) ./reverseproxy

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

.PHONY: all install format test doc vet lint ctags kontrol kontroltest
