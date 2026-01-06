package internal

import (
	"os"
	"runtime"
	"strconv"
)

type Config struct {
	Quality int
	DBFile  string
	Workers int
}

func LoadConfig() *Config {
	return &Config{
		Quality: getEnvInt("QUALITY", 80),
		DBFile:  getEnv("DB_FILE", "./cache.gob"),
		Workers: getEnvInt("WORKERS", runtime.NumCPU()),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	value := getEnv(key, "")
	if value == "" {
		return defaultValue
	}
	if intValue, err := strconv.Atoi(value); err == nil {
		return intValue
	}
	return defaultValue
}
