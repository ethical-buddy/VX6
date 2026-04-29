.PHONY: build clean install test

VERSION ?= 1.0.0
PREFIX ?= /usr
BINDIR ?= $(PREFIX)/bin

build: ebpf
	go build -ldflags "-X main.Version=$(VERSION)" -o vx6 ./cmd/vx6

ebpf:
	clang -O2 -target bpf -c internal/ebpf/onion_relay.c -o internal/onion/onion_relay.o

clean:
	rm -f vx6 internal/onion/onion_relay.o

install: build
	install -Dm755 vx6 $(DESTDIR)$(BINDIR)/vx6
	install -Dm644 deployments/systemd/vx6.service $(DESTDIR)$(PREFIX)/lib/systemd/user/vx6.service

test:
	go test ./...
