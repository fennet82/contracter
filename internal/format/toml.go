package format

import "github.com/BurntSushi/toml"

var tomlFormat = Format{Name: "toml", Typed: true, claims: extension(".toml"), parse: parseTOML}

func parseTOML(data []byte) (any, error) {
	var v map[string]any
	err := toml.Unmarshal(data, &v)
	return canonical(v), err
}
