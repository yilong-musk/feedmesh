package main

import (
	"runtime/debug"
	"strings"
)

// version is injected at build time with -X main.version=...  Release archives
// and the module build info both carry the real version; the latter is the only
// source available to `go install`, which cannot pass linker flags.
var version = "dev"

func init() {
	version = resolveVersion(version, moduleVersion())
}

// moduleVersion returns the version recorded by the Go toolchain, e.g.
// "v0.1.1" for `go install ...@v0.1.1`, or "(devel)" for a local build.
func moduleVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		return info.Main.Version
	}
	return ""
}

// resolveVersion prefers the injected version and normalises its leading "v".
// Unset markers ("", "dev", "(devel)") fall through to the next source.
func resolveVersion(injected, module string) string {
	for _, candidate := range []string{injected, module} {
		candidate = strings.TrimPrefix(strings.TrimSpace(candidate), "v")
		if candidate == "" || candidate == "dev" || candidate == "(devel)" {
			continue
		}
		return candidate
	}
	return "dev"
}
