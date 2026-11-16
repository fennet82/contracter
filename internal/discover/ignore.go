package discover

import (
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

// IgnoreFile is where a repo adds to, or takes back from, what is left out
// by default. The defaults are in default.contracterignore, which also
// documents the syntax.
const IgnoreFile = ".contracterignore"

//go:embed default.contracterignore
var defaultIgnore string

type ignoreRule struct {
	pattern  string
	dirOnly  bool // "name/"
	anywhere bool // no "/" inside: the name may be at any depth
	keep     bool // "!pattern"
}

type ignoreList []ignoreRule

// loadIgnore returns the defaults followed by the repo's own rules.
func loadIgnore(tree fs.FS) (ignoreList, error) {
	list, err := parseIgnore("default.contracterignore", defaultIgnore)
	if err != nil {
		return nil, err
	}
	data, err := fs.ReadFile(tree, IgnoreFile)
	if errors.Is(err, fs.ErrNotExist) {
		return list, nil
	}
	if err != nil {
		return nil, err
	}
	own, err := parseIgnore(IgnoreFile, string(data))
	return append(list, own...), err
}

func parseIgnore(file, text string) (ignoreList, error) {
	var list ignoreList
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var r ignoreRule
		r.pattern, r.keep = strings.CutPrefix(line, "!")
		r.pattern, r.dirOnly = strings.CutSuffix(r.pattern, "/")
		r.anywhere = !strings.Contains(r.pattern, "/")
		r.pattern = strings.TrimPrefix(r.pattern, "/")
		if _, err := path.Match(r.pattern, ""); err != nil || r.pattern == "" {
			return nil, fmt.Errorf("%s: line %d: bad pattern %q", file, i+1, line)
		}
		// As in .gitignore, a comment is a whole line. Read as a pattern,
		// "name # why" would match nothing and say nothing.
		if strings.Contains(line, " #") {
			return nil, fmt.Errorf("%s: line %d: a comment must be on a line of its own", file, i+1)
		}
		list = append(list, r)
	}
	return list, nil
}

// ignored reports whether a path, relative to the repo root, is left out.
// The last rule that matches decides, so a later "!name" overrides an
// earlier "name".
func (l ignoreList) ignored(p string, isDir bool) bool {
	ignored := false
	for _, r := range l {
		if r.dirOnly && !isDir {
			continue
		}
		target := p
		if r.anywhere {
			target = path.Base(p)
		}
		if ok, _ := path.Match(r.pattern, target); ok {
			ignored = !r.keep
		}
	}
	return ignored
}
