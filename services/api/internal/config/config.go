package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	AppEnv          string
	HTTPAddr        string
	DatabaseURL     string
	DevUserID       string
	ShutdownTimeout time.Duration
}

func Load() (Config, error) {
	_ = godotenv.Load("../../.env", ".env")
	port := getenv("API_PORT", "8080")
	shutdownSeconds, err := strconv.Atoi(getenv("SHUTDOWN_TIMEOUT_SECONDS", "10"))
	if err != nil || shutdownSeconds <= 0 {
		return Config{}, fmt.Errorf("invalid SHUTDOWN_TIMEOUT_SECONDS")
	}
	return Config{
		AppEnv:          getenv("APP_ENV", "local"),
		HTTPAddr:        ":" + port,
		DatabaseURL:     getenv("DATABASE_URL", "postgres://video:video@localhost:5432/video?sslmode=disable"),
		DevUserID:       getenv("DEV_USER_ID", "00000000-0000-4000-8000-000000000001"),
		ShutdownTimeout: time.Duration(shutdownSeconds) * time.Second,
	}, nil
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
