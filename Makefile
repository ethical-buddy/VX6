.PHONY: build clean install

VERSION ?= 1.0.0
PREFIX ?= /usr
BINDIR ?= $(PREFIX)/bin

build:
	go build -ldflags "-X main.Version=$(VERSION)" -o vx6 ./cmd/vx6

clean:
	rm -f vx6

install: build
	install -Dm755 vx6 $(DESTDIR)$(BINDIR)/vx6
	install -Dm644 deployments/systemd/vx6.service $(DESTDIR)$(PREFIX)/lib/systemd/user/vx6.service
