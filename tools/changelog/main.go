// Command changelog merges the files in changelog.d/ into CHANGELOG.md, sets
// the plugin version and the README's install tag, and checks a release
// against them. Run it from the
// repository root:
//
//	go run ./tools/changelog release 1.2.3   # merge changelog.d/ into a [1.2.3] section, set the plugin and README to 1.2.3
//	go run ./tools/changelog check v1.2.3    # fail unless v1.2.3 is ready to tag
//	go run ./tools/changelog notes v1.2.3    # print the [1.2.3] section
//
// See changelog.d/README.md.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const usage = `usage:
  changelog release [-date YYYY-MM-DD] X.Y.Z
  changelog check vX.Y.Z
  changelog notes vX.Y.Z`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "changelog:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("%s", usage)
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	date := fs.String("date", time.Now().Format(time.DateOnly), "release date")
	root := fs.String("root", ".", "repository root")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("%s", usage)
	}
	version, err := normVersion(fs.Arg(0))
	if err != nil {
		return err
	}

	path := filepath.Join(*root, "CHANGELOG.md")
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	files, err := entryFiles(filepath.Join(*root, "changelog.d"))
	if err != nil {
		return err
	}
	manifestPath := filepath.Join(*root, pluginManifest)
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}
	readmePath := filepath.Join(*root, readmeFile)
	readme, err := os.ReadFile(readmePath)
	if err != nil {
		return err
	}

	switch args[0] {
	case "release":
		if _, err := time.Parse(time.DateOnly, *date); err != nil {
			return fmt.Errorf("-date: %w", err)
		}
		if len(files) == 0 {
			return fmt.Errorf("changelog.d/ has no change files")
		}
		body, err := mergeEntries(files)
		if err != nil {
			return err
		}
		out, err := release(string(b), body, version, *date)
		if err != nil {
			return err
		}
		plugin, err := setPluginVersion(string(manifest), version)
		if err != nil {
			return err
		}
		readmeOut := string(readme)
		if !isPrerelease(version) {
			if readmeOut, err = setReadmeVersion(readmeOut, version); err != nil {
				return err
			}
		}
		if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(manifestPath, []byte(plugin), 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(readmePath, []byte(readmeOut), 0o644); err != nil {
			return err
		}
		for _, f := range files {
			if err := os.Remove(f); err != nil {
				return err
			}
		}
		fmt.Printf("CHANGELOG.md: added [%s] from %d change files\n", version, len(files))
		fmt.Printf("%s: version %s\n", pluginManifest, version)
		if !isPrerelease(version) {
			fmt.Printf("%s: install tag v%s\n", readmeFile, version)
		}
		return nil
	case "check":
		errs := []error{check(string(b), files, version), checkPlugin(string(manifest), version)}
		if !isPrerelease(version) {
			errs = append(errs, checkReadme(string(readme), version))
		}
		return errors.Join(errs...)
	case "notes":
		n, err := notes(string(b), version)
		if err != nil {
			return err
		}
		fmt.Print(n)
		return nil
	default:
		return fmt.Errorf("unknown command %q\n%s", args[0], usage)
	}
}
