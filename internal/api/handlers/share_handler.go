package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"gostore/internal/core/domain"
	"gostore/internal/service"
)

type ShareHandler struct {
	shareService *service.ShareService
}

func NewShareHandler(shareService *service.ShareService) *ShareHandler {
	return &ShareHandler{shareService: shareService}
}

// CreateShare creates a new password/metered share link
func (h *ShareHandler) CreateShare(w http.ResponseWriter, r *http.Request) {
	var req service.CreateShareRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	share, err := h.shareService.CreateShareLink(r.Context(), req)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(share)
}

// GetShareInfo gets metadata for a public share token or renders an OpenGraph preview landing page
func (h *ShareHandler) GetShareInfo(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	share, file, err := h.shareService.GetShareInfo(r.Context(), token)
	if err != nil {
		if strings.Contains(r.Header.Get("Accept"), "text/html") {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`<!DOCTYPE html><html><head><title>File Not Found</title><style>body{background:#0f172a;color:#fff;font-family:sans-serif;display:flex;align-items:center;justify-content:center;height:100vh;margin:0;}div{text-align:center;background:#1e293b;padding:2rem;border-radius:12px;border:1px solid #334155;}</style></head><body><div><h2>⚠️ Shared Link Not Found</h2><p>This share link may have expired, been revoked, or reached its download limit.</p></div></body></html>`))
			return
		}
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}

	// If browser or social crawler requests HTML, serve OpenGraph landing page
	if strings.Contains(r.Header.Get("Accept"), "text/html") && r.URL.Query().Get("format") != "json" {
		h.renderShareHTML(w, r, share, file)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"share": share,
		"file":  file,
	})
}

func (h *ShareHandler) renderShareHTML(w http.ResponseWriter, r *http.Request, share *service.ShareResponse, file *domain.FileObject) {
	formattedSize := formatFileSize(file.Size)
	downloadURL := fmt.Sprintf("/s/%s/download", share.Token)
	previewURL := downloadURL

	var previewElement string
	if !share.HasPassword {
		if strings.HasPrefix(file.ContentType, "image/") {
			previewElement = fmt.Sprintf(`<div style="margin:1.5rem 0;"><img src="%s" alt="%s" style="max-width:100%%;max-height:380px;border-radius:10px;object-fit:contain;box-shadow:0 10px 25px rgba(0,0,0,0.5);" /></div>`, previewURL, file.Name)
		} else if strings.HasPrefix(file.ContentType, "video/") {
			previewElement = fmt.Sprintf(`<div style="margin:1.5rem 0;"><video controls src="%s" style="max-width:100%%;max-height:360px;border-radius:10px;"></video></div>`, previewURL)
		} else if strings.HasPrefix(file.ContentType, "audio/") {
			previewElement = fmt.Sprintf(`<div style="margin:1.5rem 0;"><audio controls src="%s" style="width:100%%;border-radius:30px;"></audio></div>`, previewURL)
		} else if file.ContentType == "application/pdf" {
			previewElement = fmt.Sprintf(`<div style="margin:1.5rem 0;"><iframe src="%s" style="width:100%%;height:380px;border-radius:10px;border:none;"></iframe></div>`, previewURL)
		}
	}

	passwordForm := ""
	if share.HasPassword {
		passwordForm = `<div style="margin:1.5rem 0;text-align:left;">
			<label style="display:block;margin-bottom:0.5rem;font-size:0.875rem;color:#94a3b8;">Password Protected</label>
			<input type="password" name="password" id="sharePassword" placeholder="Enter share password" required style="width:100%;padding:0.75rem 1rem;background:#0f172a;border:1px solid #334155;border-radius:8px;color:#fff;font-size:0.95rem;box-sizing:border-box;" />
		</div>`
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>%s | GoStore Secure Share</title>
	
	<!-- OpenGraph & Twitter Cards -->
	<meta property="og:title" content="%s" />
	<meta property="og:description" content="Download %s (%s) securely shared via GoStore Object Storage." />
	<meta property="og:type" content="website" />
	<meta property="og:site_name" content="GoStore" />
	<meta name="twitter:card" content="summary_large_image" />
	<meta name="twitter:title" content="%s" />
	<meta name="twitter:description" content="Download %s (%s) securely shared via GoStore." />
	
	<style>
		* { box-sizing: border-box; margin: 0; padding: 0; }
		body { background: #090d16; color: #f8fafc; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; display: flex; align-items: center; justify-content: center; min-height: 100vh; padding: 1.5rem; }
		.card { background: rgba(30, 41, 59, 0.7); backdrop-filter: blur(16px); border: 1px solid rgba(255, 255, 255, 0.08); border-radius: 20px; padding: 2.5rem; max-width: 540px; width: 100%%; text-align: center; box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.7); }
		.badge { display: inline-block; padding: 0.35rem 0.85rem; background: rgba(56, 189, 248, 0.15); color: #38bdf8; border-radius: 9999px; font-size: 0.8rem; font-weight: 600; margin-bottom: 1.25rem; border: 1px solid rgba(56, 189, 248, 0.3); }
		h1 { font-size: 1.5rem; font-weight: 700; margin-bottom: 0.5rem; word-break: break-all; color: #fff; }
		.meta { color: #94a3b8; font-size: 0.9rem; margin-bottom: 1.5rem; }
		.btn { display: inline-flex; align-items: center; justify-content: center; width: 100%%; padding: 0.9rem 1.5rem; background: linear-gradient(135deg, #3b82f6, #6366f1); color: #fff; text-decoration: none; border-radius: 12px; font-weight: 600; font-size: 1.05rem; transition: transform 0.15s, opacity 0.15s; border: none; cursor: pointer; }
		.btn:hover { opacity: 0.92; transform: translateY(-2px); }
		.footer { margin-top: 1.5rem; font-size: 0.75rem; color: #64748b; }
	</style>
</head>
<body>
	<div class="card">
		<span class="badge">🔒 Secure File Transfer</span>
		<h1>%s</h1>
		<p class="meta">%s • %s</p>
		%s
		<form method="GET" action="%s" id="downloadForm">
			%s
			<button type="submit" class="btn">⚡ Download File (%s)</button>
		</form>
		<div class="footer">Powered by GoStore Cloud Storage Engine</div>
	</div>
</body>
</html>`, file.Name, file.Name, file.Name, formattedSize, file.Name, file.Name, formattedSize, file.Name, formattedSize, file.ContentType, previewElement, downloadURL, passwordForm, formattedSize)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(html))
}

func formatFileSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// DownloadSharedFile handles direct streaming download of a shared object
func (h *ShareHandler) DownloadSharedFile(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	password := r.URL.Query().Get("password")
	if password == "" {
		password = r.Header.Get("X-Share-Password")
	}

	reader, file, err := h.shareService.DownloadSharedFile(r.Context(), token, password)
	if err != nil {
		if err.Error() == "password required" || err.Error() == "invalid password" {
			writeJSONError(w, http.StatusUnauthorized, err.Error())
			return
		}
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Disposition", "inline; filename=\""+file.Name+"\"")
	w.Header().Set("Content-Type", file.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(file.Size, 10))

	http.ServeContent(w, r, file.Name, file.UpdatedAt, reader)
}

// DeleteShare revokes a share token
func (h *ShareHandler) DeleteShare(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if err := h.shareService.DeleteShareLink(r.Context(), token); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Share link revoked"})
}

// ListShares lists share links for a bucket
func (h *ShareHandler) ListShares(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	shares, err := h.shareService.ListByBucket(r.Context(), bucket)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(shares)
}
