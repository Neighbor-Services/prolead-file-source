package handlers

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gostore/internal/repository/sqlite"
	"gostore/internal/service"
)

type AdminHandler struct {
	auditService   *service.AuditService
	gcService      *service.GarbageCollector
	backupService  *service.BackupService
	storageService *service.StorageService
}

func NewAdminHandler(
	auditService *service.AuditService,
	gcService *service.GarbageCollector,
	backupService *service.BackupService,
	storageService *service.StorageService,
) *AdminHandler {
	return &AdminHandler{
		auditService:   auditService,
		gcService:      gcService,
		backupService:  backupService,
		storageService: storageService,
	}
}

func (h *AdminHandler) ListAuditLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))

	filter := sqlite.AuditFilter{
		Action: q.Get("action"),
		Bucket: q.Get("bucket"),
		Actor:  q.Get("actor"),
		Limit:  limit,
		Offset: offset,
	}

	if fromStr := q.Get("from"); fromStr != "" {
		if t, err := time.Parse(time.RFC3339, fromStr); err == nil {
			filter.From = &t
		}
	}
	if toStr := q.Get("to"); toStr != "" {
		if t, err := time.Parse(time.RFC3339, toStr); err == nil {
			filter.To = &t
		}
	}

	logs, total, err := h.auditService.List(filter)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"items":      logs,
		"totalCount": total,
	})
}

// ExportAuditLogs streams compliance audit logs in CSV or JSON format
func (h *AdminHandler) ExportAuditLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	format := strings.ToLower(q.Get("format"))
	if format == "" {
		format = "csv"
	}

	filter := sqlite.AuditFilter{
		Action: q.Get("action"),
		Bucket: q.Get("bucket"),
		Actor:  q.Get("actor"),
		Limit:  10000,
	}

	if fromStr := q.Get("from"); fromStr != "" {
		if t, err := time.Parse(time.RFC3339, fromStr); err == nil {
			filter.From = &t
		}
	}
	if toStr := q.Get("to"); toStr != "" {
		if t, err := time.Parse(time.RFC3339, toStr); err == nil {
			filter.To = &t
		}
	}

	logs, _, err := h.auditService.List(filter)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	filenameDate := time.Now().UTC().Format("2006-01-02")

	if format == "json" {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"audit-logs-%s.json\"", filenameDate))
		_ = json.NewEncoder(w).Encode(logs)
		return
	}

	// Default CSV streaming
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"audit-logs-%s.csv\"", filenameDate))

	csvWriter := csv.NewWriter(w)
	_ = csvWriter.Write([]string{"ID", "Action", "Bucket", "Path", "Actor", "IPAddress", "UserAgent", "Status", "DurationMs", "CreatedAt"})

	for _, log := range logs {
		_ = csvWriter.Write([]string{
			log.ID,
			log.Action,
			log.Bucket,
			log.Path,
			log.Actor,
			log.IPAddress,
			log.UserAgent,
			strconv.Itoa(log.Status),
			strconv.FormatInt(log.DurationMs, 10),
			log.CreatedAt.Format(time.RFC3339),
		})
	}
	csvWriter.Flush()
}

func (h *AdminHandler) GetDedupReport(w http.ResponseWriter, r *http.Request) {
	report, err := h.gcService.CalculateDedupReport()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(report)
}

func (h *AdminHandler) RunGC(w http.ResponseWriter, r *http.Request) {
	stats, err := h.gcService.RunGC(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(stats)
}

func (h *AdminHandler) RunBackup(w http.ResponseWriter, r *http.Request) {
	filename := fmt.Sprintf("gostore-backup-%s.tar.gz", time.Now().Format("20060102-150405"))
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	if err := h.backupService.StreamBackup(w); err != nil {
		return
	}
}

func (h *AdminHandler) RestoreBackup(w http.ResponseWriter, r *http.Request) {
	var stream = r.Body
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		file, _, err := r.FormFile("backup")
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "Missing 'backup' file in form data: "+err.Error())
			return
		}
		defer file.Close()
		stream = file
	}

	if stream == nil {
		writeJSONError(w, http.StatusBadRequest, "Missing backup input stream")
		return
	}

	if err := h.backupService.RestoreBackup(stream); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Restore failed: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Backup successfully restored",
	})
}
