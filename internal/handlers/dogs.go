package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
)

// GET /owners/{ownerId}/dogs
// There's no real multi-tenant auth yet, so every owner sees every dog
// (see store.Store.ListDogs).
func (s *Server) listDogs(w http.ResponseWriter, r *http.Request) {
	dogs, err := s.Store.ListDogs(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, dogs)
}

type updateDogRequest struct {
	Name      string `json:"name"`
	Breed     string `json:"breed"`
	Color     string `json:"color"`
	BirthYear int    `json:"birthYear"`
}

// PATCH /dogs/{dogId}
func (s *Server) updateDog(w http.ResponseWriter, r *http.Request) {
	dogID := r.PathValue("dogId")

	var req updateDogRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Breed == "" {
		writeError(w, http.StatusBadRequest, "breed is required")
		return
	}
	if req.Color == "" {
		writeError(w, http.StatusBadRequest, "color is required")
		return
	}
	if req.BirthYear <= 0 {
		writeError(w, http.StatusBadRequest, "birthYear must be positive")
		return
	}

	dog, err := s.Store.UpdateDog(r.Context(), dogID, req.Name, req.Breed, req.Color, req.BirthYear)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "dog not found: "+dogID)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, dog)
}
