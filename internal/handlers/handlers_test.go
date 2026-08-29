package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
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

func TestHealthCheck(t *testing.T) {
	_, routes := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestAICheckRejectsOversizedBody(t *testing.T) {
	_, routes := newTestServer(t)

	oversized := bytes.Repeat([]byte("a"), maxImageUploadBytes+1)
	req := httptest.NewRequest(http.MethodPost, "/dogs/leo/ai-check", bytes.NewReader(oversized))
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestListDogs(t *testing.T) {
	_, routes := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/owners/owner-1/dogs", nil)
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var dogs []model.Dog
	if err := json.Unmarshal(rec.Body.Bytes(), &dogs); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(dogs) != 2 {
		t.Fatalf("expected 2 dogs, got %d", len(dogs))
	}
}

func TestUpdateDog(t *testing.T) {
	_, routes := newTestServer(t)

	body, _ := json.Marshal(map[string]any{
		"name": "レオ2", "breed": "トイプードル", "color": "ホワイト", "birthYear": 2020,
	})
	req := httptest.NewRequest(http.MethodPatch, "/dogs/leo", bytes.NewReader(body))
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

func TestUpdateDogUnknownDog(t *testing.T) {
	_, routes := newTestServer(t)

	body, _ := json.Marshal(map[string]any{
		"name": "x", "breed": "x", "color": "x", "birthYear": 2020,
	})
	req := httptest.NewRequest(http.MethodPatch, "/dogs/does-not-exist", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateDogRequiresName(t *testing.T) {
	_, routes := newTestServer(t)

	body, _ := json.Marshal(map[string]any{
		"name": "", "breed": "x", "color": "x", "birthYear": 2020,
	})
	req := httptest.NewRequest(http.MethodPatch, "/dogs/leo", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestAddRecord(t *testing.T) {
	_, routes := newTestServer(t)

	body, _ := json.Marshal(map[string]any{"type": "vet", "label": "定期健診", "cost": 4500.0})
	req := httptest.NewRequest(http.MethodPost, "/dogs/leo/records", bytes.NewReader(body))
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
	_, routes := newTestServer(t)

	body, _ := json.Marshal(map[string]string{"type": "vaccine", "label": "ワクチン接種"})
	req := httptest.NewRequest(http.MethodPost, "/dogs/leo/records", bytes.NewReader(body))
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
	_, routes := newTestServer(t)

	body, _ := json.Marshal(map[string]any{"type": "vet", "label": "x", "cost": -100.0})
	req := httptest.NewRequest(http.MethodPost, "/dogs/leo/records", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestAddRecordUnknownDog(t *testing.T) {
	_, routes := newTestServer(t)

	body, _ := json.Marshal(map[string]string{"type": "vet", "label": "x"})
	req := httptest.NewRequest(http.MethodPost, "/dogs/does-not-exist/records", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", rec.Code, rec.Body.String())
	}
}

// typeは固定の列挙値ではなく自由入力なので、既知の値以外でも受け付ける。
func TestAddRecordAcceptsFreeTextType(t *testing.T) {
	_, routes := newTestServer(t)

	body, _ := json.Marshal(map[string]string{"type": "爪切り", "label": "x"})
	req := httptest.NewRequest(http.MethodPost, "/dogs/leo/records", bytes.NewReader(body))
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
	_, routes := newTestServer(t)

	body, _ := json.Marshal(map[string]string{"type": "", "label": "x"})
	req := httptest.NewRequest(http.MethodPost, "/dogs/leo/records", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestAICheck(t *testing.T) {
	_, routes := newTestServer(t)

	body, _ := json.Marshal(map[string]string{"imageBase64": "aGVsbG8="}) // "hello"
	req := httptest.NewRequest(http.MethodPost, "/dogs/leo/ai-check", bytes.NewReader(body))
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

	body, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest(http.MethodPost, "/dogs/leo/ai-check", bytes.NewReader(body))
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

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("video", "walk.mp4")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	part.Write([]byte("not a real video"))
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/dogs/leo/gait-check", &buf)
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
	_, routes := newTestServer(t)

	body, _ := json.Marshal(map[string]any{
		"startedAt":       "2026-08-27T10:00:00Z",
		"durationSeconds": 600,
		"distanceMeters":  850.5,
		"points": []map[string]any{
			{"lat": 35.0, "lng": 139.0, "timestamp": "2026-08-27T10:00:00Z"},
			{"lat": 35.001, "lng": 139.001, "timestamp": "2026-08-27T10:01:00Z"},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/dogs/leo/walks", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/dogs/leo/walks", nil)
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
	_, routes := newTestServer(t)

	body, _ := json.Marshal(map[string]any{
		"startedAt":       "2026-08-27T10:00:00Z",
		"durationSeconds": 60,
		"distanceMeters":  10.0,
		"points": []map[string]any{
			{"lat": 35.0, "lng": 139.0, "timestamp": "2026-08-27T10:00:00Z"},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/dogs/leo/walks", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}
