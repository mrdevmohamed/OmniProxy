# OmniProxy — Phase 1 (Android, Linux, Windows) release builds + testing.
#
#   make build         # all three platforms (Windows app bundle needs a
#                      #   Windows host; core DLL cross-compiles from Linux)
#   make build-native  # Android + Linux only (what a Linux host can fully build)
#   make check         # go build/vet/gofmt + flutter analyze
#   make test          # go test (core + engine) + flutter test (incl. Linux E2E)
#   make e2e-android   # bridge E2E on a connected Android device
#
# Overridable: FLUTTER, GOMOBILE, DEVICE (e.g. make e2e-android DEVICE=2eb95e94)

SHELL := /usr/bin/env bash
MAKEFLAGS += --no-print-directory

ROOT := $(CURDIR)
GO ?= go
FLUTTER ?= flutter
GOMOBILE ?= $(shell $(GO) env GOPATH)/bin/gomobile

CORE_OUT := $(ROOT)/core/out

# gomobile bind settings (docs/platform-notes.md §Android).
ANDROID_API := 24          # NDK 28 supports 21..35; Flutter default minSdk is 24
JAVAPKG := com.omniproxy.bind  # must match Bridge.kt import
# gVisor TUN stack build tag: Android AAR (aar) + Linux helper (linux-core).
GOMOD_TAGS := with_gvisor
AAR := $(CORE_OUT)/omniproxy.aar
AAR_TARGET := $(ROOT)/app/android/app/libs/omniproxy.aar

MINGW_CC ?= x86_64-w64-mingw32-gcc
WINTUN_VERSION := 0.14.1
HOST_OS := $(shell uname -s)

.PHONY: all build build-native check test clean help \
	go-check go-check-windows go-test flutter-check flutter-test \
	build-android aar apk appbundle \
	build-linux linux-core flutter-linux \
	build-windows windows-core wintun flutter-windows \
	e2e e2e-android

all: build

## Full release pipeline for every platform.
build: build-android build-linux build-windows
	@echo
	@echo "==> Android + Linux bundles and Windows core DLL built."
	@echo "    The Windows app bundle requires a Windows host: make flutter-windows"

## Android + Linux only (everything a Linux host can fully build).
build-native: build-android build-linux

# --- Quality gates ----------------------------------------------------------

## go build + vet + gofmt for core and engine.
go-check:
	@cd $(ROOT)/core && go build ./... && go vet ./... && \
		( test -z "$$(gofmt -l .)" || ( echo "gofmt needed:"; gofmt -l .; false ) )
	@cd $(ROOT)/engine && go build ./... && go vet ./... && \
		( test -z "$$(gofmt -l .)" || ( echo "gofmt needed:"; gofmt -l .; false ) )
	@$(MAKE) go-check-windows
	@echo "go-check: core + engine clean"

## GOOS=windows cross-build gate (M8): catches Windows-tagged regressions
## (core/glue platform selection) on a Linux host. Uses the same GOMOD_TAGS
## (with_gvisor) as the real Windows release build. Skips when the mingw
## cross-compiler is not installed.
go-check-windows:
	@command -v $(MINGW_CC) >/dev/null 2>&1 || { \
		echo "go-check-windows: $(MINGW_CC) not found — skipping windows cross-build"; \
		exit 0; }
	@mkdir -p $(CORE_OUT)
	@cd $(ROOT)/core && CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=$(MINGW_CC) \
		go build -buildmode=c-shared -tags $(GOMOD_TAGS) -o $(CORE_OUT)/omniproxy-check.dll ./glue
	@rm -f $(CORE_OUT)/omniproxy-check.dll $(CORE_OUT)/omniproxy-check.h
	@echo "go-check-windows: green"

## go test for core and engine.
go-test:
	@cd $(ROOT)/core && go test -count=1 ./...
	@cd $(ROOT)/engine && go test -count=1 ./...

## flutter analyze.
flutter-check:
	@cd $(ROOT)/app && $(FLUTTER) analyze

## flutter test (unit + widget + host Linux bridge E2E).
flutter-test:
	@cd $(ROOT)/app && $(FLUTTER) test

## Static analysis + formatting gates.
check: go-check flutter-check

## Unit / widget tests.
test: go-test flutter-test

## End-to-end tests (Android bridge on a connected device).
e2e: e2e-android
e2e-android:
	@cd $(ROOT)/app && $(FLUTTER) test integration_test/bridge_e2e_test.dart \
		$(if $(DEVICE),-d $(DEVICE),)

