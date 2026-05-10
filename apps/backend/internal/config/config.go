package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// parseBool returns true for "true", "1", "yes" (case-insensitive), false otherwise.
func parseBool(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "yes":
		return true
	default:
		return false
	}
}

type Config struct {
	Port      string
	DBHost    string
	DBPort    string
	DBUser    string
	DBPass    string
	DBName    string
	JWTSecret string

	RedisAddr     string
	RedisPassword string
	RedisDB       int

	RegistrationEnabled bool
}

func Load() *Config {
	jwtSecret := getSecret("JWT_SECRET", "jwt_secret", "")
	if jwtSecret == "" {
		// Generate a random secret for dev — warn loudly
		b := make([]byte, 32)
		_, _ = rand.Read(b)
		jwtSecret = hex.EncodeToString(b)
		slog.Warn("JWT_SECRET not set, using random secret -- set JWT_SECRET in .env or Docker secret for persistent sessions")
	}

	return &Config{
		Port:      getEnv("PORT", "8080"),
		DBHost:    getEnv("DB_HOST", "localhost"),
		DBPort:    getEnv("DB_PORT", "5432"),
		DBUser:    getEnv("DB_USER", "homelab_game"),
		DBPass:    getSecret("DB_PASSWORD", "db_password", ""),
		DBName:    getEnv("DB_NAME", "homelab_game"),
		JWTSecret: jwtSecret,

		RedisAddr:     getEnv("REDIS_ADDR", "redis:6379"),
		RedisPassword: getSecret("REDIS_PASSWORD", "redis_password", ""),
		RedisDB:       0,

		RegistrationEnabled: parseBool(getEnv("REGISTRATION_ENABLED", "true")),
	}
}

func (c *Config) DatabaseURL() string {
	sslmode := getEnv("DB_SSLMODE", "disable")
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		c.DBUser, c.DBPass, c.DBHost, c.DBPort, c.DBName, sslmode)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// getSecret reads a value from an env var, falling back to a Docker secret
// file at /run/secrets/<name>, then to the fallback value.
func getSecret(envKey, secretName, fallback string) string {
	if v := os.Getenv(envKey); v != "" {
		return v
	}
	if data, err := os.ReadFile("/run/secrets/" + secretName); err == nil {
		return strings.TrimSpace(string(data))
	}
	return fallback
}
