.PHONY: build build-ebpf clean install test ebpf

VERSION ?= 1.0.0
PREFIX ?= /usr
BINDIR ?= $(PREFIX)/bin
CLANG ?= clang
GO ?= go
GOCACHE ?= /tmp/vx6-go-build
GOMODCACHE ?= /tmp/vx6-go-mod
EBPF_SRC := internal/ebpf/onion_relay.c
EBPF_OBJ := internal/onion/onion_relay.o
VX6_BIN := vx6
VX6_GUI_BIN := vx6-gui
VX6_BROWSER_BIN := vx6-browser

build:
	@set -e; \
	GO_BIN=""; \
	for candidate in "$(GO)" go /usr/local/go/bin/go /usr/bin/go; do \
		[ -z "$$candidate" ] && continue; \
		if command -v "$$candidate" >/dev/null 2>&1; then \
			GO_BIN="$$(command -v "$$candidate")"; \
			break; \
		fi; \
		if [ -x "$$candidate" ]; then \
			GO_BIN="$$candidate"; \
			break; \
		fi; \
	done; \
	if [ -z "$$GO_BIN" ]; then \
		echo "go toolchain not found"; \
		echo "try one of these:"; \
		echo "  export PATH=\$$PATH:/usr/local/go/bin"; \
		echo "  /usr/local/go/bin/go build -o ./vx6 ./cmd/vx6"; \
		echo "  /usr/bin/go build -o ./vx6 ./cmd/vx6"; \
		echo "then run:"; \
		echo "  sudo make install"; \
		exit 1; \
	fi; \
	mkdir -p "$(GOCACHE)" "$(GOMODCACHE)"; \
	echo "building vx6 with $$GO_BIN"; \
	GOCACHE="$(GOCACHE)" GOMODCACHE="$(GOMODCACHE)" "$$GO_BIN" build -ldflags "-X main.Version=$(VERSION)" -o $(VX6_BIN) ./cmd/vx6; \
	echo "building vx6-gui with $$GO_BIN"; \
	GOCACHE="$(GOCACHE)" GOMODCACHE="$(GOMODCACHE)" "$$GO_BIN" build -ldflags "-X main.Version=$(VERSION)" -o $(VX6_GUI_BIN) ./cmd/vx6-gui; \
	echo "building vx6-browser with $$GO_BIN"; \
	GOCACHE="$(GOCACHE)" GOMODCACHE="$(GOMODCACHE)" "$$GO_BIN" build -ldflags "-X main.Version=$(VERSION)" -o $(VX6_BROWSER_BIN) ./cmd/vx6-gui

build-ebpf: ebpf build

ebpf:
	@CLANG_BIN=""; \
	for candidate in "$(CLANG)" clang /usr/bin/clang /usr/local/swift/usr/bin/clang; do \
		[ -z "$$candidate" ] && continue; \
		if command -v "$$candidate" >/dev/null 2>&1; then \
			CLANG_BIN="$$(command -v "$$candidate")"; \
			break; \
		fi; \
		if [ -x "$$candidate" ]; then \
			CLANG_BIN="$$candidate"; \
			break; \
		fi; \
	done; \
	if [ -z "$$CLANG_BIN" ]; then \
		if [ -f "$(EBPF_OBJ)" ]; then \
			echo "clang not found; using bundled $(EBPF_OBJ)"; \
			echo "to rebuild the eBPF object, install clang/llvm first"; \
			exit 0; \
		fi; \
		echo "clang not found and $(EBPF_OBJ) is missing"; \
		echo "install one of these packages, then rerun 'make build-ebpf':"; \
		echo "  Debian/Ubuntu: sudo apt install clang llvm"; \
		echo "  Fedora:        sudo dnf install clang llvm"; \
		echo "  Arch:          sudo pacman -S clang llvm"; \
		exit 1; \
	fi; \
	if [ ! -f /usr/include/linux/bpf.h ]; then \
		if [ -f "$(EBPF_OBJ)" ]; then \
			echo "linux eBPF headers not found; using bundled $(EBPF_OBJ)"; \
			echo "to rebuild locally, install Linux userspace headers"; \
			exit 0; \
		fi; \
		echo "linux eBPF headers not found"; \
		echo "install one of these packages, then rerun 'make build-ebpf':"; \
		echo "  Debian/Ubuntu: sudo apt install linux-libc-dev"; \
		echo "  Fedora:        sudo dnf install kernel-headers"; \
		echo "  Arch:          sudo pacman -S linux-headers"; \
		exit 1; \
	fi; \
	if [ ! -f /usr/include/asm/types.h ] && [ ! -f /usr/include/x86_64-linux-gnu/asm/types.h ] && [ ! -f /usr/include/aarch64-linux-gnu/asm/types.h ] && [ ! -f /usr/include/arm-linux-gnueabihf/asm/types.h ]; then \
		if [ -f "$(EBPF_OBJ)" ]; then \
			echo "asm/types.h not found; using bundled $(EBPF_OBJ)"; \
			echo "to rebuild locally, install Linux userspace headers"; \
			exit 0; \
		fi; \
		echo "asm/types.h not found"; \
		echo "install one of these packages, then rerun 'make build-ebpf':"; \
		echo "  Debian/Ubuntu: sudo apt install linux-libc-dev"; \
		echo "  Fedora:        sudo dnf install kernel-headers"; \
		echo "  Arch:          sudo pacman -S linux-headers"; \
		exit 1; \
	fi; \
	echo "building eBPF object with $$CLANG_BIN"; \
	TMP_OBJ="$(EBPF_OBJ).tmp"; \
	rm -f "$$TMP_OBJ"; \
	if "$$CLANG_BIN" -O2 -target bpf -c "$(EBPF_SRC)" -o "$$TMP_OBJ"; then \
		mv "$$TMP_OBJ" "$(EBPF_OBJ)"; \
	else \
		rm -f "$$TMP_OBJ"; \
		if [ -f "$(EBPF_OBJ)" ]; then \
			echo "eBPF rebuild failed; keeping bundled $(EBPF_OBJ)"; \
			echo "common fixes:"; \
			echo "  Debian/Ubuntu: sudo apt install clang llvm linux-libc-dev"; \
			echo "  Fedora:        sudo dnf install clang llvm kernel-headers"; \
			echo "  Arch:          sudo pacman -S clang llvm linux-headers"; \
			exit 0; \
		fi; \
		echo "eBPF rebuild failed"; \
		echo "common fixes:"; \
		echo "  Debian/Ubuntu: sudo apt install clang llvm linux-libc-dev"; \
		echo "  Fedora:        sudo dnf install clang llvm kernel-headers"; \
		echo "  Arch:          sudo pacman -S clang llvm linux-headers"; \
		exit 1; \
	fi

