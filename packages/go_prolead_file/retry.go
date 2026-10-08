package proleadfile

import (
	"math"
	"math/rand"
	"net/http"
	"time"
)

type retryTransport struct {
	base       http.RoundTripper
	maxRetries int
	backoff    time.Duration
}

func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var err error

	for attempt := 0; attempt <= t.maxRetries; attempt++ {
		if attempt > 0 {
			// Rewind body if present
			if req.GetBody != nil {
				body, err := req.GetBody()
				if err != nil {
					return nil, err
				}
				req.Body = body
			}

			// Exponential backoff + jitter
			backoffMs := float64(t.backoff.Milliseconds()) * math.Pow(2, float64(attempt-1))
			jitterMs := float64(rand.Intn(100))
			time.Sleep(time.Duration(backoffMs+jitterMs) * time.Millisecond)
		}

		resp, err = t.base.RoundTrip(req)
		if err == nil && resp.StatusCode < 500 {
			return resp, nil
		}

		if resp != nil && resp.StatusCode >= 500 && attempt < t.maxRetries {
			resp.Body.Close()
		}
	}

	return resp, err
}

// WithRetry configures automated exponential backoff and retry for the Client.
func WithRetry(maxRetries int, backoff time.Duration) func(*Client) {
	return func(c *Client) {
		base := c.httpClient.Transport
		if base == nil {
			base = http.DefaultTransport
		}
		c.httpClient.Transport = &retryTransport{
			base:       base,
			maxRetries: maxRetries,
			backoff:    backoff,
		}
	}
}
