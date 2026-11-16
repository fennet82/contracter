// Package shape models what a contract knows about the content of one config
// file: its key paths, the type at each, and the scalar values found there.
//
// It owns the key-path grammar ("db.host", "servers[].name"); nothing outside
// this package takes key paths apart.
package shape

import (
	"fmt"
	"time"
)

// Types a key can have in a contract.
const (
	typeString   = "string"
	typeNumber   = "number"
	typeBool     = "bool"
	typeDatetime = "datetime"

	// Containers are only recorded when empty; a filled one is described by
	// the keys under it.
	typeEmptyObject = "object"
	typeEmptyList   = "list"

	// A key of these types exists, but what it holds is not known.
	typeNull       = "null"
	typeExpression = "expression"
)

// Expr is a value that is not a literal, such as a Terraform reference. A
// parser wraps such values so that they get the expression type.
type Expr string

// Leaf is one key path of a file.
type Leaf struct {
	Type   string
	Values []string // every scalar found there; several when under a list
}

// Leaves maps the key paths of one file to what was found at each.
type Leaves map[string]*Leaf

func (l Leaves) Types() map[string]string {
	types := make(map[string]string, len(l))
	for k, leaf := range l {
		types[k] = leaf.Type
	}
	return types
}

// Flatten turns a tree of map[string]any, []any and scalars into key paths.
// Without markLists, list elements share their list's key path; formats use
// that when a "list" is just a repeated block or element.
func Flatten(tree any, markLists bool) Leaves {
	marker := ""
	if markLists {
		marker = element
	}
	l := Leaves{}
	l.add("", tree, marker)
	return l
}

func (l Leaves) add(key string, v any, marker string) {
	switch t := v.(type) {
	case map[string]any:
		// An empty file has no keys at all; only nested containers are
		// recorded as empty.
		if len(t) == 0 && key != "" {
			l.leaf(key, typeEmptyObject)
		}
		for name, child := range t {
			l.add(join(key, name), child, marker)
		}
	case []any:
		if len(t) == 0 && key != "" {
			l.leaf(key, typeEmptyList)
		}
		for _, child := range t {
			l.add(key+marker, child, marker)
		}
	default:
		leaf := l.leaf(key, typeOf(v))
		value := ""
		if v != nil {
			value = fmt.Sprint(v)
		}
		leaf.Values = append(leaf.Values, value)
	}
}

func (l Leaves) leaf(key, typ string) *Leaf {
	leaf := l[key]
	if leaf == nil {
		leaf = &Leaf{Type: typ}
		l[key] = leaf
	}
	leaf.Type = refine(leaf.Type, typ)
	return leaf
}

func typeOf(v any) string {
	switch v.(type) {
	case nil:
		return typeNull
	case bool:
		return typeBool
	case Expr:
		return typeExpression
	case int, int64, uint64, float64:
		return typeNumber
	case time.Time:
		return typeDatetime
	}
	return typeString
}

// Union adds the key types in src to dst.
func Union(dst, src map[string]string) {
	for k, t := range src {
		if old, ok := dst[k]; ok {
			t = refine(old, t)
		}
		dst[k] = t
	}
}

// refine picks the type to keep when a key is seen twice: the first one that
// says something. A list mixing concrete types keeps the first.
func refine(old, next string) string {
	if unknown(old) {
		return next
	}
	return old
}
