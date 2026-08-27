package handlers

import (
	"errors"
	"io"
	"net/http"

	"dogapp-api/internal/video"
)

const (
	maxVideoUploadBytes = 50 << 20 // 50MB; client caps recordings at 15s
	gaitCheckFrameCount = 5
)

// POST /dogs/{dogId}/gait-check (multipart/form-data, field name "video")
func (s *Server) gaitCheck(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxVideoUploadBytes)
	if err := r.ParseMultipartForm(maxVideoUploadBytes); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart request: "+err.Error())
		return
	}

	file, _, err := r.FormFile("video")
	if err != nil {
		writeError(w, http.StatusBadRequest, `missing "video" file field: `+err.Error())
		return
	}
	defer file.Close()

	videoBytes, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read video: "+err.Error())
		return
	}

	frames, err := video.ExtractFrames(r.Context(), videoBytes, gaitCheckFrameCount)
	if err != nil {
		if errors.Is(err, video.ErrFFmpegNotFound) {
			writeError(w, http.StatusNotImplemented, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, "failed to extract frames from video: "+err.Error())
		return
	}

	result, err := s.Checker.CheckGaitFrames(r.Context(), frames, "image/jpeg")
	if err != nil {
		writeError(w, http.StatusBadGateway, "gait check failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}
