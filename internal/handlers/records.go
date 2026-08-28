package handlers

import (
	"encoding/json"
	"net/http"

	"dogapp-api/internal/model"
)

type createRecordRequest struct {
	Type  model.RecordType `json:"type"`
	Label string           `json:"label"`
	Cost  *float64         `json:"cost"`
}

// POST /dogs/{dogId}/records
func (s *Server) addRecord(w http.ResponseWriter, r *http.Request) {
	dogID := r.PathValue("dogId")
	ctx := r.Context()

	exists, err := s.Store.DogExists(ctx, dogID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "dog not found: "+dogID)
		return
	}

	var req createRecordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	switch req.Type {
	case model.RecordVaccine, model.RecordGrooming, model.RecordVet, model.RecordMedication, model.RecordAICheck:
	default:
		writeError(w, http.StatusBadRequest, "unknown record type: "+string(req.Type))
		return
	}
	if req.Label == "" {
		writeError(w, http.StatusBadRequest, "label is required")
		return
	}
	if req.Cost != nil && *req.Cost < 0 {
		writeError(w, http.StatusBadRequest, "cost must not be negative")
		return
	}

	record, err := s.Store.AddRecord(ctx, dogID, req.Type, req.Label, req.Cost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, record)
}
