// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package calendarwire

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/outbound"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestCalendarRequestDoesNotFollowCredentialRedirect(t *testing.T) {
	reached := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer source.Close()
	status, err := Request(context.Background(), source.Client(), "secret", http.MethodGet, source.URL, nil, nil)
	if status != http.StatusFound || err == nil || reached {
		t.Fatalf("redirect followed: status=%d err=%v reached=%v", status, err, reached)
	}
}

func TestCalendarRequestClassifiesAuthWithoutLeakingProviderBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != outbound.CalendarHeader {
			t.Errorf("calendar request did not identify Margince: %q", r.Header.Get("User-Agent"))
		}
		w.WriteHeader(http.StatusForbidden)
		if _, err := w.Write([]byte("private-provider-diagnostic")); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	_, err := Request(context.Background(), server.Client(), "secret", http.MethodGet, server.URL, nil, nil)
	if !errors.Is(err, connector.ErrAuthRejected) || strings.Contains(err.Error(), "private-provider") {
		t.Fatalf("unsafe error: %v", err)
	}
}
