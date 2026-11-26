package glob

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatch(t *testing.T) {
	cases := []struct {
		pattern, path string
		want          bool
	}{
		{"/r/docs/specs/**", "/r/docs/specs/SW-1.md", true},
		{"/r/docs/specs/**", "/r/docs/specs/a/b.md", true},
		{"/r/docs/specs/*.md", "/r/docs/specs/a/b.md", false},
		{"/r/docs/specs/*.md", "/r/docs/specs/SW-1.md", true},
		{"/r/**/rfc/*.md", "/r/rfc/x.md", true},
		{"/r/**/rfc/*.md", "/r/a/b/rfc/x.md", true},
		{"/r/docs/SW-?.md", "/r/docs/SW-1.md", true},
		{"/r/docs/SW-?.md", "/r/docs/SW-12.md", false},
		{"/r/docs/a.md", "/r/docs/a.md", true},
		{"/r/docs/a.md", "/r/docs/a.mdx", false},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, Match(c.pattern, c.path), "%s ~ %s", c.pattern, c.path)
	}
	assert.True(t, MatchAny([]string{"/x/*", "/r/*.md"}, "/r/a.md"))
	assert.False(t, MatchAny(nil, "/r/a.md"))
}
