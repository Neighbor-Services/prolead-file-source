package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"gostore/internal/core/domain"
	"gostore/internal/service"
)

type contextKey string

const (
	APIKeyContextKey contextKey = "apiKey"
	UserContextKey   contextKey = "user"
)

func GetAPIKeyFromContext(ctx context.Context) *domain.APIKey {
	if val, ok := ctx.Value(APIKeyContextKey).(*domain.APIKey); ok {
		return val
	}
	return nil
}

func GetUserFromContext(ctx context.Context) *domain.User {
	if val, ok := ctx.Value(UserContextKey).(*domain.User); ok {
		return val
	}
	return nil
}

// CheckBucketPermission verifies APIKey role (read-only vs write) and allowedBuckets whitelist
func CheckBucketPermission(ctx context.Context, bucket string, isWrite bool) error {
	user := GetUserFromContext(ctx)
	if user != nil && user.IsSuperuser {
		return nil
	}

	apiKey := GetAPIKeyFromContext(ctx)
	if apiKey == nil {
		if isWrite {
			return errors.New("unauthorized: write operations require valid authentication credentials")
		}
		// Public or unauthenticated context allowed for reads on public resources
		return nil
	}

	if apiKey.Role == "admin" {
		return nil
	}

	// 1. Check Write Permission
	if isWrite && apiKey.Role == "read-only" {
		return errors.New("forbidden: API key has read-only permissions")
	}

	// 2. Check Allowed Buckets Whitelist
	if len(apiKey.AllowedBuckets) > 0 {
		allowed := false
		for _, b := range apiKey.AllowedBuckets {
			if b == "*" || b == bucket {
				allowed = true
				break
			}
		}
		if !allowed {
			return errors.New("forbidden: API key is not authorized for bucket: " + bucket)
		}
	}

	return nil
}

func ExtractToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}
	if strings.HasPrefix(authHeader, "bearer ") {
		return strings.TrimPrefix(authHeader, "bearer ")
	}

	if key := r.Header.Get("X-API-Key"); key != "" {
		return key
	}

	if key := r.URL.Query().Get("key"); key != "" {
		return key
	}

	return ""
}

func RequireAuth(keyService *service.KeyService, authService *service.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := ExtractToken(r)
			if token == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error": "Unauthorized: Authentication token or API key required"}`))
				return
			}

			// 1. Check User Session
			if authService != nil {
				if user := authService.ValidateToken(token); user != nil {
					role := "read-write"
					if user.IsSuperuser {
						role = "admin"
					}
					syntheticKey := &domain.APIKey{
						ID:        user.ID,
						Name:      user.Username,
						Role:      role,
						CreatedAt: user.CreatedAt,
					}
					ctx := context.WithValue(r.Context(), APIKeyContextKey, syntheticKey)
					ctx = context.WithValue(ctx, UserContextKey, user)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			// 2. Check API Key
			apiKey, err := keyService.ValidateKey(r.Context(), token)
			if err != nil || apiKey == nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error": "Unauthorized: Invalid or revoked credentials"}`))
				return
			}

			ctx := context.WithValue(r.Context(), APIKeyContextKey, apiKey)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func OptionalAuth(keyService *service.KeyService, authService *service.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := ExtractToken(r)
			if token != "" {
				// 1. Check User Session
				if authService != nil {
					if user := authService.ValidateToken(token); user != nil {
						role := "read-write"
						if user.IsSuperuser {
							role = "admin"
						}
						syntheticKey := &domain.APIKey{
							ID:        user.ID,
							Name:      user.Username,
							Role:      role,
							CreatedAt: time.Now(),
						}
						ctx := context.WithValue(r.Context(), APIKeyContextKey, syntheticKey)
						ctx = context.WithValue(ctx, UserContextKey, user)
						next.ServeHTTP(w, r.WithContext(ctx))
						return
					}
				}

				// 2. Check API Key
				if apiKey, err := keyService.ValidateKey(r.Context(), token); err == nil && apiKey != nil {
					ctx := context.WithValue(r.Context(), APIKeyContextKey, apiKey)
					r = r.WithContext(ctx)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := GetAPIKeyFromContext(r.Context())
		user := GetUserFromContext(r.Context())

		isAdmin := (apiKey != nil && apiKey.Role == "admin") || (user != nil && user.IsSuperuser)
		if !isAdmin {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"error": "Forbidden: Superuser or Admin privileges required"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}
