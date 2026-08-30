// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGoogleTokenExchangeParsesIDToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if r.FormValue("grant_type") != "authorization_code" {
			t.Fatalf("grant_type = %q", r.FormValue("grant_type"))
		}
		if r.FormValue("code_verifier") != "verifier-xyz" {
			t.Fatalf("code_verifier = %q", r.FormValue("code_verifier"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id_token":"the.id.token","access_token":"ignored"}`))
	}))
	defer srv.Close()

	ex := googleTokenExchanger{ClientID: "cid", ClientSecret: "secret", TokenURL: srv.URL, HTTPClient: srv.Client()}
	idToken, err := ex.Exchange(t.Context(), "auth-code", "verifier-xyz", "https://app.example.com/v1/auth/oidc/google/callback")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if idToken != "the.id.token" {
		t.Fatalf("idToken = %q", idToken)
	}
}

func TestGoogleTokenExchangeRejectsMissingIDToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"a"}`))
	}))
	defer srv.Close()

	ex := googleTokenExchanger{ClientID: "cid", ClientSecret: "secret", TokenURL: srv.URL, HTTPClient: srv.Client()}
	if _, err := ex.Exchange(t.Context(), "auth-code", "verifier-xyz", "https://app.example.com/cb"); err == nil {
		t.Fatal("expected an error for a response with no id_token")
	}
}

func TestGoogleTokenExchangeRejectsNonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer srv.Close()

	ex := googleTokenExchanger{ClientID: "cid", ClientSecret: "secret", TokenURL: srv.URL, HTTPClient: srv.Client()}
	if _, err := ex.Exchange(t.Context(), "auth-code", "verifier-xyz", "https://app.example.com/cb"); err == nil {
		t.Fatal("expected an error for a non-200 response")
	}
}
