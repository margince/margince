// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"io"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// probeStatus sends the uptime monitor's request: a client with no cookie jar, so no session.
func probeStatus(t *testing.T, e *apptest.AppEnv, method string) httpAnswer {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, e.TS.URL+"/v1/status", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := e.TS.Client().Do(req)
	if err != nil {
		t.Fatalf("%s /v1/status: %v", method, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("closing the body: %v", err)
		}
	}()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading the body: %v", err)
	}
	return httpAnswer{status: resp.StatusCode, header: resp.Header, body: string(body)}
}

func TestAMonitorSendingHeadToTheStatusProbeHearsWhatGetHears(t *testing.T) {
	e := apptest.SetupApp(t)
	if early := probeStatus(t, e, http.MethodHead); early.status != http.StatusServiceUnavailable || early.body != "" {
		t.Fatalf("HEAD before bootstrap = %d with %d body bytes, want 503 and none", early.status, len(early.body))
	}
	apptest.BootstrapWorkspaceSession(t, e, "Status Probe", "status-probe@fable.test", "Admin")
	get := probeStatus(t, e, http.MethodGet)
	head := probeStatus(t, e, http.MethodHead)
	if head.status != http.StatusOK || get.status != http.StatusOK || head.body != "" {
		t.Fatalf("HEAD = %d with %d body bytes, GET = %d; want both 200 and HEAD bodiless", head.status, len(head.body), get.status)
	}
	for _, header := range []string{"Cache-Control", "Content-Type"} {
		if head.header.Get(header) != get.header.Get(header) {
			t.Errorf("HEAD sent %s %q, GET %q", header, head.header.Get(header), get.header.Get(header))
		}
	}
}
