package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/time/rate"
	"gostore/internal/api/handlers"
	"gostore/internal/api/middleware"
	"gostore/internal/config"
	"gostore/internal/repository/sqlite"
	"gostore/internal/service"
	"gostore/internal/web"
)

func NewRouter(
	cfg *config.Config,
	storageService *service.StorageService,
	bucketService *service.BucketService,
	keyService *service.KeyService,
	chunkService *service.ChunkService,
	gcService *service.GarbageCollector,
	zipStreamer *service.ZipStreamer,
	webhookService *service.WebhookService,
	auditService *service.AuditService,
	backupService *service.BackupService,
	authService *service.AuthService,
	projectService *service.ProjectService,
	shareService *service.ShareService,
	lifecycleService *service.LifecycleService,
	lifecycleRepo *sqlite.LifecycleRepository,
	workerPipeline *service.WorkerPipeline,
) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.MetricsMiddleware)
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)

	ipLimiter := middleware.NewIPRateLimiter(rate.Limit(100), 200)
	r.Use(middleware.RateLimitMiddleware(ipLimiter))

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "X-API-Key", "X-Project-ID", "X-Share-Password", "Range", "If-None-Match", "Upload-Offset", "Upload-Length", "Upload-Metadata", "Upload-Defer-Length", "Tus-Resumable", "x-goog-meta-*", "x-meta-*", "x-amz-*"},
		ExposedHeaders:   []string{"Link", "Content-Length", "Content-Range", "ETag", "Accept-Ranges", "Upload-Offset", "Upload-Length", "Location", "Tus-Resumable", "Tus-Version", "Tus-Extension", "Tus-Max-Size", "Content-Disposition", "x-amz-*"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	storageHandler := handlers.NewStorageHandler(storageService, chunkService, zipStreamer, cfg.MaxUploadMB)
	bucketHandler := handlers.NewBucketHandler(bucketService)
	keyHandler := handlers.NewKeyHandler(keyService)
	webhookHandler := handlers.NewWebhookHandler(webhookService)
	workerHandler := handlers.NewWorkerHandler(workerPipeline)
	s3Handler := handlers.NewS3Handler(storageService, bucketService)
	adminHandler := handlers.NewAdminHandler(auditService, gcService, backupService, storageService)
	authHandler := handlers.NewAuthHandler(authService)
	projectHandler := handlers.NewProjectHandler(projectService)
	shareHandler := handlers.NewShareHandler(shareService)
	lifecycleHandler := handlers.NewLifecycleHandler(lifecycleService, lifecycleRepo)
	tusHandler := handlers.NewTUSHandler(chunkService, storageService)

	// Metrics & Kubernetes Health Probes
	r.Handle("/metrics", promhttp.Handler())
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","ready":true}`))
	})
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ready"}`))
	})

	// Public Share Link Access (/s/{token} and /s/{token}/download)
	r.Route("/s/{token}", func(r chi.Router) {
		r.Get("/", shareHandler.GetShareInfo)
		r.Get("/download", shareHandler.DownloadSharedFile)
		r.Post("/download", shareHandler.DownloadSharedFile)
	})

	// AWS S3 Compatibility API Gateway Routes (/s3/{bucket} and /s3/{bucket}/*)
	r.Route("/s3/{bucket}", func(r chi.Router) {
		r.Use(middleware.OptionalAuth(keyService, authService))
		r.HandleFunc("/", s3Handler.HandleBucketOrObject)
		r.HandleFunc("/*", s3Handler.HandleBucketOrObject)
	})

	// Firebase Storage v0 Compatibility Routes
	r.Route("/v0/b/{bucket}/o", func(r chi.Router) {
		r.Use(middleware.OptionalAuth(keyService, authService))
		r.Post("/", storageHandler.Upload)
		r.Get("/*", storageHandler.Download)
		r.Delete("/*", storageHandler.Delete)
	})

	// Core API v1 Routes
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(middleware.OptionalAuth(keyService, authService))

		r.Get("/stats", bucketHandler.GetStats)
		r.Get("/events", storageHandler.EventsSSE)

		// Auth & Superuser Endpoints
		r.Route("/auth", func(r chi.Router) {
			r.Post("/login", authHandler.Login)
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireAuth(keyService, authService))
				r.Post("/logout", authHandler.Logout)
				r.Get("/me", authHandler.Me)
			})
		})

		// Multi-Tenant Projects
		r.Route("/projects", func(r chi.Router) {
			r.Use(middleware.RequireAuth(keyService, authService))
			r.Get("/", projectHandler.List)
			r.Get("/{id}", projectHandler.Get)
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireAdmin)
				r.Post("/", projectHandler.Create)
				r.Delete("/{id}", projectHandler.Delete)
			})
		})

		// Bucket Management & Lifecycle
		r.Route("/buckets", func(r chi.Router) {
			r.Get("/", bucketHandler.List)
			r.Get("/{bucket}", bucketHandler.Get)
			r.Get("/{bucket}/lifecycle", lifecycleHandler.GetRule)
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireAuth(keyService, authService))
				r.Post("/", bucketHandler.Create)
				r.Put("/{bucket}", bucketHandler.Update)
				r.Delete("/{bucket}", bucketHandler.Delete)
				r.Put("/{bucket}/lifecycle", lifecycleHandler.SetRule)
			})
		})

		// Public Share Management
		r.Route("/share", func(r chi.Router) {
			r.Use(middleware.RequireAuth(keyService, authService))
			r.Post("/", shareHandler.CreateShare)
			r.Get("/bucket/{bucket}", shareHandler.ListShares)
			r.Delete("/{token}", shareHandler.DeleteShare)
		})

		// TUS 1.0.0 Open Protocol Resumable Upload Gateway
		r.Route("/tus", func(r chi.Router) {
			r.Options("/*", tusHandler.Options)
			r.Route("/files", func(r chi.Router) {
				r.Options("/", tusHandler.Options)
				r.Post("/", tusHandler.CreateUpload)
				r.Options("/{uploadId}", tusHandler.Options)
				r.Head("/{uploadId}", tusHandler.HeadUpload)
				r.Patch("/{uploadId}", tusHandler.PatchUpload)
				r.Delete("/{uploadId}", tusHandler.TerminateUpload)
			})
		})

		// Lifecycle & Scrubber Operations
		r.Route("/lifecycle", func(r chi.Router) {
			r.Use(middleware.RequireAdmin)
			r.Post("/sweep", lifecycleHandler.TriggerSweep)
			r.Post("/scrubber", lifecycleHandler.TriggerScrubber)
		})

		// Object Storage, Versioning, Resumable Uploads & Signed URLs
		r.Route("/b/{bucket}", func(r chi.Router) {
			r.Post("/sign-url", storageHandler.SignURL)
			r.Post("/rotate-token", storageHandler.RotateToken)
			r.Post("/download-zip", storageHandler.DownloadZip)
			r.Get("/lifecycle", lifecycleHandler.GetRule)
			r.With(middleware.RequireAdmin).Put("/lifecycle", lifecycleHandler.SetRule)

			// Resumable Chunked Uploads
			r.Route("/uploads", func(r chi.Router) {
				r.Post("/init", storageHandler.InitChunkUpload)
				r.Patch("/{uploadId}", storageHandler.AppendChunk)
				r.Post("/{uploadId}/chunk", storageHandler.AppendChunk)
				r.Post("/{uploadId}/complete", storageHandler.CompleteChunkUpload)
				r.Delete("/{uploadId}", storageHandler.AbortChunkUpload)
			})

			// File Management & Trash
			r.Post("/copy", storageHandler.Copy)
			r.Post("/move", storageHandler.Move)
			r.Post("/rename", storageHandler.Rename)
			r.Post("/delete-folder", storageHandler.DeleteFolder)
			r.Post("/restore", storageHandler.RestoreFile)
			r.Get("/versions", storageHandler.ListVersions)

			r.Route("/o", func(r chi.Router) {
				r.Get("/", storageHandler.List)
				r.Post("/", storageHandler.Upload)
				r.Get("/*", storageHandler.Download)
				r.Delete("/*", storageHandler.Delete)
			})
		})

		// Webhook Management
		r.Route("/webhooks", func(r chi.Router) {
			r.Use(middleware.RequireAuth(keyService, authService))
			r.Get("/", webhookHandler.List)
			r.Get("/{id}", webhookHandler.Get)
			r.Get("/{id}/deliveries", webhookHandler.ListDeliveries)
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireAdmin)
				r.Post("/", webhookHandler.Create)
				r.Put("/{id}", webhookHandler.Update)
				r.Post("/{id}/toggle", webhookHandler.Toggle)
				r.Post("/{id}/test", webhookHandler.Test)
				r.Post("/{id}/deliveries/{deliveryId}/redeliver", webhookHandler.Redeliver)
				r.Delete("/{id}", webhookHandler.Delete)
			})
		})

		// Background Worker Pool & Job Queue Engine
		r.Route("/workers", func(r chi.Router) {
			r.Use(middleware.RequireAuth(keyService, authService))
			r.Get("/status", workerHandler.GetStatus)
			r.Get("/jobs", workerHandler.GetJobs)
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireAdmin)
				r.Post("/trigger", workerHandler.TriggerJob)
				r.Post("/scale", workerHandler.ScaleWorkers)
				r.Post("/clear-history", workerHandler.ClearHistory)
			})
		})

		// API Key Management
		r.Route("/keys", func(r chi.Router) {
			r.Use(middleware.RequireAdmin)
			r.Get("/", keyHandler.List)
			r.Post("/", keyHandler.Create)
			r.Post("/{id}/rotate", keyHandler.Rotate)
			r.Post("/{id}/toggle-revoke", keyHandler.ToggleRevoke)
			r.Post("/{id}/revoke", keyHandler.Revoke)
			r.Delete("/{id}", keyHandler.Delete)
		})

		// Admin, Operations & Compliance
		r.Route("/admin", func(r chi.Router) {
			r.Use(middleware.RequireAdmin)
			r.Get("/audit-logs", adminHandler.ListAuditLogs)
			r.Get("/audit-logs/export", adminHandler.ExportAuditLogs)
			r.Get("/dedup-report", adminHandler.GetDedupReport)
			r.Post("/backup", adminHandler.RunBackup)
			r.Post("/restore", adminHandler.RestoreBackup)
			r.Post("/gc", adminHandler.RunGC)
			r.Post("/scrub", storageHandler.ScrubIntegrity)
		})
	})

	// Web UI & Embedded Static Files (SPA fallback handler)
	r.Handle("/static/*", http.StripPrefix("/static/", web.StaticHandler()))
	r.Get("/*", web.ServeIndex)
	r.Head("/*", web.ServeIndex)

	return r
}
