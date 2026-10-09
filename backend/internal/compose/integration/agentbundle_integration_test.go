// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The skill download through the real router. A signed-in human gets the ZIP
// and a passport is refused it. The ZIP and the passport list name one address.

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func TestAHumanDownloadsTheSkillAndAPassportIsRefusedIt(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	status, contentType, body := getAgentBundle(t, e, nil)
	if status != http.StatusOK || contentType != "application/zip" {
		t.Fatalf("human GET /v1/agent-bundle → %d %s, want 200 application/zip", status, contentType)
	}
	// The harness configures its public base and no API base, so that is the address.
	const want = "https://mail.example.test/v1"
	if spec := zipFile(t, body, "margince/openapi.yaml"); !strings.Contains(spec, "url: "+want+"\n") {
		t.Errorf("openapi.yaml does not name %s as its server", want)
	}
	var listed struct {
		APIBaseURL string `json:"api_base_url"`
	}
	if status := e.Call(t, "GET", "/v1/passports", nil, nil, &listed); status != http.StatusOK {
		t.Fatalf("GET /v1/passports → %d", status)
	}
	if listed.APIBaseURL != want {
		t.Errorf("GET /v1/passports api_base_url = %q, want %q, the address the skill names", listed.APIBaseURL, want)
	}

	bearer := apptest.PassportBearer(t, e, "skill download probe", "read")
	if status, _, _ := getAgentBundle(t, e, bearer); status != http.StatusForbidden {
		t.Errorf("passport GET /v1/agent-bundle → %d, want 403: the download is human-only", status)
	}
}

func TestTheAPIBaseURLWinsOverThePublicOne(t *testing.T) {
	const apiBase = "https://api.example.test"
	e := apptest.SetupAppWithOptions(t, compose.WithAPIBaseURL(apiBase))
	e.BootstrapWorkspace(t)

	_, _, body := getAgentBundle(t, e, nil)
	if spec := zipFile(t, body, "margince/openapi.yaml"); !strings.Contains(spec, "url: "+apiBase+"/v1\n") {
		t.Errorf("openapi.yaml does not name the configured API base %s/v1", apiBase)
	}
}

func getAgentBundle(t *testing.T, e *apptest.AppEnv, headers map[string]string) (int, string, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, e.TS.URL+"/v1/agent-bundle", nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := e.Client.Do(req) //nolint:bodyclose // closed by apptest.CloseBody below
	if err != nil {
		t.Fatalf("GET /v1/agent-bundle: %v", err)
	}
	defer apptest.CloseBody(t, resp)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading the bundle: %v", err)
	}
	return resp.StatusCode, resp.Header.Get("Content-Type"), body
}

func zipFile(t *testing.T, archive []byte, name string) string {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatalf("the bundle is not a ZIP: %v", err)
	}
	f, err := reader.Open(name)
	if err != nil {
		t.Fatalf("the bundle has no %s: %v", name, err)
	}
	body, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("closing %s: %v", name, err)
	}
	return string(body)
}
