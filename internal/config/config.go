package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port         string
	Host         string
	BaseURL      string
	PublicURL    string
	StoragePath   string
	MasterAPIKey  string
	EncryptionKey string
	MaxUploadMB   int64

	// Database Configuration (PostgreSQL / SQLite)
	DBType           string // "postgres" or "sqlite"
	DatabaseURL      string
	PostgresHost     string
	PostgresPort     string
	PostgresUser     string
	PostgresPassword string
	PostgresDB       string
	PostgresSSLMode  string
	DatabasePath     string // For SQLite fallback
	Env              string // "production", "staging", "development"
}

func (c *Config) IsProduction() bool {
	env := strings.ToLower(c.Env)
	return env == "production" || env == "prod" || 
		strings.ToLower(os.Getenv("GIN_MODE")) == "release" ||
		strings.ToLower(os.Getenv("APP_ENV")) == "production" ||
		strings.Contains(c.BaseURL, "proleadsolutions.co") ||
		strings.Contains(c.PublicURL, "proleadsolutions.co")
}

func LoadConfig() *Config {
	loadEnvFiles()

	env := getEnv("ENV", getEnv("APP_ENV", "development"))
	port := getEnv("PORT", "8080")
	host := getEnv("HOST", "0.0.0.0")
	
	// Support dynamic/public base URL with preference for APP_URL / PUBLIC_BASE_URL
	publicURL := getEnv("APP_URL", getEnv("PUBLIC_BASE_URL", ""))
	baseURL := getEnv("BASE_URL", publicURL)
	if baseURL == "" {
		baseURL = "" // Empty indicates dynamic request-derived origin
	}

	storagePath := getEnv("STORAGE_PATH", "./data/storage")
	masterKey := getEnv("MASTER_API_KEY", "gostore-master-secret-key")
	encryptionKey := getEnv("ENCRYPTION_KEY", getEnv("STORAGE_ENCRYPTION_KEY", masterKey))

	maxUploadMB, err := strconv.ParseInt(getEnv("MAX_UPLOAD_MB", "500"), 10, 64)
	if err != nil {
		maxUploadMB = 500
	}

	// Database Config
	dbURL := getEnv("DATABASE_URL", "")
	dbType := strings.ToLower(getEnv("DB_TYPE", ""))

	pgHost := getEnv("POSTGRES_HOST", "localhost")
	pgPort := getEnv("POSTGRES_PORT", "5432")
	pgUser := getEnv("POSTGRES_USER", "postgres")
	pgPass := getEnv("POSTGRES_PASSWORD", "postgres")
	pgDB := getEnv("POSTGRES_DB", "gostore")
	pgSSL := getEnv("POSTGRES_SSLMODE", "disable")
	sqlitePath := getEnv("DATABASE_PATH", "./data/gostore.db")

	// In production, PostgreSQL is mandatory
	isProd := env == "production" || env == "prod" || 
		strings.ToLower(os.Getenv("GIN_MODE")) == "release" ||
		strings.Contains(baseURL, "proleadsolutions.co") ||
		strings.Contains(publicURL, "proleadsolutions.co")

	if isProd {
		if dbType == "" {
			dbType = "postgres"
		}
	} else if dbType == "" {
		if dbURL != "" || os.Getenv("POSTGRES_HOST") != "" || os.Getenv("POSTGRES_DB") != "" {
			dbType = "postgres"
		} else {
			dbType = "sqlite"
		}
	}

	return &Config{
		Env:              env,
		Port:             port,
		Host:             host,
		BaseURL:          baseURL,
		PublicURL:        publicURL,
		StoragePath:      storagePath,
		MasterAPIKey:     masterKey,
		EncryptionKey:    encryptionKey,
		MaxUploadMB:      maxUploadMB,
		DBType:           dbType,
		DatabaseURL:      dbURL,
		PostgresHost:     pgHost,
		PostgresPort:     pgPort,
		PostgresUser:     pgUser,
		PostgresPassword: pgPass,
		PostgresDB:       pgDB,
		PostgresSSLMode:  pgSSL,
		DatabasePath:     sqlitePath,
	}
}

// GetPostgresDSN returns the connection string for PostgreSQL
func (c *Config) GetPostgresDSN() string {
	if c.DatabaseURL != "" {
		return c.DatabaseURL
	}
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.PostgresHost, c.PostgresPort, c.PostgresUser, c.PostgresPassword, c.PostgresDB, c.PostgresSSLMode)
}

func getEnv(key, defaultVal string) string {
	if val, exists := os.LookupEnv(key); exists && val != "" {
		return val
	}
	return defaultVal
}

func loadEnvFiles() {
	candidates := []string{
		os.Getenv("ENV_FILE"),
		".env",
		"../.env",
		"../../.env",
		"/opt/gostore/.env",
		"/etc/gostore/.env",
	}

	for _, path := range candidates {
		if path == "" {
			continue
		}
		if data, err := os.ReadFile(path); err == nil {
			parseEnvContent(string(data))
		}
	}
}

func parseEnvContent(content string) {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			k := strings.TrimSpace(parts[0])
			v := strings.TrimSpace(parts[1])
			v = strings.Trim(v, `"'`)
			if _, exists := os.LookupEnv(k); !exists {
				_ = os.Setenv(k, v)
			}
		}
	}
}

