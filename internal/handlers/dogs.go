package handlers

import "net/http"

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
