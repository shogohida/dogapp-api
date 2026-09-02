// Package mailer sends transactional email - currently just the signup
// welcome email - via Resend's HTTP API (https://resend.com).
package mailer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// requestTimeout bounds a single Resend API call so a stalled upstream
// request can't hang the caller indefinitely.
const requestTimeout = 10 * time.Second

const resendAPIURL = "https://api.resend.com/emails"

// Mailer is the interface handlers depend on, so tests can substitute a
// fake instead of calling the real Resend API.
type Mailer interface {
	SendWelcomeEmail(ctx context.Context, toEmail string) error
}

// ResendMailer sends email through Resend.
type ResendMailer struct {
	apiKey     string
	fromEmail  string
	httpClient *http.Client
	// apiURL defaults to resendAPIURL; tests point it at an httptest server.
	apiURL string
}

// NewResendMailer builds a Mailer backed by the real Resend API. fromEmail
// must be a sender address verified on the Resend account/domain.
func NewResendMailer(apiKey, fromEmail string) *ResendMailer {
	return &ResendMailer{
		apiKey:     apiKey,
		fromEmail:  fromEmail,
		httpClient: &http.Client{Timeout: requestTimeout},
		apiURL:     resendAPIURL,
	}
}

type resendEmailRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html"`
}

const welcomeSubject = "dogappへようこそ"

const welcomeHTML = `<p>dogappにご登録いただきありがとうございます。</p>
<p>愛犬の健康管理・散歩記録を今日から始めましょう。</p>`

func (m *ResendMailer) SendWelcomeEmail(ctx context.Context, toEmail string) error {
	body, err := json.Marshal(resendEmailRequest{
		From:    m.fromEmail,
		To:      []string{toEmail},
		Subject: welcomeSubject,
		HTML:    welcomeHTML,
	})
	if err != nil {
		return fmt.Errorf("marshal resend request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.apiURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build resend request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+m.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("resend request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("resend request failed: status %d: %s", resp.StatusCode, respBody)
	}
	return nil
}

// NoopMailer discards every send. It's the fallback when RESEND_API_KEY
// isn't configured, so local dev and tests don't need real credentials -
// signup still succeeds, just without an actual email going out.
type NoopMailer struct{}

func (NoopMailer) SendWelcomeEmail(ctx context.Context, toEmail string) error {
	log.Printf("mailer: RESEND_API_KEY not set, skipping welcome email to %s", toEmail)
	return nil
}
