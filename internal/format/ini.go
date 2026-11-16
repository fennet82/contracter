package format

import "gopkg.in/ini.v1"

var iniFormat = Format{Name: "ini", claims: extension(".ini", ".cfg"), parse: parseINI}

func parseINI(data []byte) (any, error) {
	// Boolean keys: my.cnf-style files have keys without a value.
	f, err := ini.LoadSources(ini.LoadOptions{AllowBooleanKeys: true}, data)
	if err != nil {
		return nil, err
	}
	root := map[string]any{}
	for _, section := range f.Sections() {
		if section.Name() == ini.DefaultSection {
			// Keys before the first section header are top-level keys.
			for k, v := range section.KeysHash() {
				root[k] = v
			}
		} else {
			root[section.Name()] = stringMap(section.KeysHash())
		}
	}
	return root, nil
}
