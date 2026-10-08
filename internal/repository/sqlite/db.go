package sqlite

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"gostore/internal/config"
	"gostore/internal/core/domain"
)

type DB struct {
	*gorm.DB
	DriverName string
}

// NewDatabase initializes a database connection based on the provided configuration.
// In production, PostgreSQL is strictly required. SQLite is restricted to development & testing.
func NewDatabase(cfg *config.Config) (*DB, error) {
	if cfg.IsProduction() {
		if strings.ToLower(cfg.DBType) == "sqlite" {
			return nil, fmt.Errorf("FATAL ERROR: SQLite is disabled in production environments. Please configure PostgreSQL via DB_TYPE=postgres, POSTGRES_HOST / DATABASE_URL")
		}
		dsn := cfg.GetPostgresDSN()
		log.Printf("[DATABASE] Connecting to Production PostgreSQL database: %s (host=%s, db=%s)", dsnSafe(cfg), cfg.PostgresHost, cfg.PostgresDB)
		db, err := NewPostgresDB(dsn)
		if err != nil {
			return nil, fmt.Errorf("FATAL ERROR: Failed to connect to PostgreSQL in production: %w. SQLite fallback is strictly prohibited in production", err)
		}
		return db, nil
	}

	// Development / Testing environment
	if cfg.DBType == "postgres" || cfg.DatabaseURL != "" {
		dsn := cfg.GetPostgresDSN()
		log.Printf("[DATABASE] Connecting to PostgreSQL database: %s (host=%s, db=%s)", dsnSafe(cfg), cfg.PostgresHost, cfg.PostgresDB)
		db, err := NewPostgresDB(dsn)
		if err != nil {
			log.Printf("[DATABASE WARNING] Failed to connect to PostgreSQL (%v). Falling back to local SQLite for development at %s", err, cfg.DatabasePath)
			return NewDB(cfg.DatabasePath)
		}
		return db, nil
	}

	return NewDB(cfg.DatabasePath)
}

// NewPostgresDB initializes a connection to a PostgreSQL database using GORM.
func NewPostgresDB(dsn string) (*DB, error) {
	gormDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open PostgreSQL database with GORM: %w", err)
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(time.Hour)

	db := &DB{DB: gormDB, DriverName: "postgres"}
	if err := db.migrate(); err != nil {
		return nil, fmt.Errorf("database migration failed: %w", err)
	}

	return db, nil
}

// NewDB initializes a connection to an embedded SQLite database using GORM.
func NewDB(dbPath string) (*DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	dsn := fmt.Sprintf("%s?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)", dbPath)

	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database with GORM: %w", err)
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(time.Hour)

	db := &DB{DB: gormDB, DriverName: "sqlite"}
	if err := db.migrate(); err != nil {
		return nil, fmt.Errorf("database migration failed: %w", err)
	}

	return db, nil
}

func (db *DB) migrate() error {
	if err := db.AutoMigrate(
		&domain.Project{},
		&domain.User{},
		&domain.Bucket{},
		&domain.FileObject{},
		&domain.FileVersion{},
		&domain.Webhook{},
		&domain.WebhookDelivery{},
		&domain.APIKey{},
		&domain.AuditLog{},
		&domain.ShareLink{},
		&domain.LifecycleRule{},
		&domain.WorkerJobRecord{},
	); err != nil {
		return err
	}

	// Seed Default Project if not present
	var projCount int64
	db.Model(&domain.Project{}).Where("slug = ?", "default").Count(&projCount)
	if projCount == 0 {
		defaultProj := domain.Project{
			ID:          "p-default",
			Name:        "Default Project",
			Slug:        "default",
			Description: "Main production workspace",
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		}
		if err := db.Create(&defaultProj).Error; err != nil {
			return err
		}
	}

	// Seed Default Bucket if not present
	var count int64
	db.Model(&domain.Bucket{}).Where("name = ?", "default").Count(&count)
	if count == 0 {
		defaultBucket := domain.Bucket{
			ID:          "b-default",
			ProjectID:   "p-default",
			Name:        "default",
			Description: "Default storage bucket",
			IsPublic:    true,
			MaxFileSize: 0,
			AllowedMimes: domain.StringList{},
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		}
		if err := db.Create(&defaultBucket).Error; err != nil {
			return err
		}
	}

	return nil
}

func dsnSafe(cfg *config.Config) string {
	if cfg.DatabaseURL != "" {
		return "DATABASE_URL (set)"
	}
	return fmt.Sprintf("postgres://%s:***@%s:%s/%s", cfg.PostgresUser, cfg.PostgresHost, cfg.PostgresPort, cfg.PostgresDB)
}
