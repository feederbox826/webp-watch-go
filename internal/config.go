package internal

import (
	"os"
	"runtime"
	"strconv"
)

type Config struct {
	Quality      int
	VideoQuality int
	DBFile       string
	SkipDB       bool
	Workers      int
}

func LoadConfig() *Config {
	numCPU := runtime.NumCPU()
	var defaultWorkers int
	// now with webm, don't scale past cores
	defaultWorkers = numCPU

	return &Config{
		Quality:      getEnvInt("QUALITY", 80),
		VideoQuality: getEnvInt("VIDEO_QUALITY", 28),
		DBFile:       getEnv("DB_FILE", "./cache.gob"),
		SkipDB:       getEnv("SKIP_DB", "false") == "true",
		Workers:      getEnvInt("WORKERS", defaultWorkers),
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
