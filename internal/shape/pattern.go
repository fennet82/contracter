package shape

import (
	"regexp"
	"strings"
)

// Pattern selects keys: "db.host", "debug.*", "module.*.source". "*" matches
// any run of characters, and a pattern also selects everything nested under
// the keys it matches.
type Pattern struct {
	text string
	re   *regexp.Regexp
}

func (p *Pattern) UnmarshalText(text []byte) error {
	p.text = string(text)
	p.re = regexp.MustCompile(`^` + strings.ReplaceAll(regexp.QuoteMeta(p.text), `\*`, `.*`) +
		`(?:$|` + regexp.QuoteMeta(separator) + `|` + regexp.QuoteMeta(element) + `)`)
	return nil
}

func (p Pattern) MarshalText() ([]byte, error) { return []byte(p.text), nil }

func (p Pattern) String() string { return p.text }

func (p Pattern) Match(key string) bool { return p.re != nil && p.re.MatchString(key) }

type Patterns []Pattern

func (ps Patterns) Match(key string) bool {
	for _, p := range ps {
		if p.Match(key) {
			return true
		}
	}
	return false
}
