package config

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	AppEnv            string
	DatabaseURL       string
	RabbitMQURL       string
	S3Endpoint        string
	S3Region          string
	S3AccessKey       string
	S3SecretKey       string
	S3Bucket          string
	S3UsePathStyle    bool
	FFProbePath       string
	FFmpegPath        string
	ThumbnailMaxEdge  int
	ProcessingTempDir string
	ShutdownTimeout   time.Duration
}

func Load() Config {
	_ = godotenv.Load("../../.env", ".env")
	seconds, err := strconv.Atoi(getenv("SHUTDOWN_TIMEOUT_SECONDS", "10"))
	if err != nil || seconds <= 0 {
		seconds = 10
	}
	pathStyle, err := strconv.ParseBool(getenv("S3_USE_PATH_STYLE", "true"))
	if err != nil {
		pathStyle = true
	}
	return Config{
		AppEnv:            getenv("APP_ENV", "local"),
		DatabaseURL:       getenv("DATABASE_URL", "postgres://video:video@localhost:5432/video?sslmode=disable"),
		RabbitMQURL:       getenv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),
		S3Endpoint:        getenv("S3_ENDPOINT", "http://localhost:9000"),
		S3Region:          getenv("S3_REGION", "us-east-1"),
		S3AccessKey:       getenv("S3_ACCESS_KEY", "minio"),
		S3SecretKey:       getenv("S3_SECRET_KEY", "minio123"),
		S3Bucket:          getenv("S3_BUCKET", "video-platform"),
		S3UsePathStyle:    pathStyle,
		FFProbePath:       getenv("FFPROBE_PATH", "ffprobe"),
		FFmpegPath:        getenv("FFMPEG_PATH", "ffmpeg"),
		ThumbnailMaxEdge:  positiveInt("THUMBNAIL_MAX_EDGE", 640),
		ProcessingTempDir: getenv("PROCESSING_TEMP_DIR", ""),
		ShutdownTimeout:   time.Duration(seconds) * time.Second,
	}
}

func positiveInt(key string, fallback int) int {
	value, err := strconv.Atoi(getenv(key, strconv.Itoa(fallback)))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
