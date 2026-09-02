package mailer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResendMailerSendWelcomeEmail(t *testing.T) {
	var gotAuth string
	var gotBody resendEmailRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := NewResendMailer("test-key", "dogapp@example.com")
	m.httpClient = srv.Client()
	m.apiURL = srv.URL

	if err := m.SendWelcomeEmail(context.Background(), "new-user@example.com"); err != nil {
		t.Fatalf("SendWelcomeEmail: %v", err)
	}

	if gotAuth != "Bearer test-key" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer test-key")
	}
	if gotBody.From != "dogapp@example.com" {
		t.Errorf("From = %q, want %q", gotBody.From, "dogapp@example.com")
	}
	if len(gotBody.To) != 1 || gotBody.To[0] != "new-user@example.com" {
		t.Errorf("To = %v, want [new-user@example.com]", gotBody.To)
	}
	if gotBody.Subject == "" || gotBody.HTML == "" {
		t.Errorf("expected non-empty subject/html, got subject=%q html=%q", gotBody.Subject, gotBody.HTML)
	}
}

func TestResendMailerSendWelcomeEmailErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"invalid api key"}`))
	}))
	defer srv.Close()

	m := NewResendMailer("bad-key", "dogapp@example.com")
	m.httpClient = srv.Client()
	m.apiURL = srv.URL

	if err := m.SendWelcomeEmail(context.Background(), "new-user@example.com"); err == nil {
		t.Fatal("expected an error for a non-2xx response, got nil")
	}
}

func TestNoopMailerSucceeds(t *testing.T) {
	if err := (NoopMailer{}).SendWelcomeEmail(context.Background(), "someone@example.com"); err != nil {
		t.Fatalf("NoopMailer.SendWelcomeEmail: %v", err)
	}
}
