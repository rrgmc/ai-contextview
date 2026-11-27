package main

import "runtime/debug"

// version is the binary's version. Release builds take it from the git tag:
//
//   - `go install github.com/rrgmc/ai-contextview/cmd/ai-contextview@v1.2.3` records
//     the module version, "v1.2.3".
//   - `go build` or `go install ./cmd/ai-contextview` in a checkout records the
//     tag on HEAD ("v1.2.3"), a pseudo-version for commits after a tag
//     ("v1.2.4-0.20261124101500-abcdef123456"), and "+dirty" when the tree has
//     uncommitted changes.
//
// `-ldflags "-X main.version=…"` overrides both. A build with no version
// information (for example `go run`, or a build without VCS stamping) is
// "devel", with the commit appended when it is known.
var version = ""

func init() {
	info, _ := debug.ReadBuildInfo() // nil when there is none
	version = resolveVersion(version, info)
}

// resolveVersion picks the version from an -ldflags value, else from the
// build info.
func resolveVersion(ldflags string, info *debug.BuildInfo) string {
	if ldflags != "" {
		return ldflags
	}
	if info == nil {
		return "devel"
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	rev, modified := "", false
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	if rev == "" {
		return "devel"
	}
	if len(rev) > 12 {
		rev = rev[:12]
	}
	if modified {
		return "devel (" + rev + ", modified)"
	}
	return "devel (" + rev + ")"
}
