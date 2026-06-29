package web

import (
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

// render executes the named template with data, writing to w.
// On template error it writes a 500.
func render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("web: template %q error: %v", name, err)
		http.Error(w, "error intern del servidor", http.StatusInternalServerError)
	}
}
