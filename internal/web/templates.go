package web

import (
	"embed"
	"html/template"
)

//go:embed templates
var templateFS embed.FS

// ParseTemplates parses every *.html file under templates/ into one
// [template.Template], keyed by filename (e.g. "wishlist.html"). Called
// once at startup (internal/app/servers.go) — not per-request.
func ParseTemplates() (*template.Template, error) {
	return template.ParseFS(templateFS, "templates/*.html")
}
