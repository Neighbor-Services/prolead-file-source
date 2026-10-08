package sqlite

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gostore/internal/config"
)

func TestNewDatabase_ProductionStrictPostgresEnforcement(t *testing.T) {
	// 1. Production with SQLite explicitly requested -> MUST FAIL
	prodSQLiteCfg := &config.Config{
		Env:          "production",
		DBType:       "sqlite",
		DatabasePath: "/tmp/never_created.db",
	}
	db, err := NewDatabase(prodSQLiteCfg)
	assert.Error(t, err)
	assert.Nil(t, db)
	assert.Contains(t, err.Error(), "SQLite is disabled in production")

	// 2. Production with unreachable Postgres -> MUST FAIL, NEVER FALL BACK TO SQLITE
	prodPostgresCfg := &config.Config{
		Env:              "production",
		DBType:           "postgres",
		PostgresHost:     "127.0.0.1",
		PostgresPort:     "59999", // closed port
		PostgresUser:     "invalid_user",
		PostgresPassword: "invalid_pass",
		PostgresDB:       "invalid_db",
		PostgresSSLMode:  "disable",
		DatabasePath:     "/tmp/should_never_fallback.db",
	}
	db, err = NewDatabase(prodPostgresCfg)
	assert.Error(t, err)
	assert.Nil(t, db)
	assert.Contains(t, err.Error(), "SQLite fallback is strictly prohibited in production")

	// 3. Development with SQLite -> SUCCESS
	devSQLitePath := "/tmp/dev_test_gostore.db"
	_ = os.Remove(devSQLitePath)
	defer os.Remove(devSQLitePath)

	devCfg := &config.Config{
		Env:          "development",
		DBType:       "sqlite",
		DatabasePath: devSQLitePath,
	}
	db, err = NewDatabase(devCfg)
	require.NoError(t, err)
	assert.NotNil(t, db)
	assert.Equal(t, "sqlite", db.DriverName)
}
