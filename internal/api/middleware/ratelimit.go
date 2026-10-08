package middleware

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type IPRateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*clientLimiter
	rate     rate.Limit
	burst    int
}

type clientLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func NewIPRateLimiter(r rate.Limit, b int) *IPRateLimiter {
	i := &IPRateLimiter{
		limiters: make(map[string]*clientLimiter),
		rate:     r,
		burst:    b,
	}

	// Periodically clean up inactive clients every 5 minutes
	go func() {
		for {
			time.Sleep(5 * time.Minute)
			i.mu.Lock()
			for ip, cl := range i.limiters {
				if time.Since(cl.lastSeen) > 10*time.Minute {
					delete(i.limiters, ip)
				}
			}
			i.mu.Unlock()
		}
	}()

	return i
}

func (i *IPRateLimiter) GetLimiter(key string) *rate.Limiter {
	i.mu.Lock()
	defer i.mu.Unlock()

	cl, exists := i.limiters[key]
	if !exists {
		limiter := rate.NewLimiter(i.rate, i.burst)
		i.limiters[key] = &clientLimiter{limiter: limiter, lastSeen: time.Now()}
		return limiter
	}

	cl.lastSeen = time.Now()
	return cl.limiter
}

func RateLimitMiddleware(limiter *IPRateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract client identifier (API Key if present, otherwise remote IP)
			var clientID string
			if apiKey := ExtractToken(r); apiKey != "" {
				clientID = "key:" + apiKey
			} else {
				ip, _, err := net.SplitHostPort(r.RemoteAddr)
				if err != nil {
					ip = r.RemoteAddr
				}
				clientID = "ip:" + ip
			}

			lim := limiter.GetLimiter(clientID)
			if !lim.Allow() {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"error": map[string]interface{}{
						"code":    http.StatusTooManyRequests,
						"message": "Too Many Requests: Rate limit exceeded",
					},
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// HotlinkProtection verifies that media requests come from allowed Referer/Origin headers
func HotlinkProtection(allowedDomains []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(allowedDomains) == 0 {
				next.ServeHTTP(w, r)
				return
			}

			referer := r.Header.Get("Referer")
			origin := r.Header.Get("Origin")

			// Allow direct downloads (cURL, mobile apps, direct browser address bar)
			if referer == "" && origin == "" {
				next.ServeHTTP(w, r)
				return
			}

			matched := false
			for _, domain := range allowedDomains {
				if domain == "*" || strings.Contains(referer, domain) || strings.Contains(origin, domain) {
					matched = true
					break
				}
			}

			if !matched {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"error": map[string]interface{}{
						"code":    http.StatusForbidden,
						"message": "Forbidden: Hotlinking blocked",
					},
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
