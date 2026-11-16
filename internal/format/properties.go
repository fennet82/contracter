package format

import "github.com/magiconair/properties"

var propertiesFormat = Format{Name: "properties", claims: extension(".properties"), parse: parseProperties}

func parseProperties(data []byte) (any, error) {
	// Expansion is off because "${...}" may refer to things only the
	// application can resolve, as in Spring files.
	p, err := (&properties.Loader{Encoding: properties.UTF8, DisableExpansion: true}).LoadBytes(data)
	if err != nil {
		return nil, err
	}
	return stringMap(p.Map()), nil
}
