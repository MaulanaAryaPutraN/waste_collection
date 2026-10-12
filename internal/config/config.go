package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	AppPort     string
	DatabaseURL string
	UploadDir   string
	MaxUploadMB int64
	Shutdown    time.Duration
}

func Load() (Config, error) {
	_ = godotenv.Load()
	port := env("APP_PORT", "8080")
	maxMB, err := strconv.ParseInt(env("MAX_UPLOAD_MB", "5"), 10, 64)
	if err != nil || maxMB < 1 {
		return Config{}, fmt.Errorf("MAX_UPLOAD_MB must be a positive integer")
	}
	shutdownSeconds, err := strconv.Atoi(env("SHUTDOWN_TIMEOUT_SECONDS", "10"))
	if err != nil || shutdownSeconds < 1 {
		return Config{}, fmt.Errorf("SHUTDOWN_TIMEOUT_SECONDS must be a positive integer")
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		connectionURL := url.URL{
			Scheme: "postgres",
			User:   url.UserPassword(env("DB_USER", "postgres"), env("DB_PASSWORD", "postgres")),
			Host:   net.JoinHostPort(env("DB_HOST", "localhost"), env("DB_PORT", "5432")),
			Path:   env("DB_NAME", "waste_collection"),
		}
		query := url.Values{}
		query.Set("sslmode", env("DB_SSLMODE", "disable"))
		connectionURL.RawQuery = query.Encode()
		dbURL = connectionURL.String()
	}
	return Config{
		AppPort: port, DatabaseURL: dbURL, UploadDir: env("UPLOAD_DIR", "uploads"),
		MaxUploadMB: maxMB, Shutdown: time.Duration(shutdownSeconds) * time.Second,
	}, nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
