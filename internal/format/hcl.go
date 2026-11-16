package format

import (
	"encoding/json"
	"strings"

	"github.com/tmccombs/hcl2json/convert"

	"contracter/internal/shape"
)

// HCL blocks come out of the converter as lists even when there is only one,
// hence flatLists: "variable.region.default", not "variable.region[].default".
var hclFormat = Format{Name: "hcl", Typed: true, flatLists: true, claims: extension(".tf", ".tfvars", ".hcl"), parse: parseHCL}

func parseHCL(data []byte) (any, error) {
	converted, err := convert.Bytes(data, "config.hcl", convert.Options{})
	if err != nil {
		return nil, err
	}
	var v any
	if err := json.Unmarshal(converted, &v); err != nil {
		return nil, err
	}
	return markExpressions(v), nil
}

// markExpressions finds the non-literal values, which the converter renders
// as "${...}" strings.
func markExpressions(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			t[k] = markExpressions(child)
		}
	case []any:
		for i, child := range t {
			t[i] = markExpressions(child)
		}
	case string:
		if strings.HasPrefix(t, "${") && strings.HasSuffix(t, "}") {
			return shape.Expr(t)
		}
	}
	return v
}
