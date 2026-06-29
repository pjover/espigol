package web

import (
	"bytes"
	"embed"
	"html/template"
	"io/fs"
	"log"
	"net/http"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

var tmpl *template.Template

func init() {
	t, err := template.New("").ParseFS(templateFS, "templates/*.html")
	if err != nil {
		log.Fatalf("web: parsing templates: %v", err)
	}
	tmpl = t
}

// staticFileServer returns an http.Handler that serves files from the embedded static/ directory.
func staticFileServer() http.Handler {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("web: static sub-fs: %v", err)
	}
	return http.FileServerFS(sub)
}

// renderStatus buffers the named template execution, then writes Content-Type, status, and body.
// If ExecuteTemplate fails nothing has been written to w, so it falls back to a 500 plain-text error.
func renderStatus(w http.ResponseWriter, status int, name string, data any) {
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		log.Printf("web: template %q error: %v", name, err)
		http.Error(w, "error intern del servidor", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

// render executes the named template with a 200 OK status.
func render(w http.ResponseWriter, name string, data any) {
	renderStatus(w, http.StatusOK, name, data)
}
