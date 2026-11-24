package specsrc

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rrgmc/ai-specview/internal/protocol"
)

// Error is an error with a protocol error code.
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Message }

func errf(code, format string, args ...any) error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Code returns the protocol code of an error, or "internal".
func Code(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return protocol.CodeInternal
}

// Root is a shown spec: a single markdown file or a folder of artifacts.
type Root struct {
	// Abs is the absolute path given to spec_show.
	Abs   string
	IsDir bool
	// Dir is the directory artifact paths are relative to: Abs for a folder,
	// its parent for a single file.
	Dir string
	// Primary is the primary file, relative to Dir.
	Primary   string
	Artifacts []protocol.Artifact
}

// PrimaryAbs is the absolute path of the primary file.
func (r *Root) PrimaryAbs() string { return filepath.Join(r.Dir, r.Primary) }

// Artifact returns the artifact for a path relative to Dir.
func (r *Root) Artifact(rel string) *protocol.Artifact {
	rel = filepath.ToSlash(rel)
	for i := range r.Artifacts {
		if r.Artifacts[i].Path == rel {
			return &r.Artifacts[i]
		}
	}
	return nil
}

// Rel returns the path of abs relative to Dir, with forward slashes.
func (r *Root) Rel(abs string) string {
	rel, err := filepath.Rel(r.Dir, abs)
	if err != nil {
		return abs
	}
	return filepath.ToSlash(rel)
}

// IsMarkdown reports whether a path has a markdown extension.
func IsMarkdown(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown":
		return true
	}
	return false
}

const maxDepth = 3

// LoadRoot loads a single markdown file or a folder of artifacts.
func LoadRoot(abs, primary string) (*Root, error) {
	st, err := os.Stat(abs)
	if err != nil {
		return nil, errf(protocol.CodeFileNotFound, "%s does not exist", abs)
	}
	if !st.IsDir() {
		if primary != "" {
			return nil, errf(protocol.CodeBadRequest, "primary is only allowed for folders")
		}
		if !IsMarkdown(abs) {
			return nil, errf(protocol.CodeNotMarkdown, "%s is not a markdown file (.md or .markdown)", abs)
		}
		r := &Root{Abs: abs, Dir: filepath.Dir(abs), Primary: filepath.Base(abs)}
		a, err := loadArtifact(r.Dir, r.Primary)
		if err != nil {
			return nil, err
		}
		r.Artifacts = []protocol.Artifact{a}
		return r, nil
	}

	r := &Root{Abs: abs, IsDir: true, Dir: abs}
	files, err := ListMarkdown(abs)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, errf(protocol.CodeEmptyFolder, "%s contains no markdown files", abs)
	}
	for _, f := range files {
		a, err := loadArtifact(abs, f)
		if err != nil {
			return nil, err
		}
		r.Artifacts = append(r.Artifacts, a)
	}
	if primary != "" {
		p := filepath.ToSlash(filepath.Clean(primary))
		if filepath.IsAbs(primary) {
			p = r.Rel(primary)
		}
		if strings.HasPrefix(p, "../") || r.Artifact(p) == nil {
			return nil, errf(protocol.CodeBadRequest, "primary %q is not a markdown file inside %s", primary, abs)
		}
		r.Primary = p
	} else {
		r.Primary = files[0]
		for _, cand := range []string{"proposal.md", "spec.md", "README.md"} {
			if r.Artifact(cand) != nil {
				r.Primary = cand
				break
			}
		}
	}
	// Primary first, the rest in path order.
	sort.SliceStable(r.Artifacts, func(i, j int) bool {
		return r.Artifacts[i].Path == r.Primary && r.Artifacts[j].Path != r.Primary
	})
	return r, nil
}

// ListMarkdown lists markdown files under dir, up to 3 levels deep, skipping
// dot-files and dot-directories. Paths are relative with forward slashes,
// sorted.
func ListMarkdown(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if p == dir {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		if strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		depth := strings.Count(filepath.ToSlash(rel), "/") + 1
		if d.IsDir() {
			if depth >= maxDepth {
				return filepath.SkipDir
			}
			return nil
		}
		if IsMarkdown(p) {
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}

func loadArtifact(dir, rel string) (protocol.Artifact, error) {
	data, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		return protocol.Artifact{}, errf(protocol.CodeFileNotFound, "cannot read %s: %v", rel, err)
	}
	body, fm := Prepare(data)
	secs := Headings(Sections(body))
	if secs == nil {
		secs = []string{}
	}
	return protocol.Artifact{Path: filepath.ToSlash(rel), Kind: InferKind(rel, fm), Sections: secs}, nil
}

// InferKind infers the artifact kind: frontmatter `kind:`, then parent
// directory names, then the file name, else other.
func InferKind(rel, frontmatter string) protocol.ArtifactKind {
	for _, line := range strings.Split(frontmatter, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), ":")
		if ok && strings.TrimSpace(k) == "kind" {
			kind := protocol.ArtifactKind(strings.Trim(strings.TrimSpace(v), `"'`))
			if kind.Valid() {
				return kind
			}
		}
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for i := len(parts) - 2; i >= 0; i-- {
		switch strings.ToLower(parts[i]) {
		case "adr", "adrs", "decisions":
			return protocol.KindADR
		case "research":
			return protocol.KindResearch
		case "open-questions", "questions":
			return protocol.KindOpenQuestion
		case "specs":
			return protocol.KindSpec
		}
	}
	switch strings.ToLower(parts[len(parts)-1]) {
	case "proposal.md", "spec.md", "design.md":
		return protocol.KindSpec
	}
	return protocol.KindOther
}

// ReadPrepared reads a markdown file and blanks its frontmatter.
func ReadPrepared(abs string) ([]byte, error) {
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	body, _ := Prepare(data)
	return body, nil
}
