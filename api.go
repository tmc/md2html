package main

import (
	"encoding/json"
	"net/http"
)

// handleVersionsAPI returns the list of available versions as JSON
func (s *server) handleVersionsAPI(w http.ResponseWriter, r *http.Request) {
	if !s.config.Versions {
		http.Error(w, "Versioning not enabled", http.StatusNotFound)
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

	// Try to get current version if we're in a git repo
	if s.versionMgr != nil {
		if current, err := s.versionMgr.GetCurrentVersion(); err == nil {
			response.CurrentVersion = current
		}
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.Error("Error encoding versions JSON", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}
