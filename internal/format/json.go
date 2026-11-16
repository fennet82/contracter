package format

import "encoding/json"

var jsonFormat = Format{Name: "json", Typed: true, claims: extension(".json"), parse: parseJSON}

func parseJSON(data []byte) (any, error) {
	var v any
	err := json.Unmarshal(data, &v)
	return v, err
}
