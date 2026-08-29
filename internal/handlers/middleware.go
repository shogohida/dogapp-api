package handlers

import (
	"context"
	"net/http"
	"strings"

	"dogapp-api/internal/auth"
)

type contextKey string

const userIDContextKey contextKey = "userID"

// withAuth requires a valid "Authorization: Bearer <token>" header, and
// makes the authenticated user's id available to the wrapped handler via
// userIDFrom.
func withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || token == "" {
			writeError(w, http.StatusUnauthorized, "missing or invalid Authorization header")
			return
		}
		userID, err := auth.VerifyToken(token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userIDContextKey, userID)))
	}
}

func userIDFrom(r *http.Request) string {
	id, _ := r.Context().Value(userIDContextKey).(string)
	return id
}

// requireOwnedDog reports whether dogID belongs to the authenticated user,
// writing a 404 (never leaking whether the dog exists under someone else)
// or 500 and returning false if not.
func (s *Server) requireOwnedDog(w http.ResponseWriter, r *http.Request, dogID string) bool {
	owned, err := s.Store.DogOwnedBy(r.Context(), dogID, userIDFrom(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return false
	}
	if !owned {
		writeError(w, http.StatusNotFound, "dog not found: "+dogID)
		return false
	}
	return true
}