clean:
	rm -f $(VX6_BIN) $(VX6_GUI_BIN) $(VX6_BROWSER_BIN)

install:
	@set -e; \
	if [ -f $(VX6_BIN) ] && [ -f $(VX6_GUI_BIN) ] && [ -f $(VX6_BROWSER_BIN) ]; then \
		echo "installing existing ./$(VX6_BIN), ./$(VX6_GUI_BIN), and ./$(VX6_BROWSER_BIN)"; \
	else \
		echo "release binaries not found in the current directory"; \
		echo "trying to build them before install"; \
		GO_BIN=""; \
		for candidate in "$(GO)" go /usr/local/go/bin/go /usr/bin/go; do \
			[ -z "$$candidate" ] && continue; \
			if command -v "$$candidate" >/dev/null 2>&1; then \
				GO_BIN="$$(command -v "$$candidate")"; \
				break; \
			fi; \
			if [ -x "$$candidate" ]; then \
				GO_BIN="$$candidate"; \
				break; \
			fi; \
		done; \
		if [ -z "$$GO_BIN" ]; then \
			echo "go toolchain not found"; \
			echo "if you already built VX6, place the executables at ./$(VX6_BIN), ./$(VX6_GUI_BIN), and ./$(VX6_BROWSER_BIN) and rerun:"; \
			echo "  sudo make install"; \
			echo "otherwise build with:"; \
			echo "  make build"; \
			echo "or:"; \
			echo "  /usr/local/go/bin/go build -o ./$(VX6_BIN) ./cmd/vx6"; \
			echo "  /usr/local/go/bin/go build -o ./$(VX6_GUI_BIN) ./cmd/vx6-gui"; \
			echo "  /usr/local/go/bin/go build -o ./$(VX6_BROWSER_BIN) ./cmd/vx6-gui"; \
			exit 1; \
		fi; \
		mkdir -p "$(GOCACHE)" "$(GOMODCACHE)"; \
		echo "building vx6 with $$GO_BIN before install"; \
		GOCACHE="$(GOCACHE)" GOMODCACHE="$(GOMODCACHE)" "$$GO_BIN" build -ldflags "-X main.Version=$(VERSION)" -o $(VX6_BIN) ./cmd/vx6; \
		echo "building vx6-gui with $$GO_BIN before install"; \
		GOCACHE="$(GOCACHE)" GOMODCACHE="$(GOMODCACHE)" "$$GO_BIN" build -ldflags "-X main.Version=$(VERSION)" -o $(VX6_GUI_BIN) ./cmd/vx6-gui; \
		echo "building vx6-browser with $$GO_BIN before install"; \
		GOCACHE="$(GOCACHE)" GOMODCACHE="$(GOMODCACHE)" "$$GO_BIN" build -ldflags "-X main.Version=$(VERSION)" -o $(VX6_BROWSER_BIN) ./cmd/vx6-gui; \
	fi
	install -Dm755 $(VX6_BIN) $(DESTDIR)$(BINDIR)/vx6
	install -Dm755 $(VX6_GUI_BIN) $(DESTDIR)$(BINDIR)/vx6-gui
	install -Dm755 $(VX6_BROWSER_BIN) $(DESTDIR)$(BINDIR)/vx6-browser
	install -Dm644 deployments/systemd/vx6.service $(DESTDIR)$(PREFIX)/lib/systemd/user/vx6.service

test:
	@set -e; \
	GO_BIN=""; \
	for candidate in "$(GO)" go /usr/local/go/bin/go /usr/bin/go; do \
		[ -z "$$candidate" ] && continue; \
		if command -v "$$candidate" >/dev/null 2>&1; then \
			GO_BIN="$$(command -v "$$candidate")"; \
			break; \
		fi; \
		if [ -x "$$candidate" ]; then \
			GO_BIN="$$candidate"; \
			break; \
		fi; \
	done; \
	if [ -z "$$GO_BIN" ]; then \
		echo "go toolchain not found"; \
		exit 1; \
	fi; \
	mkdir -p "$(GOCACHE)" "$(GOMODCACHE)"; \
	GOCACHE="$(GOCACHE)" GOMODCACHE="$(GOMODCACHE)" "$$GO_BIN" test ./...
