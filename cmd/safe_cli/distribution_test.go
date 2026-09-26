package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestVersionIsStamped pins the distribution contract for issue #111 / fuzz F1: the version
// reported by `safe_cli version` / --version must be stamped at build time into main.version,
// not left at the hardcoded default, and a pushed tag must produce a release. These are
// source-level assertions (no runtime build fits): the Makefile and GoReleaser must inject
// -X main.version, and the release workflow must fire on a v* tag and run goreleaser.
func TestVersionIsStamped(t *testing.T) {
	root := repoRoot(t)
	read := func(rel string) string {
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		return string(b)
	}

	if mk := read("Makefile"); !strings.Contains(mk, "-X main.version=") {
		t.Error("Makefile build must stamp -X main.version= so `safe_cli version` is not the hardcoded default")
	}

	gr := read(".goreleaser.yaml")
	if !strings.Contains(gr, "main.version={{ .Version }}") {
		t.Error(".goreleaser.yaml must stamp -X main.version={{ .Version }} (the tag) into the binary")
	}
	for _, arch := range []string{"amd64", "arm64"} {
		if !strings.Contains(gr, arch) {
			t.Errorf(".goreleaser.yaml must build %s", arch)
		}
	}

	rel := read(".github/workflows/release.yml")
	if !strings.Contains(rel, "goreleaser") {
		t.Error("release workflow must run goreleaser")
	}
	if !strings.Contains(rel, `"v*"`) && !strings.Contains(rel, "- v*") {
		t.Error("release workflow must trigger on a v* tag")
	}
}

// TestVersionFlagWired pins that --version stays a wired global flag (fuzz F1): a fresh binary
// otherwise silently loses it. The Globals struct must declare a kong VersionFlag named version.
func TestVersionFlagWired(t *testing.T) {
	main := func() string {
		b, err := os.ReadFile(filepath.Join(repoRoot(t), "cmd", "safe_cli", "main.go"))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}()
	if !strings.Contains(main, "kong.VersionFlag") || !strings.Contains(main, `kong.Vars{"version"`) {
		t.Error("main.go must wire a kong.VersionFlag with kong.Vars{\"version\": …} so --version works")
	}
}
