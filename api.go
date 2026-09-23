package md2html

import (
	"encoding/json"
	"html/template"
	"net/http"
	"strings"
)

// handleVersionsAPI returns the list of available versions as JSON
func (s *server) handleVersionsAPI(w http.ResponseWriter, r *http.Request) {
	if !s.config.Versions {
		http.Error(w, "versioning not enabled", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	response := struct {
		Versions       []GitVersion `json:"versions"`
		CurrentVersion string       `json:"current_version,omitempty"`
		DefaultVersion string       `json:"default_version,omitempty"`
	}{
		Versions:       s.versions,
		DefaultVersion: s.config.VersionDefault,
	}

	if s.versionMgr != nil {
		if current, err := s.versionMgr.currentVersion(r.Context()); err == nil {
			response.CurrentVersion = current
		}
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.Error("Error encoding versions JSON", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

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
