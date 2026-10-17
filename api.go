package md2html

import (
	"html/template"
	"net/http"
	"strings"
)

// handleJSONSpecSchemas returns the loaded schema bundle as JSON. It
// serves the same payload embedded in each page's
// <script id="md-jsonspec-schemas"> tag, so tools or lazy-loading JS
// can fetch it without scraping HTML.
func (s *server) handleJSONSpecSchemas(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(s.config.JSONSpec) == "" {
		http.Error(w, "jsonspec schemas not configured", http.StatusNotFound)
		return
	}
	var payload template.JS
	if s.prepared != nil {
		payload = s.prepared.jsonSpecBundle
	}
	if payload == "" {
		http.Error(w, "schema bundle unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(payload))
}
