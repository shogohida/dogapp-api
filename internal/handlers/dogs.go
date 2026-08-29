package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
)

// GET /dogs (authenticated)
func (s *Server) listDogs(w http.ResponseWriter, r *http.Request) {
	dogs, err := s.Store.ListDogs(r.Context(), userIDFrom(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, dogs)
}

type dogProfileRequest struct {
	Name      string `json:"name"`
	Breed     string `json:"breed"`
	Color     string `json:"color"`
	BirthYear int    `json:"birthYear"`
}

func (req dogProfileRequest) validate() string {
	switch {
	case req.Name == "":
		return "name is required"
	case req.Breed == "":
		return "breed is required"
	case req.Color == "":
		return "color is required"
	case req.BirthYear <= 0:
		return "birthYear must be positive"
	default:
		return ""
	}
}

// POST /dogs (authenticated)
func (s *Server) createDog(w http.ResponseWriter, r *http.Request) {
	var req dogProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if msg := req.validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	dog, err := s.Store.CreateDog(r.Context(), userIDFrom(r), req.Name, req.Breed, req.Color, req.BirthYear)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, dog)
}

// PATCH /dogs/{dogId} (authenticated; must be owned by the caller)
func (s *Server) updateDog(w http.ResponseWriter, r *http.Request) {
	dogID := r.PathValue("dogId")

	var req dogProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if msg := req.validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	dog, err := s.Store.UpdateDog(r.Context(), dogID, userIDFrom(r), req.Name, req.Breed, req.Color, req.BirthYear)
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
