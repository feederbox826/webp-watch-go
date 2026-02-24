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
	if numCPU == 1 { // 4 workers on single core
		defaultWorkers = 4
	} else { // otherwise scale to 2x cores
		defaultWorkers = numCPU * 2
	}

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
