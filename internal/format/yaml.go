package format

import (
	"bytes"
	"io"

	"gopkg.in/yaml.v3"
)

var yamlFormat = Format{Name: "yaml", Typed: true, claims: extension(".yaml", ".yml"), parse: parseYAML}

func parseYAML(data []byte) (any, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var docs []any
	for {
		var doc any
		if err := dec.Decode(&doc); err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		// An empty document, such as the one after a trailing "---", is not
		// part of the config.
		if doc != nil {
			docs = append(docs, canonical(doc))
		}
	}
	if manifests := byKindAndName(docs); manifests != nil {
		return manifests, nil
	}
	if len(docs) == 1 {
		return docs[0], nil
	}
	return docs, nil
}

// byKindAndName keys Kubernetes-style documents as "Kind/name", so that each
// manifest in a file keeps its own shape and a dropped manifest is noticed.
// Documents without that identity can only be told apart by position, so it
// returns nil unless every document has a unique kind and metadata.name.
func byKindAndName(docs []any) map[string]any {
	manifests := map[string]any{}
	for _, doc := range docs {
		m, _ := doc.(map[string]any)
		kind, _ := m["kind"].(string)
		meta, _ := m["metadata"].(map[string]any)
		name, _ := meta["name"].(string)
		id := kind + "/" + name
		if kind == "" || name == "" || manifests[id] != nil {
			return nil
		}
		manifests[id] = m
	}
	if len(manifests) == 0 {
		return nil
	}
	return manifests
}
