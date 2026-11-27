package resolve

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rrgmc/ai-contextview/internal/protocol"
)

func testCwd(t *testing.T) string {
	abs, err := filepath.Abs("testdata/mod")
	require.NoError(t, err)
	return abs
}

func TestResolveSymbols(t *testing.T) {
	cwd := testCwd(t)
	r := New()
	cases := []struct {
		sym        string
		start, end int
		kind       string
	}{
		{"sync.(*Retrier).Next", 9, 14, "method"},
		{"(*Retrier).Next", 9, 14, "method"},
		{"Retrier.Next", 9, 14, "method"},
		{"sync.Retrier.Reset", 16, 20, "method"},
		{"sync.Retrier", 3, 7, "type"},
		{"New", 22, 25, "func"},
		{"sync.(*List[T]).Push", 32, 35, "method"},
		{"List", 27, 30, "type"},
		{"MaxDelay", 37, 38, "const"},
		{"DefaultBase", 41, 42, "var"},
		{"example.com/mod/sync.New", 22, 25, "func"},
	}
	for _, c := range cases {
		t.Run(c.sym, func(t *testing.T) {
			ref, err := r.Resolve(context.Background(), cwd, protocol.CodeRef{Path: "sync/retry.go", Symbol: c.sym})
			require.NoError(t, err)
			assert.Equal(t, c.start, ref.StartLine)
			assert.Equal(t, c.end, ref.EndLine)
			assert.Equal(t, c.kind, ref.Kind)
			assert.Equal(t, "symbol", ref.Source)
			assert.Equal(t, filepath.Join(cwd, "sync/retry.go"), ref.Path)
		})
	}
}

func TestResolveNearMiss(t *testing.T) {
	_, err := New().Resolve(context.Background(), testCwd(t), protocol.CodeRef{Path: "sync/retry.go", Symbol: "sync.(*Retrier).Nxt"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "did you mean")
	assert.Contains(t, err.Error(), "Retrier.Next")
}

func TestResolveLines(t *testing.T) {
	r := New()
	cwd := testCwd(t)
	ref, err := r.Resolve(context.Background(), cwd, protocol.CodeRef{Path: "sync/retry.go", Lines: "9-14"})
	require.NoError(t, err)
	assert.Equal(t, 9, ref.StartLine)
	assert.Equal(t, 14, ref.EndLine)
	assert.Equal(t, "lines", ref.Source)

	ref, err = r.Resolve(context.Background(), cwd, protocol.CodeRef{Path: "sync/retry.go", Lines: "40-999"})
	require.NoError(t, err)
	assert.Equal(t, 44, ref.EndLine, "clamped to the file length")

	_, err = r.Resolve(context.Background(), cwd, protocol.CodeRef{Path: "sync/retry.go", Lines: "x"})
	assert.Error(t, err)
	_, err = r.Resolve(context.Background(), cwd, protocol.CodeRef{Path: "go.mod", Symbol: "X"})
	assert.ErrorContains(t, err, "only supported for Go")
	_, err = r.Resolve(context.Background(), cwd, protocol.CodeRef{Path: "sync/retry.go", Symbol: "X", Lines: "1"})
	assert.ErrorContains(t, err, "exactly one")
	_, err = r.Resolve(context.Background(), cwd, protocol.CodeRef{Path: "missing.go", Lines: "1"})
	assert.ErrorContains(t, err, "does not exist")
}
