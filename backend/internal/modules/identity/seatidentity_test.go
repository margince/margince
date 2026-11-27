// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

type seatFieldError struct {
	Field string `json:"field"`
	Code  string `json:"code"`
	Max   int    `json:"max"`
}

type seatRefusal struct {
	Details struct {
		Errors []seatFieldError `json:"errors"`
	} `json:"details"`
}

func seatRefusalOf(t *testing.T, email string) seatFieldError {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/users", nil)
	if _, _, ok := seatIdentity(rec, req, email, "Ada", values.ParseEmail); ok {
		t.Fatalf("an invite to %q was accepted", email)
	}
	var body seatRefusal
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body.Details.Errors) != 1 {
		t.Fatalf("refusal of %q = %s (%v), want one field error", email, rec.Body, err)
	}
	return body.Details.Errors[0]
}

func TestAnInviteRefusesAnAddressWithNoDottedDomain(t *testing.T) {
	got := seatRefusalOf(t, "a@b")
	if got.Field != "email" || got.Code != "invalid_email" {
		t.Fatalf("refusal = %+v, want email/invalid_email", got)
	}
}

func TestAnInviteNamesTheLengthLimitOfAnAddress(t *testing.T) {
	got := seatRefusalOf(t, strings.Repeat("a", 300)+"@example.com")
	if got.Field != "email" || got.Code != "too_long" || got.Max != values.MaxEmailLength {
		t.Fatalf("refusal = %+v, want email/too_long with max %d", got, values.MaxEmailLength)
	}
}
