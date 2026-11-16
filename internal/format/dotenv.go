package format

import (
	"strings"

	"github.com/joho/godotenv"
)

var dotenvFormat = Format{Name: "dotenv", claims: isDotenv, parse: parseDotenv}

func isDotenv(name string) bool {
	return strings.HasPrefix(name, ".env.") || strings.HasSuffix(name, ".env")
}

func parseDotenv(data []byte) (any, error) {
	m, err := godotenv.UnmarshalBytes(data)
	return stringMap(m), err
}
