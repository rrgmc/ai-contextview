package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// kinds are the Keep a Changelog headings, in the order a release lists them.
var kinds = []string{"Added", "Changed", "Deprecated", "Removed", "Fixed", "Security"}

var (
	versionRe    = regexp.MustCompile(`^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`)
	unreleasedRe = regexp.MustCompile(`^\[Unreleased\]: (\S+)/compare/(\S+?)\.\.\.HEAD$`)
	linkRe       = regexp.MustCompile(`^\[[^\]]+\]: `)
)

// normVersion accepts "1.2.3" or "v1.2.3" and returns "1.2.3".
func normVersion(v string) (string, error) {
	v = strings.TrimPrefix(v, "v")
	if !versionRe.MatchString(v) {
		return "", fmt.Errorf("version %q is not X.Y.Z", v)
	}
	return v, nil
}

// entryFiles lists the change files in dir, sorted, without README.md.
func entryFiles(dir string) ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return nil, err
	}
	var files []string
	for _, m := range matches {
		if filepath.Base(m) != "README.md" {
			files = append(files, m)
		}
	}
	sort.Strings(files)
	return files, nil
}

// parseEntry splits a change file into its "### Kind" sections.
func parseEntry(name, text string) (map[string][]string, error) {
	out := map[string][]string{}
	kind := ""
	for i, line := range strings.Split(text, "\n") {
		if h, ok := strings.CutPrefix(line, "### "); ok {
			kind = strings.TrimSpace(h)
			if !isKind(kind) {
				return nil, fmt.Errorf("%s:%d: unknown heading %q (want one of %s)",
					name, i+1, kind, strings.Join(kinds, ", "))
			}
			continue
		}
		if kind == "" {
			if strings.TrimSpace(line) != "" {
				return nil, fmt.Errorf("%s:%d: text before the first ### heading", name, i+1)
			}
			continue
		}
		out[kind] = append(out[kind], line)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: no ### sections", name)
	}
	for k, lines := range out {
		out[k] = trimBlank(lines)
	}
	return out, nil
}

func isKind(s string) bool {
	for _, k := range kinds {
		if k == s {
			return true
		}
	}
	return false
}

// trimBlank drops blank lines at both ends.
func trimBlank(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// mergeEntries joins the change files into one release body.
func mergeEntries(files []string) (string, error) {
	merged := map[string][]string{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return "", err
		}
		sections, err := parseEntry(filepath.Base(f), string(b))
		if err != nil {
			return "", err
		}
		for k, lines := range sections {
			if len(lines) > 0 {
				merged[k] = append(merged[k], lines...)
			}
		}
	}
	var parts []string
	for _, k := range kinds {
		if len(merged[k]) > 0 {
			parts = append(parts, "### "+k+"\n\n"+strings.Join(merged[k], "\n"))
		}
	}
	if len(parts) == 0 {
		return "", errors.New("the change files hold no entries")
	}
	return strings.Join(parts, "\n\n"), nil
}

// release adds a "## [version] - date" section built from the change files
// to changelog, and updates the compare links at its end.
func release(changelog, body, version, date string) (string, error) {
	lines := strings.Split(changelog, "\n")
	if findSection(lines, version) >= 0 {
		return "", fmt.Errorf("CHANGELOG.md already has a [%s] section", version)
	}
	unrel := findSection(lines, "Unreleased")
	if unrel < 0 {
		return "", errors.New("CHANGELOG.md has no ## [Unreleased] section")
	}
	insert := len(lines)
	for i := unrel + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") || linkRe.MatchString(lines[i]) {
			insert = i
			break
		}
	}

	link := -1
	var base, prev string
	for i, l := range lines {
		if m := unreleasedRe.FindStringSubmatch(l); m != nil {
			link, base, prev = i, m[1], m[2]
		}
	}
	if link < 0 {
		return "", errors.New("CHANGELOG.md has no [Unreleased]: …/compare/vX.Y.Z...HEAD link")
	}

	tag := "v" + version
	out := make([]string, 0, len(lines)+8)
	for i, l := range lines {
		if i == insert {
			out = append(out, fmt.Sprintf("## [%s] - %s", version, date), "")
			out = append(out, strings.Split(body, "\n")...)
			out = append(out, "")
		}
		if i == link {
			out = append(out,
				fmt.Sprintf("[Unreleased]: %s/compare/%s...HEAD", base, tag),
				fmt.Sprintf("[%s]: %s/compare/%s...%s", version, base, prev, tag))
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n"), nil
}

// findSection returns the line index of "## [name]", or -1.
func findSection(lines []string, name string) int {
	head := "## [" + name + "]"
	for i, l := range lines {
		if l == head || strings.HasPrefix(l, head+" ") {
			return i
		}
	}
	return -1
}

// notes returns the body of the "## [version]" section.
func notes(changelog, version string) (string, error) {
	lines := strings.Split(changelog, "\n")
	start := findSection(lines, version)
	if start < 0 {
		return "", fmt.Errorf("CHANGELOG.md has no [%s] section", version)
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") || linkRe.MatchString(lines[i]) {
			end = i
			break
		}
	}
	body := trimBlank(lines[start+1 : end])
	if len(body) == 0 {
		return "", fmt.Errorf("the [%s] section is empty", version)
	}
	return strings.Join(body, "\n") + "\n", nil
}

// check reports why the tree is not ready to release version.
func check(changelog string, files []string, version string) error {
	var errs []error
	if _, err := notes(changelog, version); err != nil {
		errs = append(errs, err)
	}
	if !strings.Contains(changelog, "\n["+version+"]: ") {
		errs = append(errs, fmt.Errorf("CHANGELOG.md has no [%s] compare link", version))
	}
	if !strings.Contains(changelog, "/compare/v"+version+"...HEAD") {
		errs = append(errs, fmt.Errorf("the [Unreleased] link does not start from v%s", version))
	}
	for _, f := range files {
		errs = append(errs, fmt.Errorf("%s is not merged into CHANGELOG.md", f))
	}
	return errors.Join(errs...)
}
