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

ifndef POSTGRES_HOST
	ifdef DOCKER_HOST
		POSTGRES_HOST=$(shell echo $(DOCKER_HOST) | cut -d: -f2 | cut -c 3-)
	else
		POSTGRES_HOST=127.0.0.1
	endif
endif

all: test

postgres:
	docker stop postgres && docker rm postgres || true
	docker run -d -v $(PWD)/postgres.d:/docker-entrypoint-initdb.d --name postgres -p 5432:5432 -P postgres:9.3
	while ! docker logs postgres 2>&1 | grep 'ready for start up' >/dev/null; do sleep 1; done
	psql -h $(POSTGRES_HOST) postgres -f kontrol/001-schema.sql -U postgres
	psql -h $(POSTGRES_HOST) -c 'CREATE DATABASE kontrol owner kontrol;' -U postgres
	psql -h $(POSTGRES_HOST) kontrol -f kontrol/002-table.sql -U postgres
	psql -h $(POSTGRES_HOST) kontrol -f kontrol/003-migration-001-add-kite-key-table.sql -U postgres
	psql -h $(POSTGRES_HOST) kontrol -f kontrol/003-migration-002-add-key-indexes.sql -U postgres
	echo "#!/bin/bash" > .env
	echo "alias psql-kite='psql postgresql://postgres@$(POSTGRES_HOST):5432/kontrol'" >> .env
	echo "export KONTROL_POSTGRES_HOST=$(POSTGRES_HOST)" >> .env
	echo "export KONTROL_STORAGE=postgres" >> .env
	echo "export KONTROL_POSTGRES_USERNAME=kontrolapplication" >> .env
	echo "export KONTROL_POSTGRES_DBNAME=kontrol" >> .env
	echo "export KONTROL_POSTGRES_PASSWORD=somerandompassword" >> .env

postgres-logs:
	docker exec -ti postgres /bin/bash -c 'tail -f /var/lib/postgresql/data/pg_log/*.log'

format:
	@echo "$(OK_COLOR)==> Formatting the code $(NO_COLOR)"
	@gofmt -s -w *.go
	@goimports -w *.go

kontrol:
	@echo "$(OK_COLOR)==> Preparing kontrol test environment $(NO_COLOR)"
	@rm -rf $(KITE_HOME)

	@echo "$(OK_COLOR)==> Creating openssl keys $(NO_COLOR)"
	@openssl genrsa -out /tmp/privateKey.pem 2048
	@openssl rsa -in /tmp/privateKey.pem -pubout -out /tmp/publicKey.pem

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
	test -d "_etcd" || git clone -b release-2.2 https://github.com/coreos/etcd _etcd
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
	test -d "_etcd" || git clone -b release-2.2 https://github.com/coreos/etcd _etcd
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
	@`which go` test -race $(VERBOSE) -p 1 ./...

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
