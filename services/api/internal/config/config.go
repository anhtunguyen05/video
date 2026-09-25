package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	AppEnv                 string
	HTTPAddr               string
	DatabaseURL            string
	RabbitMQURL            string
	DevUserID              string
	ShutdownTimeout        time.Duration
	S3Endpoint             string
	S3Region               string
	S3AccessKey            string
	S3SecretKey            string
	S3Bucket               string
	S3UsePathStyle         bool
	UploadURLExpiry        time.Duration
	MaxUploadSizeBytes     int64
	AllowedUploadMimeTypes []string
}

func Load() (Config, error) {
	_ = godotenv.Load("../../.env", ".env")
	port := getenv("API_PORT", "8080")
	shutdownSeconds, err := strconv.Atoi(getenv("SHUTDOWN_TIMEOUT_SECONDS", "10"))
	if err != nil || shutdownSeconds <= 0 {
		return Config{}, fmt.Errorf("invalid SHUTDOWN_TIMEOUT_SECONDS")
	}
	pathStyle, err := strconv.ParseBool(getenv("S3_USE_PATH_STYLE", "true"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid S3_USE_PATH_STYLE")
	}
	uploadURLExpiry, err := time.ParseDuration(getenv("UPLOAD_URL_EXPIRY", "15m"))
	if err != nil || uploadURLExpiry <= 0 {
		return Config{}, fmt.Errorf("invalid UPLOAD_URL_EXPIRY")
	}
	maxUploadSizeBytes, err := strconv.ParseInt(getenv("MAX_UPLOAD_SIZE_BYTES", "104857600"), 10, 64)
	if err != nil || maxUploadSizeBytes <= 0 {
		return Config{}, fmt.Errorf("invalid MAX_UPLOAD_SIZE_BYTES")
	}
	allowedUploadMimeTypes := splitCSV(getenv("ALLOWED_UPLOAD_CONTENT_TYPES", "video/mp4,video/webm"))
	if len(allowedUploadMimeTypes) == 0 {
		return Config{}, fmt.Errorf("invalid ALLOWED_UPLOAD_CONTENT_TYPES")
	}
	return Config{
		AppEnv:                 getenv("APP_ENV", "local"),
		HTTPAddr:               ":" + port,
		DatabaseURL:            getenv("DATABASE_URL", "postgres://video:video@localhost:5432/video?sslmode=disable"),
		RabbitMQURL:            getenv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),
		DevUserID:              getenv("DEV_USER_ID", "00000000-0000-4000-8000-000000000001"),
		ShutdownTimeout:        time.Duration(shutdownSeconds) * time.Second,
		S3Endpoint:             getenv("S3_ENDPOINT", "http://localhost:9000"),
		S3Region:               getenv("S3_REGION", "us-east-1"),
		S3AccessKey:            getenv("S3_ACCESS_KEY", "minio"),
		S3SecretKey:            getenv("S3_SECRET_KEY", "minio123"),
		S3Bucket:               getenv("S3_BUCKET", "video-platform"),
		S3UsePathStyle:         pathStyle,
		UploadURLExpiry:        uploadURLExpiry,
		MaxUploadSizeBytes:     maxUploadSizeBytes,
		AllowedUploadMimeTypes: allowedUploadMimeTypes,
	}, nil
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if item := strings.TrimSpace(part); item != "" {
			result = append(result, item)
		}
	}
	return result
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
