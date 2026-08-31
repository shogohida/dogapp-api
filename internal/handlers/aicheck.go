package handlers

import (
	"encoding/base64"
	"encoding/json"
	"net/http"

	"dogapp-api/internal/claude"
)

type aiCheckRequest struct {
	ImageBase64 string `json:"imageBase64"`
	// BodyPart selects which vet-assistant prompt to use (see
	// claude.ValidBodyPart). Empty defaults to "skin" so older clients that
	// predate this field keep working unchanged.
	BodyPart string `json:"bodyPart"`
}

// maxImageUploadBytes caps the raw request body. Base64 inflates the
// original bytes by ~4/3, so this comfortably covers a compressed phone
// photo (the client requests imageQuality: 85) while still bounding memory
// use per request.
const maxImageUploadBytes = 15 << 20 // 15MB

// POST /dogs/{dogId}/ai-check
func (s *Server) aiCheck(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxImageUploadBytes)

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

	bodyPart := req.BodyPart
	if bodyPart == "" {
		bodyPart = claude.BodyPartSkin
	}
	if !claude.ValidBodyPart(bodyPart) {
		writeError(w, http.StatusBadRequest, "invalid bodyPart: "+bodyPart)
		return
	}

	mediaType := http.DetectContentType(imageBytes)
	result, err := s.Checker.CheckPhoto(r.Context(), imageBytes, mediaType, bodyPart)
	if err != nil {
		writeError(w, http.StatusBadGateway, "AI check failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}
