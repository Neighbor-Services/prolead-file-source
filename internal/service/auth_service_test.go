package service

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gostore/internal/config"
	"gostore/internal/core/domain"
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
	res, err := authSvc.Login(ctx, "sysadmin", "SuperSecurePass123!", "")
	require.NoError(t, err)
	assert.NotEmpty(t, res.Token)
	assert.True(t, res.IsAdmin)
	assert.Equal(t, "sysadmin", res.User.Username)

	// 3. Email login
	resEmail, err := authSvc.Login(ctx, "sysadmin@proleadsolutions.co", "SuperSecurePass123!", "")
	require.NoError(t, err)
	assert.NotEmpty(t, resEmail.Token)

	// 4. Failed Login - Wrong Password
	_, err = authSvc.Login(ctx, "sysadmin", "WrongPassword", "")
	assert.Error(t, err)

	// 5. Failed Login - Master Key Bypass must be rejected
	_, err = authSvc.Login(ctx, "master-admin", "secret-master-api-key", "")
	assert.Error(t, err)

	_, err = authSvc.Login(ctx, "secret-master-api-key", "secret-master-api-key", "")
	assert.Error(t, err)

	// 6. Test 2FA Setup and Enable Flow
	setupRes, err := authSvc.Setup2FA(ctx, admin.ID)
	require.NoError(t, err)
	assert.NotEmpty(t, setupRes.Secret)
	assert.NotEmpty(t, setupRes.OTPAuthURI)

	// Generate valid code with TOTP
	validCode, err := GenerateTOTPCode(setupRes.Secret, time.Now())
	require.NoError(t, err)

	err = authSvc.VerifyAndEnable2FA(ctx, admin.ID, validCode, setupRes.Secret)
	require.NoError(t, err)

	// Login with 2FA enabled but without code -> require 2FA challenge
	res2FAChallenge, err := authSvc.Login(ctx, "sysadmin", "SuperSecurePass123!", "")
	require.NoError(t, err)
	assert.True(t, res2FAChallenge.Require2FA)
	assert.NotEmpty(t, res2FAChallenge.TempToken)
	assert.Empty(t, res2FAChallenge.Token)

	// Complete 2FA login verification
	verifyRes, err := authSvc.Verify2FALogin(ctx, res2FAChallenge.TempToken, validCode)
	require.NoError(t, err)
	assert.NotEmpty(t, verifyRes.Token)
	assert.Equal(t, admin.ID, verifyRes.User.ID)
}

func TestAuthService_StatelessTokenValidationAcrossInstances(t *testing.T) {
	masterKey := "prolead-master-jwt-signing-key-12345"
	authSvc1 := NewAuthService(nil, masterKey)
	authSvc2 := NewAuthService(nil, masterKey) // Fresh instance without shared memory

	user := &domain.User{
		ID:          "usr-99",
		Username:    "admin",
		IsSuperuser: true,
	}

	token := authSvc1.generateSignedToken(user, time.Now().Add(24*time.Hour))
	assert.True(t, strings.HasPrefix(token, "usr_"))

	// Validate on instance 2
	validatedUser := authSvc2.ValidateToken(token)
	require.NotNil(t, validatedUser)
	assert.Equal(t, "usr-99", validatedUser.ID)
	assert.Equal(t, "admin", validatedUser.Username)
	assert.True(t, validatedUser.IsSuperuser)

	// Tampered token must fail
	tampered := token + "bad"
	assert.Nil(t, authSvc2.ValidateToken(tampered))
}

