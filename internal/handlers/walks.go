package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"dogapp-api/internal/model"
)

type geoPointRequest struct {
	Lat       float64   `json:"lat"`
	Lng       float64   `json:"lng"`
	Timestamp time.Time `json:"timestamp"`
}

type createWalkRequest struct {
	StartedAt       time.Time         `json:"startedAt"`
	DurationSeconds int               `json:"durationSeconds"`
	DistanceMeters  float64           `json:"distanceMeters"`
	Points          []geoPointRequest `json:"points"`
}

// GET /dogs/{dogId}/walks
func (s *Server) listWalks(w http.ResponseWriter, r *http.Request) {
	dogID := r.PathValue("dogId")
	if !s.requireOwnedDog(w, r, dogID) {
		return
	}
	walks, err := s.Store.ListWalks(r.Context(), dogID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, walks)
}

// POST /dogs/{dogId}/walks
func (s *Server) createWalk(w http.ResponseWriter, r *http.Request) {
	dogID := r.PathValue("dogId")
	ctx := r.Context()

	if !s.requireOwnedDog(w, r, dogID) {
		return
	}

	var req createWalkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if len(req.Points) < 2 {
		writeError(w, http.StatusBadRequest, "a walk needs at least 2 GPS points")
		return
	}

	walk, err := s.Store.CreateWalk(ctx, dogID, req.StartedAt, req.DurationSeconds, req.DistanceMeters, toModelPoints(req.Points))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, walk)
}

func toModelPoints(points []geoPointRequest) []model.GeoPoint {
	result := make([]model.GeoPoint, len(points))
	for i, p := range points {
		result[i] = model.GeoPoint{Lat: p.Lat, Lng: p.Lng, Timestamp: p.Timestamp}
	}
	return result
}
