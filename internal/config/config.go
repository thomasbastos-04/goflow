package config

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	Env            string
	HTTPPort       string
	DatabaseURL    string
	RedisAddr      string
	RabbitMQURL    string
	JWTSecret      string
	AccessTokenTTL time.Duration
}

func Load() (Config, error) {
	ttl, err := time.ParseDuration(env("ACCESS_TOKEN_TTL", "24h"))
	if err != nil {
		return Config{}, fmt.Errorf("ACCESS_TOKEN_TTL: %w", err)
	}
	cfg := Config{
		Env: env("APP_ENV", "development"),
		HTTPPort: env("HTTP_PORT", "8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		RedisAddr: env("REDIS_ADDR", "localhost:6379"),
		RabbitMQURL: os.Getenv("RABBITMQ_URL"),
		JWTSecret: os.Getenv("JWT_SECRET"),
		AccessTokenTTL: ttl,
	}
	if cfg.DatabaseURL == "" || cfg.RabbitMQURL == "" || cfg.JWTSecret == "" {
		return Config{}, fmt.Errorf("DATABASE_URL, RABBITMQ_URL and JWT_SECRET are required")
	}
	return cfg, nil
}

func env(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}
