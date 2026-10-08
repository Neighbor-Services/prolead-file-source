package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed static/*
var staticFS embed.FS

func StaticHandler() http.Handler {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}
	return http.FileServer(http.FS(sub))
}

func ServeIndex(w http.ResponseWriter, r *http.Request) {
	// If requesting a specific file with extension (e.g. .js, .css, .ico), try serving static
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path != "" && strings.Contains(path, ".") {
		data, err := staticFS.ReadFile("static/" + path)
		if err == nil {
			if strings.HasSuffix(path, ".js") {
				w.Header().Set("Content-Type", "application/javascript")
			} else if strings.HasSuffix(path, ".css") {
				w.Header().Set("Content-Type", "text/css")
			} else if strings.HasSuffix(path, ".ico") {
				w.Header().Set("Content-Type", "image/x-icon")
			}
			_, _ = w.Write(data)
			return
		}
	}

	data, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error": "Dashboard index.html not found"}`))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}
