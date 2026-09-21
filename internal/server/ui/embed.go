package ui

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
	"strings"
)

// Dist holds the built Vite SPA (web/dist). A placeholder is committed so `go build` works before npm build.
//
//go:embed all:dist
var Dist embed.FS

// Handler serves the SPA with history-fallback to index.html.
// basePath is the public path prefix (e.g. "/lens") used to inject window.__LENS_BASE__.
func Handler(basePath string) http.Handler {
	sub, err := fs.Sub(Dist, "dist")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(sub))
	basePath = strings.TrimRight(basePath, "/")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" || !strings.Contains(path, ".") {
			serveIndex(w, sub, basePath)
			return
		}
		if _, err := fs.Stat(sub, path); err != nil {
			serveIndex(w, sub, basePath)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

func serveIndex(w http.ResponseWriter, sub fs.FS, basePath string) {
	f, err := sub.Open("index.html")
	if err != nil {
		http.Error(w, "ui not built", http.StatusInternalServerError)
		return
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		http.Error(w, "ui read error", http.StatusInternalServerError)
		return
	}
	html := string(b)
	inject := `<script>window.__LENS_BASE__=` + jsString(basePath) + `;</script>`
	if strings.Contains(html, "</head>") {
		html = strings.Replace(html, "</head>", inject+"</head>", 1)
	} else {
		html = inject + html
	}
	// Fix absolute asset paths for subpath deploys.
	if basePath != "" {
		html = strings.ReplaceAll(html, `href="/assets/`, `href="`+basePath+`/assets/`)
		html = strings.ReplaceAll(html, `src="/assets/`, `src="`+basePath+`/assets/`)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(html))
}

func jsString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}