# --- Android (release) ------------------------------------------------------

## AAR (gomobile bind) copied into the Flutter android/libs directory.
aar:
	@command -v $(GOMOBILE) >/dev/null 2>&1 || { \
		echo "error: gomobile not found at $(GOMOBILE)"; \
		echo "       install it with: go install golang.org/x/mobile/cmd/gomobile@latest"; \
		exit 1; }
	@mkdir -p $(dir $(AAR)) $(dir $(AAR_TARGET))
	@cd $(ROOT)/core && $(GOMOBILE) bind \
		-androidapi=$(ANDROID_API) \
		-javapkg $(JAVAPKG) \
		-tags $(GOMOD_TAGS) \
		-target=android \
		-o $(AAR) omniproxy/core/mobile
	@cp -v $(AAR) $(AAR_TARGET)

## Release APK (and AAB for Play).
apk: aar
	@cd $(ROOT)/app && $(FLUTTER) build apk --release
appbundle: aar
	@cd $(ROOT)/app && $(FLUTTER) build appbundle --release

## Full Android release.
build-android: apk

# --- Linux (release) --------------------------------------------------------

## Go bridge artifacts: libomniproxy.so (c-shared) + omniproxy-helper (with_gvisor).
linux-core:
	@GOMOD_TAGS="$(GOMOD_TAGS)" $(ROOT)/tools/build_linux.sh

## Release desktop bundle.
flutter-linux: linux-core
	@cd $(ROOT)/app && $(FLUTTER) build linux --release

## Full Linux release.
build-linux: flutter-linux

# --- Windows (release) ------------------------------------------------------

## Cross-compiled omniproxy.dll + bundled wintun.dll (runs on a Linux host).
## Built with the same with_gvisor tag as the Android AAR and Linux helper so
## the gVisor user-space TUN stack is available for Windows VPN mode.
windows-core:
	@command -v $(MINGW_CC) >/dev/null 2>&1 || { \
		echo "error: mingw-w64 cross compiler not found ($(MINGW_CC))."; \
		echo "       Debian/Ubuntu: apt install gcc-mingw-w64-x86-64"; \
		exit 1; }
	@mkdir -p $(CORE_OUT)
	@cd $(ROOT)/core && CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=$(MINGW_CC) \
		go build -buildmode=c-shared -tags $(GOMOD_TAGS) -o $(CORE_OUT)/omniproxy.dll ./glue
	@$(MAKE) wintun
	@echo "==> windows core artifacts in $(CORE_OUT)"

## Download wintun.dll (runtime dependency for the TUN path) if missing.
wintun:
	@if [ ! -f "$(CORE_OUT)/wintun.dll" ]; then \
		if command -v curl >/dev/null 2>&1 && command -v unzip >/dev/null 2>&1; then \
			echo "==> fetching wintun $(WINTUN_VERSION)"; \
			tmp=$$(mktemp -d); \
			curl -fsSL -o "$$tmp/wintun.zip" \
				"https://www.wintun.net/builds/wintun-$(WINTUN_VERSION).zip" && \
			unzip -o -j "$$tmp/wintun.zip" "wintun/bin/amd64/wintun.dll" \
				-d "$(CORE_OUT)"; \
			rm -rf "$$tmp"; \
		else \
			echo "warning: curl/unzip unavailable — place wintun.dll into $(CORE_OUT) manually"; \
		fi; \
	fi

## Release Windows app bundle — requires a Windows host (or CI).
flutter-windows:
	@case "$(HOST_OS)" in MINGW*|MSYS*|CYGWIN*) ;; *) \
		echo "error: flutter build windows must run on a Windows host (this is $(HOST_OS))."; \
		echo "       Run 'make flutter-windows' in CI on Windows."; exit 1;; esac
	@cd $(ROOT)/app && $(FLUTTER) build windows --release

## Full Windows release (core DLL here; app bundle on Windows).
build-windows: windows-core

# --- Cleanup ----------------------------------------------------------------

clean:
	@rm -rf $(CORE_OUT)
	@cd $(ROOT)/app && $(FLUTTER) clean
	@echo "cleaned: core/out + flutter build artifacts"

help:
	@echo "OmniProxy Makefile (Android, Linux, Windows — release + testing)"
	@echo
	@echo "Targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## //' | sed 's/\.$$//'
	@echo
	@echo "Variables: FLUTTER=$(FLUTTER)  GOMOBILE=$(GOMOBILE)  DEVICE=$(DEVICE)"
