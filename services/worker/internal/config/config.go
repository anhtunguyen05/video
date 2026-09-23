package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
	"time"
)

type Config struct {
	AppEnv          string
	DatabaseURL     string
	ShutdownTimeout time.Duration
}

func Load() Config {
	_ = godotenv.Load("../../.env", ".env")
	seconds, err := strconv.Atoi(getenv("SHUTDOWN_TIMEOUT_SECONDS", "10"))
	if err != nil || seconds <= 0 {
		seconds = 10
	}
	return Config{
		AppEnv:          getenv("APP_ENV", "local"),
		DatabaseURL:     getenv("DATABASE_URL", "postgres://video:video@localhost:5432/video?sslmode=disable"),
		ShutdownTimeout: time.Duration(seconds) * time.Second,
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
