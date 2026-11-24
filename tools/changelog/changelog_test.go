package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const before = `# Changelog

Intro.

## [Unreleased]

See [changelog.d/](changelog.d/).

## [0.5.0] - 2026-11-24

### Added

- First release.

[Unreleased]: https://github.com/o/r/compare/v0.5.0...HEAD
[0.5.0]: https://github.com/o/r/releases/tag/v0.5.0
`

const after = `# Changelog

Intro.

## [Unreleased]

See [changelog.d/](changelog.d/).

## [0.6.0] - 2026-12-01

### Added

- A from a.
- B from b.

### Fixed

- A fix.
  Two lines.

## [0.5.0] - 2026-11-24

### Added

- First release.

[Unreleased]: https://github.com/o/r/compare/v0.6.0...HEAD
[0.6.0]: https://github.com/o/r/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/o/r/releases/tag/v0.5.0
`

const manifest = `{
  "name": "p",
  "version": "0.2.0",
  "author": {
    "name": "A"
  }
}
`

const readme = "# R\n\nReleases are git tags such as `v0.5.0`.\n\n" +
	"    go install github.com/o/r/cmd/ai-specview@v0.5.0\n\n" +
	"`ai-specview version` prints the git tag (`v1.2.3`).\n"

func writeRepo(t *testing.T, entries map[string]string) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "CHANGELOG.md"), []byte(before), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, filepath.Dir(pluginManifest)), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, pluginManifest), []byte(manifest), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, readmeFile), []byte(readme), 0o644))
	dir := filepath.Join(root, "changelog.d")
	require.NoError(t, os.Mkdir(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Readme\n"), 0o644))
	for name, text := range entries {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644))
	}
	return root
}

func TestReleaseCheckNotes(t *testing.T) {
	root := writeRepo(t, map[string]string{
		"a.md": "### Fixed\n\n- A fix.\n  Two lines.\n\n### Added\n\n- A from a.\n",
		"b.md": "\n### Added\n\n- B from b.\n\n",
	})

	err := run([]string{"check", "-root", root, "v0.6.0"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no [0.6.0] section")
	assert.Contains(t, err.Error(), "a.md is not merged")
	assert.Contains(t, err.Error(), "has version 0.2.0, want 0.6.0")
	assert.Contains(t, err.Error(), "names v0.5.0, want v0.6.0")

	require.NoError(t, run([]string{"release", "-root", root, "-date", "2026-12-01", "0.6.0"}))

	got, err := os.ReadFile(filepath.Join(root, "CHANGELOG.md"))
	require.NoError(t, err)
	assert.Equal(t, after, string(got))

	plugin, err := os.ReadFile(filepath.Join(root, pluginManifest))
	require.NoError(t, err)
	assert.Equal(t, strings.Replace(manifest, `"0.2.0"`, `"0.6.0"`, 1), string(plugin))

	gotReadme, err := os.ReadFile(filepath.Join(root, readmeFile))
	require.NoError(t, err)
	assert.Equal(t, strings.ReplaceAll(readme, "v0.5.0", "v0.6.0"), string(gotReadme))

	left, err := entryFiles(filepath.Join(root, "changelog.d"))
	require.NoError(t, err)
	assert.Empty(t, left)
	assert.FileExists(t, filepath.Join(root, "changelog.d", "README.md"))

	assert.NoError(t, run([]string{"check", "-root", root, "v0.6.0"}))

	n, err := notes(string(got), "0.6.0")
	require.NoError(t, err)
	assert.Equal(t, "### Added\n\n- A from a.\n- B from b.\n\n### Fixed\n\n- A fix.\n  Two lines.\n", n)

	err = run([]string{"release", "-root", root, "0.6.0"})
	assert.ErrorContains(t, err, "no change files")
}

func TestReleaseRejectsExistingVersion(t *testing.T) {
	_, err := release(before, "### Added\n\n- X.", "0.5.0", "2026-12-01")
	assert.ErrorContains(t, err, "already has a [0.5.0] section")
}

func TestParseEntryErrors(t *testing.T) {
	_, err := parseEntry("x.md", "### Improved\n\n- Y.\n")
	assert.ErrorContains(t, err, `unknown heading "Improved"`)

	_, err = parseEntry("x.md", "- Y.\n")
	assert.ErrorContains(t, err, "text before the first ### heading")

	_, err = parseEntry("x.md", "\n")
	assert.ErrorContains(t, err, "no ### sections")
}

func TestNormVersion(t *testing.T) {
	v, err := normVersion("v1.2.3")
	require.NoError(t, err)
	assert.Equal(t, "1.2.3", v)

	v, err = normVersion("1.2.3-rc.1")
	require.NoError(t, err)
	assert.Equal(t, "1.2.3-rc.1", v)

	_, err = normVersion("1.2")
	assert.Error(t, err)
}

func TestSetPluginVersion(t *testing.T) {
	out, err := setPluginVersion(manifest, "1.2.3-rc.1")
	require.NoError(t, err)
	v, err := pluginVersion(out)
	require.NoError(t, err)
	assert.Equal(t, "1.2.3-rc.1", v)

	_, err = setPluginVersion(`{"name": "p"}`, "1.0.0")
	assert.ErrorContains(t, err, "has no version")
	_, err = setPluginVersion(`{"version": "1", "x": {"version": "2"}}`, "1.0.0")
	assert.ErrorContains(t, err, "2 \"version\" fields")
	_, err = setPluginVersion(`{`, "1.0.0")
	assert.Error(t, err)

	assert.NoError(t, checkPlugin(out, "1.2.3-rc.1"))
	assert.ErrorContains(t, checkPlugin(manifest, "1.2.3"), "has version 0.2.0, want 1.2.3")
}

func TestReleasePrereleaseKeepsReadme(t *testing.T) {
	root := writeRepo(t, map[string]string{"a.md": "### Added\n\n- A.\n"})
	require.NoError(t, run([]string{"release", "-root", root, "-date", "2026-12-01", "0.6.0-rc.1"}))

	got, err := os.ReadFile(filepath.Join(root, readmeFile))
	require.NoError(t, err)
	assert.Equal(t, readme, string(got))
	assert.NoError(t, run([]string{"check", "-root", root, "v0.6.0-rc.1"}))
}

func TestSetReadmeVersion(t *testing.T) {
	out, err := setReadmeVersion(readme, "1.2.3")
	require.NoError(t, err)
	assert.Contains(t, out, "tags such as `v1.2.3`")
	assert.Contains(t, out, "cmd/ai-specview@v1.2.3\n")
	assert.Contains(t, out, "git tag (`v1.2.3`)")
	assert.NoError(t, checkReadme(out, "1.2.3"))

	_, err = setReadmeVersion("# R\n", "1.2.3")
	assert.ErrorContains(t, err, "no release tag")
	assert.ErrorContains(t, checkReadme("# R\n", "1.2.3"), "no release tag")

	mixed := strings.Replace(readme, "such as `v0.5.0`", "such as `v0.4.0`", 1)
	assert.ErrorContains(t, checkReadme(mixed, "0.5.0"), "names v0.4.0, want v0.5.0")
}
