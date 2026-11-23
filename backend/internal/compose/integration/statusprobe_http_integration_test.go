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
func probeStatus(t *testing.T, e *apptest.AppEnv, method string) (*http.Response, []byte) {
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
	return resp, body
}

func TestAMonitorSendingHeadToTheStatusProbeHearsWhatGetHears(t *testing.T) {
	e := apptest.SetupApp(t)
	if resp, body := probeStatus(t, e, http.MethodHead); resp.StatusCode != http.StatusServiceUnavailable || len(body) != 0 {
		t.Fatalf("HEAD before bootstrap = %d with %d body bytes, want 503 and none", resp.StatusCode, len(body))
	}
	apptest.BootstrapWorkspaceSession(t, e, "Status Probe", "status-probe@fable.test", "Admin")
	get, _ := probeStatus(t, e, http.MethodGet)
	head, body := probeStatus(t, e, http.MethodHead)
	if head.StatusCode != http.StatusOK || get.StatusCode != http.StatusOK || len(body) != 0 {
		t.Fatalf("HEAD = %d with %d body bytes, GET = %d; want both 200 and HEAD bodiless", head.StatusCode, len(body), get.StatusCode)
	}
	for _, header := range []string{"Cache-Control", "Content-Type"} {
		if head.Header.Get(header) != get.Header.Get(header) {
			t.Errorf("HEAD sent %s %q, GET %q", header, head.Header.Get(header), get.Header.Get(header))
		}
	}
}
