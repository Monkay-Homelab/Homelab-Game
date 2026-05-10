package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegister_RegistrationDisabled(t *testing.T) {
	handler := NewAuthHandler(nil, nil, "test-secret", false)

	body := strings.NewReader(`{"email":"a@b.com","password":"12345678","display_name":"TestUser"}`)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/auth/register", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Register(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}

	want := `{"error":"registration is currently disabled"}`
	got := strings.TrimSpace(rec.Body.String())
	if got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

func TestRegister_RegistrationEnabled_PassesGuard(t *testing.T) {
	// With registrationEnabled=true and nil query pointers, the handler
	// proceeds past the guard and eventually fails on request decoding or
	// a later step. The key assertion: the response is NOT 403.
	handler := NewAuthHandler(nil, nil, "test-secret", true)

	// Send an empty body so the handler fails at JSON decoding (400),
	// proving the registration guard did not block the request.
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/auth/register", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Register(rec, req)

	if rec.Code == http.StatusForbidden {
		t.Fatalf("status = 403, but registration is enabled — guard should not have fired")
	}

	// Verify we got a 400 (invalid request body) which confirms the
	// handler moved past the guard into request parsing.
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d (expected failure at request parsing, not at the guard)", rec.Code, http.StatusBadRequest)
	}
}
