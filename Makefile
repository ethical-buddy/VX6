.PHONY: build clean install test ebpf

VERSION ?= 1.0.0
PREFIX ?= /usr
BINDIR ?= $(PREFIX)/bin
CLANG ?= clang
EBPF_SRC := internal/ebpf/onion_relay.c
EBPF_OBJ := internal/onion/onion_relay.o

build: ebpf
	go build -ldflags "-X main.Version=$(VERSION)" -o vx6 ./cmd/vx6

ebpf:
	@if command -v "$(CLANG)" >/dev/null 2>&1; then \
		echo "building eBPF object with $(CLANG)"; \
		"$(CLANG)" -O2 -target bpf -c "$(EBPF_SRC)" -o "$(EBPF_OBJ)"; \
	elif [ -f "$(EBPF_OBJ)" ]; then \
		echo "clang not found; using bundled $(EBPF_OBJ)"; \
	else \
		echo "clang not found and $(EBPF_OBJ) is missing"; \
		echo "install clang/llvm, then run 'make ebpf' or 'make build' again"; \
		exit 1; \
	fi

clean:
	rm -f vx6 $(EBPF_OBJ)

install: build
	install -Dm755 vx6 $(DESTDIR)$(BINDIR)/vx6
	install -Dm644 deployments/systemd/vx6.service $(DESTDIR)$(PREFIX)/lib/systemd/user/vx6.service

test:
	go test ./...
