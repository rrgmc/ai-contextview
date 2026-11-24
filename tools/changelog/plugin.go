package main

import (
	"encoding/json"
	"fmt"
	"regexp"
)

// pluginManifest is the Claude Code plugin's manifest, relative to the
// repository root. Its "version" is the release version, so a release sets
// it and check requires it.
const pluginManifest = "plugin/.claude-plugin/plugin.json"

var pluginVersionRe = regexp.MustCompile(`("version"\s*:\s*")[^"]*(")`)

// pluginVersion returns the "version" of a plugin manifest.
func pluginVersion(manifest string) (string, error) {
	var m struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal([]byte(manifest), &m); err != nil {
		return "", fmt.Errorf("%s: %w", pluginManifest, err)
	}
	if m.Version == "" {
		return "", fmt.Errorf("%s has no version", pluginManifest)
	}
	return m.Version, nil
}

// setPluginVersion returns the manifest with its "version" set to version,
// keeping the rest of the file as it is.
func setPluginVersion(manifest, version string) (string, error) {
	if _, err := pluginVersion(manifest); err != nil {
		return "", err
	}
	if n := len(pluginVersionRe.FindAllStringIndex(manifest, -1)); n != 1 {
		return "", fmt.Errorf("%s has %d \"version\" fields, want 1", pluginManifest, n)
	}
	out := pluginVersionRe.ReplaceAllString(manifest, "${1}"+version+"${2}")
	if got, err := pluginVersion(out); err != nil || got != version {
		return "", fmt.Errorf("%s: could not set the version to %s", pluginManifest, version)
	}
	return out, nil
}

// checkPlugin fails unless the manifest's version is the release version.
func checkPlugin(manifest, version string) error {
	got, err := pluginVersion(manifest)
	if err != nil {
		return err
	}
	if got != version {
		return fmt.Errorf("%s has version %s, want %s (run the release command)", pluginManifest, got, version)
	}
	return nil
}
