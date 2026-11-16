package shape

import "strings"

// The key-path grammar: map keys are joined by separator, and a list's
// elements live under the list's own path followed by element.
// A map key that itself contains a separator is indistinguishable from nesting.
const (
	separator = "."
	element   = "[]"
)

func join(parent, name string) string {
	if parent == "" {
		return name
	}
	return parent + separator + name
}

// Diff is how the keys of a file differ from the keys a contract recorded.
type Diff struct {
	Missing map[string]string    // key -> the type the contract recorded
	Unknown []string             // keys the contract does not have
	Changed map[string][2]string // key -> recorded type, found type
}

// Compare reports how the key types found in a file differ from the recorded
// ones. Types are only compared when typed; keys matching optional may be
// absent or extra.
//
// List length is never compared. Where the contract recorded an empty
// container it accepts any contents, and where it recorded a null or an
// expression it accepts anything at all.
func Compare(recorded, found map[string]string, typed bool, optional Patterns) Diff {
	d := Diff{Missing: map[string]string{}, Changed: map[string][2]string{}}
	for k, want := range recorded {
		got, ok := found[k]
		switch {
		case ok:
			if typed && got != want && !unknown(got) && !unknown(want) {
				d.Changed[k] = [2]string{want, got}
			}
		case optional.Match(k):
		case open(want) && hasBelow(found, k):
		case below(found, k, func(t string) bool { return t == typeEmptyList || t == typeExpression }):
			// The file has no elements, or an expression, where the contract saw keys.
		default:
			d.Missing[k] = want
		}
	}
	for k := range found {
		if _, ok := recorded[k]; ok || optional.Match(k) {
			continue
		}
		// Not unknown if it is inside something the contract left open, or
		// is a container the contract has keys under: those keys are then
		// already reported as missing.
		if below(recorded, k, open) || hasBelow(recorded, k) {
			continue
		}
		d.Unknown = append(d.Unknown, k)
	}
	return d
}

func unknown(typ string) bool { return typ == typeNull || typ == typeExpression }

func open(typ string) bool { return typ == typeEmptyObject || typ == typeEmptyList || unknown(typ) }

func hasBelow(keys map[string]string, k string) bool {
	for other := range keys {
		if strings.HasPrefix(other, k+separator) || strings.HasPrefix(other, k+element) {
			return true
		}
	}
	return false
}

// below reports whether k is nested under a key whose type satisfies is.
func below(keys map[string]string, k string, is func(typ string) bool) bool {
	for i := range k {
		if strings.HasPrefix(k[i:], separator) || strings.HasPrefix(k[i:], element) {
			if t, ok := keys[k[:i]]; ok && is(t) {
				return true
			}
		}
	}
	return false
}
