package config

import (
	"fmt"
	"os"
)

type envConfig struct {
	Port             string
	PostgresqlUrl    string
	JWTSecret        string
	NominatimContact string // contact email for the geocoder User-Agent; empty disables geocoding
}

var Env envConfig

// getEnv returns the env var or panics if unset (required config).
func getEnv(name string) string {
	v, ok := os.LookupEnv(name)
	if !ok {
		panic(fmt.Sprintf("%s must be set", name))
	}
	return v
}

// getOptEnv returns the env var or def if unset (optional config).
func getOptEnv(name, def string) string {
	if v, ok := os.LookupEnv(name); ok {
		return v
	}
	return def
}

// InitConfig loads config from environment variables.
func InitConfig() {
	Env = envConfig{
		Port:             getOptEnv("PORT", "8080"),
		PostgresqlUrl:    getEnv("POSTGRESQL_URL"),
		JWTSecret:        getEnv("JWT_SECRET"),
		NominatimContact: getOptEnv("NOMINATIM_CONTACT", ""),
	}
}
