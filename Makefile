.PHONY: build clean install test ebpf build-windows build-windows-arm64 build-perf-test-gui build-perf-test-gui-all

VERSION ?= 1.0.0
PREFIX ?= /usr
BINDIR ?= $(PREFIX)/bin
CLANG ?= clang
EBPF_SRC := internal/ebpf/onion_relay.c
EBPF_OBJ := internal/onion/onion_relay.o

build: ebpf
	go build -ldflags "-X main.Version=$(VERSION)" -o vx6 ./cmd/vx6

# Windows AMD64 build
build-windows:
	GOOS=windows GOARCH=amd64 go build -ldflags "-X main.Version=$(VERSION)" -o vx6-amd64.exe ./cmd/vx6

# Windows ARM64 build
build-windows-arm64:
	GOOS=windows GOARCH=arm64 go build -ldflags "-X main.Version=$(VERSION)" -o vx6-arm64.exe ./cmd/vx6

# Performance test GUI CLI (all platforms)
build-perf-test-gui:
	cd cmd/perf-test-gui && go build -o perf-test-cli ./

# Windows AMD64 + ARM64 builds for performance test GUI
build-perf-test-gui-windows:
	cd cmd/perf-test-gui && \
	GOOS=windows GOARCH=amd64 go build -o perf-test-cli-amd64.exe ./ && \
	GOOS=windows GOARCH=arm64 go build -o perf-test-cli-arm64.exe ./

# Build performance test CLI for all platforms
build-perf-test-gui-all: build-perf-test-gui build-perf-test-gui-windows
	@echo "Performance test CLI built for all platforms"

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
	rm -f vx6 vx6-amd64.exe vx6-arm64.exe $(EBPF_OBJ)
	cd cmd/perf-test-gui && go clean

install: build
	install -Dm755 vx6 $(DESTDIR)$(BINDIR)/vx6
	install -Dm644 deployments/systemd/vx6.service $(DESTDIR)$(PREFIX)/lib/systemd/user/vx6.service

test:
	go test ./...

# Run performance baseline tests
test-perf: build-perf-test-gui
	cd cmd/perf-test-gui && ./perf-test-cli -format json -output ../../../perf-results.json
