package files

import (
	"errors"
	"path/filepath"
	"strings"

	"contracter/internal/format"
)

// Source re-points a contract path, or everything under a contract directory,
// to another location: a file, a directory or a pipe.
type Source struct {
	Path     string // the contract path being re-pointed
	Format   string // format of what is there, when the path does not say
	Location string
}

// ParseSource reads "CONTRACT-PATH=[FORMAT:]ACTUAL-PATH".
func ParseSource(arg string) (Source, error) {
	p, location, ok := strings.Cut(arg, "=")
	p = strings.TrimSuffix(p, "/")
	if !ok || location == "" || !ValidPath(p) {
		return Source{}, errors.New("want CONTRACT-PATH=[FORMAT:]ACTUAL-PATH, with CONTRACT-PATH relative to the repo")
	}
	s := Source{Path: p, Location: location}
	if name, rest, ok := strings.Cut(location, ":"); ok && rest != "" {
		if _, known := format.Named(name); known {
			s.Format, s.Location = name, rest
		}
	}
	return s, nil
}

// Touches reports whether the source re-points anything a pattern could match.
func (s Source) Touches(pattern string) bool {
	dir := compile(pattern).dir
	return under(s.Path, dir) || under(dir, s.Path)
}

func under(p, dir string) bool {
	return dir == "" || p == dir || strings.HasPrefix(p, dir+"/")
}

// locate returns where a contract path really is, and the source that says
// so, if any. The most specific source wins; the default is the repo.
func locate(root, p string, sources []Source) (string, *Source) {
	var best *Source
	for i, s := range sources {
		if under(p, s.Path) && (best == nil || len(s.Path) > len(best.Path)) {
			best = &sources[i]
		}
	}
	if best == nil {
		return filepath.Join(root, filepath.FromSlash(p)), nil
	}
	return best.Location + filepath.FromSlash(p[len(best.Path):]), best
}
