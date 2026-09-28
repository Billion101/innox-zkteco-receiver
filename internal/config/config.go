package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port            string
	DBDSN           string
	Timezone        string
	DebounceMinutes int
}

func Load() *Config {
	port := getEnv("PORT", "8087")
	dbDSN := getEnv("DB_DSN", "")
	tz := getEnv("TZ", "Asia/Vientiane")

	debounceMin := 5
	if val := os.Getenv("DEBOUNCE_MINUTES"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
			debounceMin = parsed
		}
	}

	return &Config{
		Port:            port,
		DBDSN:           dbDSN,
		Timezone:        tz,
		DebounceMinutes: debounceMin,
	}
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultVal
}
