// Package files finds and reads the files a contract entry covers: it owns
// path patterns, the re-pointing of paths to other locations, and loading.
package files

import (
	"path"
	"regexp"
	"strings"
)

const placeholder = `\{[^/{}]*\}`

var (
	placeholderRE = regexp.MustCompile(placeholder)
	wildcardRE    = regexp.MustCompile(placeholder + `|\*\*/|\*\*|\*|\?`)
)

// A pattern is an entry path such as "public/locales/{lng}/*.json". "*" and "?"
// stay within one path segment and "**" crosses segments. A "{name}"
// placeholder also stays within a segment, and every value it takes is an
// instance that must look like the others.
type pattern struct {
	re           *regexp.Regexp // one capture group per placeholder
	placeholders []string
	// dir is the leading part of the pattern that has no wildcard: the only
	// place its files can be. It is the whole path when nothing is wild.
	dir string
}

func compile(text string) pattern {
	var p pattern
	var re strings.Builder
	re.WriteString("^")
	last := 0
	for _, loc := range wildcardRE.FindAllStringIndex(text, -1) {
		re.WriteString(regexp.QuoteMeta(text[last:loc[0]]))
		switch wildcard := text[loc[0]:loc[1]]; wildcard {
		case "**/":
			re.WriteString(`(?:.*/)?`)
		case "**":
			re.WriteString(`.*`)
		case "*":
			re.WriteString(`[^/]*`)
		case "?":
			re.WriteString(`[^/]`)
		default:
			re.WriteString(`([^/]+)`)
			p.placeholders = append(p.placeholders, wildcard)
		}
		last = loc[1]
	}
	re.WriteString(regexp.QuoteMeta(text[last:]) + "$")
	p.re = regexp.MustCompile(re.String())

	segments := strings.Split(text, "/")
	n := 0
	for n < len(segments) && !wildcardRE.MatchString(segments[n]) {
		n++
	}
	p.dir = strings.Join(segments[:n], "/")
	return p
}

// match reports whether a path belongs to the pattern. If so it returns the
// logical path, in which placeholders are kept ("public/locales/{lng}/a.json"),
// and the instance, the values they took ("he").
func (p pattern) match(file string) (logical, instance string, ok bool) {
	m := p.re.FindStringSubmatchIndex(file)
	if m == nil {
		return "", "", false
	}
	var b strings.Builder
	var values []string
	last := 0
	for i, name := range p.placeholders {
		start, end := m[2*i+2], m[2*i+3]
		b.WriteString(file[last:start] + name)
		values = append(values, file[start:end])
		last = end
	}
	b.WriteString(file[last:])
	return b.String(), strings.Join(values, "/"), true
}

// Concrete is the inverse of matching: the path a logical path has in an instance.
func Concrete(logical, instance string) string {
	values := strings.Split(instance, "/")
	i := 0
	return placeholderRE.ReplaceAllStringFunc(logical, func(name string) string {
		if i < len(values) {
			name = values[i]
		}
		i++
		return name
	})
}

// ValidPath accepts only clean, relative, slash-separated paths that stay
// inside the directory they are joined to. Contract paths must pass it: a
// contract is input, and must not be able to make contracter read elsewhere.
func ValidPath(p string) bool {
	return p != "" && p != "." && !path.IsAbs(p) && !strings.Contains(p, `\`) &&
		path.Clean(p) == p && p != ".." && !strings.HasPrefix(p, "../")
}
