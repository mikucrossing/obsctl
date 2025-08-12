GO ?= go
PKG := ./cmd/obsctl
DIST := dist

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: all clean dist build windows linux macos macos-universal build-all test

all: build

dist:
	mkdir -p $(DIST)

build:
	$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o obsctl $(PKG)

clean:
	rm -rf $(DIST) obsctl

windows: dist
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(DIST)/obsctl_windows_amd64.exe $(PKG)
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(DIST)/obsctl_windows_arm64.exe $(PKG)

linux: dist
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(DIST)/obsctl_linux_amd64 $(PKG)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(DIST)/obsctl_linux_arm64 $(PKG)

macos: dist
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(DIST)/obsctl_darwin_amd64 $(PKG)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(DIST)/obsctl_darwin_arm64 $(PKG)

macos-universal: macos
	@if command -v lipo >/dev/null 2>&1; then \
		echo "Creating macOS universal binary..."; \
		lipo -create -output $(DIST)/obsctl_darwin_universal $(DIST)/obsctl_darwin_amd64 $(DIST)/obsctl_darwin_arm64; \
		lipo -info $(DIST)/obsctl_darwin_universal; \
	else \
		echo "lipo not found; run on macOS with Xcode tools to create universal binary."; \
	fi

build-all: windows linux macos macos-universal

test:
	$(GO) test ./...
