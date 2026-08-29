package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dogapp-api/internal/model"
	"dogapp-api/internal/store"
	"dogapp-api/internal/store/storetest"
)

// fakeChecker stands in for the real Claude API in tests.
type fakeChecker struct {
	result model.AICheckResult
	err    error
}

func (f *fakeChecker) CheckSkinPhoto(ctx context.Context, imageBytes []byte, mediaType string) (model.AICheckResult, error) {
	return f.result, f.err
}

func (f *fakeChecker) CheckGaitFrames(ctx context.Context, frames [][]byte, mediaType string) (model.AICheckResult, error) {
	return f.result, f.err
}

func newTestServer(t *testing.T) (*Server, http.Handler) {
	t.Helper()
	s, err := store.Open(context.Background(), storetest.NewDSN(t))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	srv := &Server{
		Store: s,
		Checker: &fakeChecker{
			result: model.AICheckResult{Level: model.LevelNormal, Title: "問題なし", Detail: "異常なし"},
		},
	}
	return srv, srv.Routes()
}

func authedRequest(method, path string, body []byte, token string) *http.Request {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

// signupToken registers a fresh user and returns their auth token.
func signupToken(t *testing.T, routes http.Handler, email string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"email": email, "password": "correct-password"})
	req := httptest.NewRequest(http.MethodPost, "/auth/signup", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("signup: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp authResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal signup response: %v", err)
	}
	return resp.Token
}

// createDog creates a dog owned by token's user and returns it.
func createDog(t *testing.T, routes http.Handler, token, name string) model.Dog {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"name": name, "breed": "Standard Poodle", "color": "Apricot", "birthYear": 2021,
	})
	req := authedRequest(http.MethodPost, "/dogs", body, token)
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create dog: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var dog model.Dog
	if err := json.Unmarshal(rec.Body.Bytes(), &dog); err != nil {
		t.Fatalf("unmarshal dog: %v", err)
	}
	return dog
}

// newOwnedDogFixture is the common case nearly every dog-scoped handler test
// needs: a signed-up user with one dog they own.
func newOwnedDogFixture(t *testing.T) (routes http.Handler, token string, dogID string) {
	t.Helper()
	_, routes = newTestServer(t)
	token = signupToken(t, routes, "owner@example.com")
	dogID = createDog(t, routes, token, "Leo").ID
	return routes, token, dogID
}

func TestHealthCheck(t *testing.T) {
	_, routes := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestSignupThenLogin(t *testing.T) {
	_, routes := newTestServer(t)

	body, _ := json.Marshal(map[string]string{"email": "person@example.com", "password": "correct-password"})
	signupReq := httptest.NewRequest(http.MethodPost, "/auth/signup", bytes.NewReader(body))
	signupRec := httptest.NewRecorder()
	routes.ServeHTTP(signupRec, signupReq)
	if signupRec.Code != http.StatusCreated {
		t.Fatalf("signup: status = %d, body = %s", signupRec.Code, signupRec.Body.String())
	}

	loginReq := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
	loginRec := httptest.NewRecorder()
	routes.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login: status = %d, body = %s", loginRec.Code, loginRec.Body.String())
	}
	var resp authResponse
	if err := json.Unmarshal(loginRec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Token == "" || resp.User.Email != "person@example.com" {
		t.Fatalf("unexpected login response: %+v", resp)
	}
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	_, routes := newTestServer(t)
	signupToken(t, routes, "person2@example.com")

	body, _ := json.Marshal(map[string]string{"email": "person2@example.com", "password": "wrong-password"})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body = %s", rec.Code, rec.Body.String())
	}
}

func TestLoginRejectsUnknownEmail(t *testing.T) {
	_, routes := newTestServer(t)

	body, _ := json.Marshal(map[string]string{"email": "nobody@example.com", "password": "whatever1"})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body = %s", rec.Code, rec.Body.String())
	}
}

