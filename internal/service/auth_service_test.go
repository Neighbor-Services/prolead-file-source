package service

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gostore/internal/config"
	"gostore/internal/repository/sqlite"
)

func TestAuthService_StrictSuperuserLogin(t *testing.T) {
	dbPath := "/tmp/test_gostore_auth.db"
	_ = os.Remove(dbPath)
	defer os.Remove(dbPath)

	cfg := &config.Config{
		DBType:       "sqlite",
		DatabasePath: dbPath,
		MasterAPIKey: "secret-master-api-key",
	}

	db, err := sqlite.NewDatabase(cfg)
	require.NoError(t, err)

	userRepo := sqlite.NewUserRepository(db)
	authSvc := NewAuthService(userRepo, cfg.MasterAPIKey)
	ctx := context.Background()

	// 1. Create Superuser
	admin, err := authSvc.CreateSuperuser(ctx, "sysadmin", "sysadmin@proleadsolutions.co", "SuperSecurePass123!")
	require.NoError(t, err)
	assert.True(t, admin.IsSuperuser)

	// 2. Successful Login
	res, err := authSvc.Login(ctx, "sysadmin", "SuperSecurePass123!")
	require.NoError(t, err)
	assert.NotEmpty(t, res.Token)
	assert.True(t, res.IsAdmin)
	assert.Equal(t, "sysadmin", res.User.Username)

	// 3. Email login
	resEmail, err := authSvc.Login(ctx, "sysadmin@proleadsolutions.co", "SuperSecurePass123!")
	require.NoError(t, err)
	assert.NotEmpty(t, resEmail.Token)

	// 4. Failed Login - Wrong Password
	_, err = authSvc.Login(ctx, "sysadmin", "WrongPassword")
	assert.Error(t, err)

	// 5. Failed Login - Master Key Bypass must be rejected
	_, err = authSvc.Login(ctx, "master-admin", "secret-master-api-key")
	assert.Error(t, err)

	_, err = authSvc.Login(ctx, "secret-master-api-key", "secret-master-api-key")
	assert.Error(t, err)
}
