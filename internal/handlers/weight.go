package handlers

import (
	"encoding/json"
	"net/http"
)

type addWeightEntryRequest struct {
	Month string  `json:"month"`
	Kg    float64 `json:"kg"`
}

// POST /dogs/{dogId}/weight
func (s *Server) addWeightEntry(w http.ResponseWriter, r *http.Request) {
	dogID := r.PathValue("dogId")

	if !s.requireOwnedDog(w, r, dogID) {
		return
	}

	var req addWeightEntryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.Month == "" {
		writeError(w, http.StatusBadRequest, "month is required")
		return
	}
	if req.Kg <= 0 {
		writeError(w, http.StatusBadRequest, "kg must be positive")
		return
	}

	entry, err := s.Store.AddWeightEntry(r.Context(), dogID, req.Month, req.Kg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, entry)
}
