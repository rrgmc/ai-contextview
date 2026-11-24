package main

import (
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveVersion(t *testing.T) {
	vcs := func(rev, modified string) []debug.BuildSetting {
		return []debug.BuildSetting{{Key: "vcs.revision", Value: rev}, {Key: "vcs.modified", Value: modified}}
	}
	const rev = "ebc1df0a1b2c3d4e5f60718293a4b5c6d7e8f901"
	tests := []struct {
		name    string
		ldflags string
		info    *debug.BuildInfo
		want    string
	}{
		{"ldflags wins", "v9.9.9", &debug.BuildInfo{Main: debug.Module{Version: "v1.2.3"}}, "v9.9.9"},
		{"no build info", "", nil, "devel"},
		{"tag", "", &debug.BuildInfo{Main: debug.Module{Version: "v1.2.3"}, Settings: vcs(rev, "false")}, "v1.2.3"},
		{"pseudo-version", "", &debug.BuildInfo{Main: debug.Module{Version: "v1.2.4-0.20261124101500-ebc1df0a1b2c"}}, "v1.2.4-0.20261124101500-ebc1df0a1b2c"},
		{"dirty tree", "", &debug.BuildInfo{Main: debug.Module{Version: "v1.2.3+dirty"}}, "v1.2.3+dirty"},
		{"devel with commit", "", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}, Settings: vcs(rev, "false")}, "devel (ebc1df0a1b2c)"},
		{"devel modified", "", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}, Settings: vcs(rev, "true")}, "devel (ebc1df0a1b2c, modified)"},
		{"devel without vcs", "", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, "devel"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, resolveVersion(tt.ldflags, tt.info))
		})
	}
}
