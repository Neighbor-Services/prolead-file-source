package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	RequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gostore_http_requests_total",
			Help: "Total number of HTTP requests processed by GoStore",
		},
		[]string{"method", "path", "status"},
	)

	RequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gostore_http_request_duration_seconds",
			Help:    "Histogram of response latency for HTTP requests",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	UploadedBytes = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "gostore_uploaded_bytes_total",
			Help: "Total bytes of files uploaded to GoStore",
		},
	)

	DownloadedBytes = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "gostore_downloaded_bytes_total",
			Help: "Total bytes of files downloaded from GoStore",
		},
	)

	ActiveSSEConnections = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "gostore_active_sse_connections",
			Help: "Current active Server-Sent Events (SSE) connections",
		},
	)
)

type responseWriterWrapper struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
}

func (w *responseWriterWrapper) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *responseWriterWrapper) Write(b []byte) (int, error) {
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytesWritten += int64(n)
	return n, err
}

func (w *responseWriterWrapper) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapper := &responseWriterWrapper{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapper, r)

		duration := time.Since(start).Seconds()
		statusStr := strconv.Itoa(wrapper.statusCode)

		// Record Metrics
		RequestsTotal.WithLabelValues(r.Method, r.URL.Path, statusStr).Inc()
		RequestDuration.WithLabelValues(r.Method, r.URL.Path).Observe(duration)

		if wrapper.bytesWritten > 0 {
			DownloadedBytes.Add(float64(wrapper.bytesWritten))
		}
	})
}
