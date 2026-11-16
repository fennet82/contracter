package format

import (
	"bytes"
	"encoding/xml"
	"strings"
)

// An element that occurs once and one that repeats are the same key, hence
// flatLists.
var xmlFormat = Format{Name: "xml", flatLists: true, claims: extension(".xml"), parse: parseXML}

// parseXML maps elements to keys, attributes to "@name" and text beside
// child elements to "#text".
func parseXML(data []byte) (any, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err // io.EOF here means there is no root element
		}
		if start, ok := tok.(xml.StartElement); ok {
			root, err := xmlElement(dec, start)
			return map[string]any{start.Name.Local: root}, err
		}
	}
}

func xmlElement(dec *xml.Decoder, start xml.StartElement) (any, error) {
	node := map[string]any{}
	for _, attr := range start.Attr {
		node["@"+attr.Name.Local] = attr.Value
	}
	var text strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			child, err := xmlElement(dec, t)
			if err != nil {
				return nil, err
			}
			name := t.Name.Local
			switch seen := node[name].(type) {
			case nil:
				node[name] = child
			case []any:
				node[name] = append(seen, child)
			default:
				node[name] = []any{seen, child}
			}
		case xml.CharData:
			text.Write(t)
		case xml.EndElement:
			content := strings.TrimSpace(text.String())
			if len(node) == 0 {
				return content, nil
			}
			if content != "" {
				node["#text"] = content
			}
			return node, nil
		}
	}
}
