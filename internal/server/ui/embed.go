package ui

import (
	"embed"
	"html"
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
// basePath is the public path prefix (e.g. "/gitseer") injected as <meta name="gitseer-base">
// so the SPA can resolve API routes without an inline script (CSP script-src 'self').
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
	htmlDoc := string(b)
	inject := `<meta name="gitseer-base" content="` + html.EscapeString(basePath) + `">`
	if strings.Contains(htmlDoc, "</head>") {
		htmlDoc = strings.Replace(htmlDoc, "</head>", inject+"</head>", 1)
	} else {
		htmlDoc = inject + htmlDoc
	}
	// Fix absolute asset paths for subpath deploys.
	if basePath != "" {
		htmlDoc = strings.ReplaceAll(htmlDoc, `href="/assets/`, `href="`+basePath+`/assets/`)
		htmlDoc = strings.ReplaceAll(htmlDoc, `src="/assets/`, `src="`+basePath+`/assets/`)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(htmlDoc))
}
