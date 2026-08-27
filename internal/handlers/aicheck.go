package handlers

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
)

type aiCheckRequest struct {
	ImageBase64 string `json:"imageBase64"`
}

// POST /dogs/{dogId}/ai-check
func (s *Server) aiCheck(w http.ResponseWriter, r *http.Request) {
	var req aiCheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.ImageBase64 == "" {
		writeError(w, http.StatusBadRequest, "imageBase64 is required")
		return
	}

	imageBytes, err := base64.StdEncoding.DecodeString(req.ImageBase64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "imageBase64 is not valid base64: "+err.Error())
		return
	}

	mediaType := http.DetectContentType(imageBytes)
	result, err := s.Checker.CheckSkinPhoto(r.Context(), imageBytes, mediaType)
	if err != nil {
		writeError(w, http.StatusBadGateway, "AI check failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}