func TestSignupRejectsDuplicateEmail(t *testing.T) {
	_, routes := newTestServer(t)
	signupToken(t, routes, "dup@example.com")

	body, _ := json.Marshal(map[string]string{"email": "dup@example.com", "password": "correct-password"})
	req := httptest.NewRequest(http.MethodPost, "/auth/signup", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
}

func TestSignupRejectsShortPassword(t *testing.T) {
	_, routes := newTestServer(t)

	body, _ := json.Marshal(map[string]string{"email": "short@example.com", "password": "short"})
	req := httptest.NewRequest(http.MethodPost, "/auth/signup", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestSignupRejectsInvalidEmail(t *testing.T) {
	_, routes := newTestServer(t)

	body, _ := json.Marshal(map[string]string{"email": "not-an-email", "password": "correct-password"})
	req := httptest.NewRequest(http.MethodPost, "/auth/signup", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestProtectedRouteRejectsMissingToken(t *testing.T) {
	_, routes := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/dogs", nil)
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body = %s", rec.Code, rec.Body.String())
	}
}

func TestProtectedRouteRejectsGarbageToken(t *testing.T) {
	_, routes := newTestServer(t)

	req := authedRequest(http.MethodGet, "/dogs", nil, "not-a-real-token")
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body = %s", rec.Code, rec.Body.String())
	}
}

func TestCreateAndListDogs(t *testing.T) {
	_, routes := newTestServer(t)
	token := signupToken(t, routes, "owner@example.com")

	created := createDog(t, routes, token, "Leo")
	if created.ID == "" {
		t.Fatal("expected a generated dog id")
	}

	req := authedRequest(http.MethodGet, "/dogs", nil, token)
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var dogs []model.Dog
	if err := json.Unmarshal(rec.Body.Bytes(), &dogs); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(dogs) != 1 || dogs[0].ID != created.ID {
		t.Fatalf("expected only the created dog, got %+v", dogs)
	}
}

func TestListDogsIsScopedToOwner(t *testing.T) {
	_, routes := newTestServer(t)
	aliceToken := signupToken(t, routes, "alice@example.com")
	bobToken := signupToken(t, routes, "bob@example.com")
	aliceDog := createDog(t, routes, aliceToken, "Alice's dog")
	createDog(t, routes, bobToken, "Bob's dog")

	req := authedRequest(http.MethodGet, "/dogs", nil, aliceToken)
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	var dogs []model.Dog
	if err := json.Unmarshal(rec.Body.Bytes(), &dogs); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(dogs) != 1 || dogs[0].ID != aliceDog.ID {
		t.Fatalf("expected only alice's dog, got %+v", dogs)
	}
}

// Regression test: a Go nil slice marshals to JSON `null`, which the
// Flutter client's `as List<dynamic>` cast can't handle. Every freshly
// signed-up user has zero dogs, so this must come back as `[]`, not `null`.
func TestListDogsForNewUserReturnsEmptyArrayNotNull(t *testing.T) {
	_, routes := newTestServer(t)
	token := signupToken(t, routes, "nodogs@example.com")

	req := authedRequest(http.MethodGet, "/dogs", nil, token)
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := strings.TrimSpace(rec.Body.String())
	if body != "[]" {
		t.Fatalf("expected response body \"[]\", got %q", body)
	}
}

func TestUpdateDog(t *testing.T) {
	routes, token, dogID := newOwnedDogFixture(t)

	body, _ := json.Marshal(map[string]any{
		"name": "レオ2", "breed": "トイプードル", "color": "ホワイト", "birthYear": 2020,
	})
	req := authedRequest(http.MethodPatch, "/dogs/"+dogID, body, token)
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var dog model.Dog
	if err := json.Unmarshal(rec.Body.Bytes(), &dog); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if dog.Name != "レオ2" || dog.Breed != "トイプードル" || dog.Color != "ホワイト" || dog.BirthYear != 2020 {
		t.Fatalf("unexpected dog: %+v", dog)
	}
}

// A user must not be able to edit another user's dog.
func TestUpdateDogRejectsNonOwner(t *testing.T) {
	_, routes := newTestServer(t)
	ownerToken := signupToken(t, routes, "owner@example.com")
	otherToken := signupToken(t, routes, "other@example.com")
	dogID := createDog(t, routes, ownerToken, "Leo").ID

	body, _ := json.Marshal(map[string]any{
		"name": "x", "breed": "x", "color": "x", "birthYear": 2020,
	})
	req := authedRequest(http.MethodPatch, "/dogs/"+dogID, body, otherToken)
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateDogUnknownDog(t *testing.T) {
	routes, token, _ := newOwnedDogFixture(t)

	body, _ := json.Marshal(map[string]any{
		"name": "x", "breed": "x", "color": "x", "birthYear": 2020,
	})
	req := authedRequest(http.MethodPatch, "/dogs/does-not-exist", body, token)
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateDogRequiresName(t *testing.T) {
	routes, token, dogID := newOwnedDogFixture(t)

	body, _ := json.Marshal(map[string]any{
		"name": "", "breed": "x", "color": "x", "birthYear": 2020,
	})
	req := authedRequest(http.MethodPatch, "/dogs/"+dogID, body, token)
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestAddWeightEntry(t *testing.T) {
	routes, token, dogID := newOwnedDogFixture(t)

	body, _ := json.Marshal(map[string]any{"month": "9月", "kg": 25.6})
	req := authedRequest(http.MethodPost, "/dogs/"+dogID+"/weight", body, token)
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var entry model.WeightEntry
	if err := json.Unmarshal(rec.Body.Bytes(), &entry); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if entry.Month != "9月" || entry.Kg != 25.6 {
		t.Fatalf("unexpected entry: %+v", entry)
	}
}

func TestAddWeightEntryRejectsNonPositiveKg(t *testing.T) {
	routes, token, dogID := newOwnedDogFixture(t)

	body, _ := json.Marshal(map[string]any{"month": "9月", "kg": 0})
	req := authedRequest(http.MethodPost, "/dogs/"+dogID+"/weight", body, token)
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

// A user must not be able to add a weight entry to another user's dog.
func TestAddWeightEntryRejectsNonOwner(t *testing.T) {
	_, routes := newTestServer(t)
	ownerToken := signupToken(t, routes, "owner@example.com")
	otherToken := signupToken(t, routes, "other@example.com")
	dogID := createDog(t, routes, ownerToken, "Leo").ID

	body, _ := json.Marshal(map[string]any{"month": "9月", "kg": 25.6})
	req := authedRequest(http.MethodPost, "/dogs/"+dogID+"/weight", body, otherToken)
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", rec.Code, rec.Body.String())
	}
}

func TestAddRecord(t *testing.T) {
	routes, token, dogID := newOwnedDogFixture(t)

	body, _ := json.Marshal(map[string]any{"type": "vet", "label": "定期健診", "cost": 4500.0})
	req := authedRequest(http.MethodPost, "/dogs/"+dogID+"/records", body, token)
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var record model.HealthRecord
	if err := json.Unmarshal(rec.Body.Bytes(), &record); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if record.Label != "定期健診" || record.Type != model.RecordVet {
		t.Fatalf("unexpected record: %+v", record)
	}
	if record.Cost == nil || *record.Cost != 4500.0 {
		t.Fatalf("unexpected cost: %v", record.Cost)
	}
}

func TestAddRecordWithoutCost(t *testing.T) {
	routes, token, dogID := newOwnedDogFixture(t)

	body, _ := json.Marshal(map[string]string{"type": "vaccine", "label": "ワクチン接種"})
	req := authedRequest(http.MethodPost, "/dogs/"+dogID+"/records", body, token)
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var record model.HealthRecord
	if err := json.Unmarshal(rec.Body.Bytes(), &record); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if record.Cost != nil {
		t.Fatalf("expected nil cost, got %v", *record.Cost)
	}
}

func TestAddRecordRejectsNegativeCost(t *testing.T) {
	routes, token, dogID := newOwnedDogFixture(t)

	body, _ := json.Marshal(map[string]any{"type": "vet", "label": "x", "cost": -100.0})
	req := authedRequest(http.MethodPost, "/dogs/"+dogID+"/records", body, token)
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestAddRecordUnknownDog(t *testing.T) {
	routes, token, _ := newOwnedDogFixture(t)

	body, _ := json.Marshal(map[string]string{"type": "vet", "label": "x"})
	req := authedRequest(http.MethodPost, "/dogs/does-not-exist/records", body, token)
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", rec.Code, rec.Body.String())
	}
}

// A user must not be able to add a record to another user's dog.
func TestAddRecordRejectsNonOwner(t *testing.T) {
	_, routes := newTestServer(t)
	ownerToken := signupToken(t, routes, "owner@example.com")
	otherToken := signupToken(t, routes, "other@example.com")
	dogID := createDog(t, routes, ownerToken, "Leo").ID

	body, _ := json.Marshal(map[string]string{"type": "vet", "label": "x"})
	req := authedRequest(http.MethodPost, "/dogs/"+dogID+"/records", body, otherToken)
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", rec.Code, rec.Body.String())
	}
}

// typeは固定の列挙値ではなく自由入力なので、既知の値以外でも受け付ける。
func TestAddRecordAcceptsFreeTextType(t *testing.T) {
	routes, token, dogID := newOwnedDogFixture(t)

	body, _ := json.Marshal(map[string]string{"type": "爪切り", "label": "x"})
	req := authedRequest(http.MethodPost, "/dogs/"+dogID+"/records", body, token)
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", rec.Code, rec.Body.String())
	}
	var record model.HealthRecord
	if err := json.Unmarshal(rec.Body.Bytes(), &record); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if record.Type != "爪切り" {
		t.Fatalf("unexpected type: %q", record.Type)
	}
}

func TestAddRecordRequiresType(t *testing.T) {
	routes, token, dogID := newOwnedDogFixture(t)

	body, _ := json.Marshal(map[string]string{"type": "", "label": "x"})
	req := authedRequest(http.MethodPost, "/dogs/"+dogID+"/records", body, token)
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestAICheck(t *testing.T) {
	_, routes := newTestServer(t)
	token := signupToken(t, routes, "owner@example.com")

	body, _ := json.Marshal(map[string]string{"imageBase64": "aGVsbG8="}) // "hello"
	req := authedRequest(http.MethodPost, "/dogs/leo/ai-check", body, token)
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var result model.AICheckResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.Level != model.LevelNormal {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestAICheckMissingImage(t *testing.T) {
	_, routes := newTestServer(t)
	token := signupToken(t, routes, "owner@example.com")

	body, _ := json.Marshal(map[string]string{})
	req := authedRequest(http.MethodPost, "/dogs/leo/ai-check", body, token)
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestAICheckRejectsOversizedBody(t *testing.T) {
	_, routes := newTestServer(t)
	token := signupToken(t, routes, "owner@example.com")

	oversized := bytes.Repeat([]byte("a"), maxImageUploadBytes+1)
	req := authedRequest(http.MethodPost, "/dogs/leo/ai-check", oversized, token)
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

// ffmpeg isn't guaranteed to be installed on the test machine, so this test
// only asserts the graceful failure mode (501 with a clear message) rather
// than a real extraction.
func TestGaitCheckWithoutFFmpegOrBadVideo(t *testing.T) {
	_, routes := newTestServer(t)
	token := signupToken(t, routes, "owner@example.com")

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("video", "walk.mp4")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	part.Write([]byte("not a real video"))
	writer.Close()

	req := authedRequest(http.MethodPost, "/dogs/leo/gait-check", buf.Bytes(), token)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	// Either ffmpeg is missing (501) or it's present but rejects the fake
	// video content (400) - both are the correct "can't proceed" response,
	// never a 200 or a panic.
	if rec.Code != http.StatusNotImplemented && rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 501 or 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestCreateAndListWalks(t *testing.T) {
	routes, token, dogID := newOwnedDogFixture(t)

	body, _ := json.Marshal(map[string]any{
		"startedAt":       "2026-08-27T10:00:00Z",
		"durationSeconds": 600,
		"distanceMeters":  850.5,
		"points": []map[string]any{
			{"lat": 35.0, "lng": 139.0, "timestamp": "2026-08-27T10:00:00Z"},
			{"lat": 35.001, "lng": 139.001, "timestamp": "2026-08-27T10:01:00Z"},
		},
	})
	req := authedRequest(http.MethodPost, "/dogs/"+dogID+"/walks", body, token)
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	listReq := authedRequest(http.MethodGet, "/dogs/"+dogID+"/walks", nil, token)
	listRec := httptest.NewRecorder()
	routes.ServeHTTP(listRec, listReq)

	var walks []model.WalkRoute
	if err := json.Unmarshal(listRec.Body.Bytes(), &walks); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(walks) != 1 || len(walks[0].Points) != 2 {
		t.Fatalf("unexpected walks: %+v", walks)
	}
}

func TestCreateWalkRejectsTooFewPoints(t *testing.T) {
	routes, token, dogID := newOwnedDogFixture(t)

	body, _ := json.Marshal(map[string]any{
		"startedAt":       "2026-08-27T10:00:00Z",
		"durationSeconds": 60,
		"distanceMeters":  10.0,
		"points": []map[string]any{
			{"lat": 35.0, "lng": 139.0, "timestamp": "2026-08-27T10:00:00Z"},
		},
	})
	req := authedRequest(http.MethodPost, "/dogs/"+dogID+"/walks", body, token)
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

// A user must not be able to see another user's walks.
func TestListWalksRejectsNonOwner(t *testing.T) {
	_, routes := newTestServer(t)
	ownerToken := signupToken(t, routes, "owner@example.com")
	otherToken := signupToken(t, routes, "other@example.com")
	dogID := createDog(t, routes, ownerToken, "Leo").ID

	req := authedRequest(http.MethodGet, "/dogs/"+dogID+"/walks", nil, otherToken)
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", rec.Code, rec.Body.String())
	}
}
