package main

import (
	"fmt"
	"regexp"
	"strings"
)

// readmeFile is the repository's README, relative to the repository root.
// Its install section names the latest release tag, so a release sets it and
// check requires it.
const readmeFile = "README.md"

// readmeVersionRe matches the release tags in the README's install section:
// the example tag ("tags such as `v1.2.3`") and the install command
// ("ai-specview@v1.2.3"). Other tags in the README, such as the sample
// output of `ai-specview version`, do not match.
var readmeVersionRe = regexp.MustCompile("(tags such as `v|/cmd/ai-specview@v)([0-9]+\\.[0-9]+\\.[0-9]+[^`\\s]*)")

// isPrerelease reports whether version has a suffix such as "-rc.1". The
// README keeps pointing at the last stable release during a prerelease.
func isPrerelease(version string) bool {
	return strings.Contains(version, "-")
}

// setReadmeVersion returns the README with every release tag in its install
// section set to version, keeping the rest of the file as it is.
func setReadmeVersion(readme, version string) (string, error) {
	if !readmeVersionRe.MatchString(readme) {
		return "", fmt.Errorf("%s has no release tag in its install section", readmeFile)
	}
	return readmeVersionRe.ReplaceAllString(readme, "${1}"+version), nil
}

// checkReadme fails unless every release tag in the README's install section
// is version.
func checkReadme(readme, version string) error {
	m := readmeVersionRe.FindAllStringSubmatch(readme, -1)
	if len(m) == 0 {
		return fmt.Errorf("%s has no release tag in its install section", readmeFile)
	}
	for _, s := range m {
		if s[2] != version {
			return fmt.Errorf("%s names v%s, want v%s (run the release command)", readmeFile, s[2], version)
		}
	}
	return nil
}
