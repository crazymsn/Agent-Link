# The recipes (clean's rm -rf too) name paths relative to the working
# folder, so they run only in Agent Link's own, known by its go.mod: run from
# another one (make -f path/to/Makefile) they would act there. make -C
# works. go.mod is read relative to the working folder, so a path with
# spaces (or a symlinked Makefile) changes nothing.
ifneq ($(shell sed -n 's/^module //p' go.mod 2>/dev/null),github.com/crazymsn/Agent-Link)
  $(error run make in Agent Link's folder (make -C path/to/agent-link), not in $(CURDIR))
endif

# A tag may hold $, ( or `, which the shell lines below would run: the
# version keeps only what a version is spelled with.
VERSION ?= $(or $(shell git describe --tags --always --dirty 2>/dev/null | tr -cd 'A-Za-z0-9._+-'),dev)
LDFLAGS  = -s -w -X main.version=$(VERSION)
TAGS     = production
TARGETS  = darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64 windows/arm64 android/arm64

# The GUI links the platform webview through cgo, so it is built natively.
# `nogui` builds the terminal-only Agent Link, which cross-compiles anywhere.
# macOS 12 is the oldest Go (1.25 and later) runs on; Info.plist says so too.
ifeq ($(shell uname -s),Darwin)
  export CGO_CFLAGS  = -mmacosx-version-min=12.0 $(MACOS_ARCH_FLAGS)
  export CGO_CXXFLAGS = $(MACOS_ARCH_FLAGS)
  export CGO_LDFLAGS = -mmacosx-version-min=12.0 $(MACOS_ARCH_FLAGS)
endif
# On Linux, GTK 3 and WebKitGTK 4.1: older distributions have them, where
# Wails' default GTK 4 and WebKitGTK 6 would leave them out.
ifeq ($(shell uname -s),Linux)
  TAGS += gtk3
endif
# Termux uses Android's libc and has no desktop webview. Keep the web UI
# and terminal commands, without linking GTK or Wails.
ifeq ($(shell go env GOOS),android)
  TAGS = nogui
endif

.PHONY: build cli install test test-ui app icons release release-cli release-windows release-linux clean dev dev-once

build:
	go build -tags "$(TAGS)" -trimpath -ldflags="$(LDFLAGS)" -o agent-link .

cli:
	CGO_ENABLED=0 go build -tags nogui -trimpath -ldflags="$(LDFLAGS)" -o agent-link .

install:
	go install -tags "$(TAGS)" -trimpath -ldflags="$(LDFLAGS)" .

test:
	go vet -tags "$(TAGS)" ./... && go test -tags "$(TAGS)" ./...

# The browser regressions under internal/gui/tests: the real pages in
# Playwright's Chromium and WebKit; see internal/gui/tests/README.md.
UI_TESTS = $(wildcard internal/gui/tests/*.test.cjs)
# Files are independent and run in parallel; 1 diagnoses a flaky one.
UI_TEST_CONCURRENCY ?= 2

test-ui:
	@test -n "$(UI_TESTS)" || { echo "no GUI tests found under internal/gui/tests" >&2; exit 1; }
	node --test --test-concurrency=$(UI_TEST_CONCURRENCY) $(UI_TESTS)

# macOS bundle: menu bar app with no Dock icon (LSUIElement).
app: build
	@rm -rf agent-link.app
	@mkdir -p agent-link.app/Contents/MacOS agent-link.app/Contents/Resources
	@cp agent-link agent-link.app/Contents/MacOS/agent-link
	@cp build/darwin/agent-link.icns agent-link.app/Contents/Resources/agent-link.icns
	@sed 's/@VERSION@/$(VERSION)/' build/darwin/Info.plist > agent-link.app/Contents/Info.plist
	@xattr -cr agent-link.app 2>/dev/null || true
	@codesign --force --deep --sign - agent-link.app
	@codesign --verify --deep --strict agent-link.app
	@echo "  agent-link.app"

icons:
	@go run build/icon/gen.go tray internal/gui/tray.png
	@go run build/icon/gen.go tray-flap internal/gui/trayflap
	@go run build/icon/gen.go app 64 internal/gui/agent-link-icon-64.png
	@go run build/icon/gen.go app 1024 internal/gui/agent-link-icon-1024.png
	@go run build/icon/gen.go app 256 internal/gui/agent-link-icon-256.png
	@rm -rf build/darwin/agent-link.iconset && mkdir -p build/darwin/agent-link.iconset
	@for s in 16 32 128 256 512; do \
		go run build/icon/gen.go app $$s build/darwin/agent-link.iconset/icon_$${s}x$${s}.png; \
		go run build/icon/gen.go app $$((s*2)) build/darwin/agent-link.iconset/icon_$${s}x$${s}@2x.png; \
	done
	@iconutil -c icns build/darwin/agent-link.iconset -o build/darwin/agent-link.icns && rm -rf build/darwin/agent-link.iconset
	@# macOS 26 draws the Dock icon from AppIcon.icon, light and dark (Xcode 26's actool);
	@# the compiled Assets.car is kept in the tree so a release needn't have it
	@go run build/icon/gen.go glyph 1024 build/darwin/AppIcon.icon/Assets/agent-link.png
	@rm -rf build/darwin/actool && mkdir -p build/darwin/actool
	@xcrun actool build/darwin/AppIcon.icon --compile build/darwin/actool --platform macosx \
		--minimum-deployment-target 11.0 --app-icon AppIcon --output-partial-info-plist build/darwin/actool/partial.plist >/dev/null
	@if test -f build/darwin/actool/Assets.car; then mv build/darwin/actool/Assets.car build/darwin/Assets.car; fi; rm -rf build/darwin/actool

release: clean build
	@mkdir -p dist
	@cp agent-link dist/agent-link-$(shell go env GOOS)-$(shell go env GOARCH)
	@$(MAKE) --no-print-directory release-cli

release-cli:
	@mkdir -p dist
	@for t in $(TARGETS); do \
		os=$${t%/*}; arch=$${t#*/}; ext=""; [ $$os = windows ] && ext=.exe; \
		echo "  $$os/$$arch (cli)"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -tags nogui -trimpath -ldflags="$(LDFLAGS)" -o dist/agent-link-cli-$$os-$$arch$$ext . || exit 1; \
	done

# The desktop app for Windows: the system WebView2 needs no cgo, so both
# arches cross-compile. Linked as a GUI program (no console window), with
# the icon, version and a DPI-aware manifest from build/windows.
WINVER = $(shell echo $(VERSION) | sed -E 's/^v//; s/[^0-9.].*//')
release-windows:
	@mkdir -p dist
	@go run github.com/tc-hib/go-winres@v0.3.3 make --in build/windows/winres.json --arch amd64,arm64 --out rsrc \
		--file-version "$(or $(WINVER),0.0.0)" --product-version "$(or $(WINVER),0.0.0)"
	@for arch in amd64 arm64; do \
		echo "  windows/$$arch (app)"; \
		CGO_ENABLED=0 GOOS=windows GOARCH=$$arch go build -tags production -trimpath -ldflags="$(LDFLAGS) -H windowsgui" -o dist/agent-link-windows-$$arch.exe . || exit 1; \
	done; rm -f rsrc_windows_*.syso

# The desktop app for Linux links GTK 3 and WebKitGTK 4.1 through cgo, so it
# is built natively, one arch per machine.
release-linux:
	@mkdir -p dist
	@echo "  linux/$(shell go env GOARCH) (app)"
	@go build -tags "$(TAGS)" -trimpath -ldflags="$(LDFLAGS)" -o dist/agent-link-linux-$(shell go env GOARCH) .

clean:
	rm -rf agent-link agent-link.exe agent-link.app dist rsrc_windows_*.syso

# Development: the UI is served from internal/gui/assets and the window
# reloads itself when a file there is saved. With fswatch installed, the
# windows stay up and a Go change rebuilds and restarts only the backend
# behind them (build/dev.sh); without it, dev-once runs the app as one
# process. Uses its own ports so a running agentlink keeps serving the agents.
DEV_ADDR ?= 127.0.0.1:3426
DEV_UI ?= 127.0.0.1:3427
DEV_BACKEND ?= 127.0.0.1:3428
DEV_CONTROL ?= 127.0.0.1:3429
dev:
	@if command -v fswatch >/dev/null; then \
	  AGENT_LINK_ADDR=$(DEV_ADDR) AGENT_LINK_DEV_UI=$(DEV_UI) AGENT_LINK_DEV_BACKEND=$(DEV_BACKEND) AGENT_LINK_DEV_CONTROL=$(DEV_CONTROL) build/dev.sh; \
	else $(MAKE) --no-print-directory dev-once; fi

dev-once:
	@go build -tags dev -o agent-link-dev . && echo "  agent-link-dev · UI from internal/gui/assets, reload on save · gateway $(DEV_ADDR)"
	@AGENT_LINK_ADDR=$(DEV_ADDR) AGENT_LINK_DEV_UI=$(DEV_UI) ./agent-link-dev app
