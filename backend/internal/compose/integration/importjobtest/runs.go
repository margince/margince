// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package importjobtest

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// Column is one source column an upload profiled.
type Column struct {
	Header   string   `json:"header"`
	FillRate float64  `json:"fill_rate"`
	Samples  []string `json:"samples"`
}

// Profile is what an upload answers: the run it opened and its columns.
type Profile struct {
	RunID            string            `json:"run_id"`
	RowCount         int               `json:"row_count"`
	SourceRef        string            `json:"source_ref"`
	Object           string            `json:"object"`
	Columns          []Column          `json:"columns"`
	RowsProfiled     int               `json:"rows_profiled"`
	SuggestedMapping map[string]string `json:"suggested_mapping"`
	Targets          []string          `json:"targets"`
}

// Run is an import run as the API answers it.
type Run struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	Connector  string `json:"connector"`
	Object     string `json:"object"`
	Checkpoint int    `json:"checkpoint"`
	Error      string `json:"error"`
	Version    int    `json:"version"`
	DraftStep  string `json:"draft_step"`
	// Released is the run's source_released.
	Released     bool   `json:"source_released"`
	OnBrokenLink string `json:"on_broken_link"`
	SupersededBy string `json:"superseded_by"`
}

// CreateRunWithMapping starts a run over an uploaded source with mapping, and
// works the test job it queues.
func CreateRunWithMapping(t *testing.T, e *apptest.AppEnv, object, sourceRef string, mapping map[string]string) (Run, int) {
	t.Helper()
	var run Run
	status := CallTestRun(t, e, "/v1/imports", map[string]any{
		"connector": "csv", "object": object,
		"source_ref": sourceRef, "mapping": mapping,
	}, &run)
	return run, status
}

// UploadCSV posts one file to the upload operation and returns the profile.
func UploadCSV(t *testing.T, e *apptest.AppEnv, object, body string) (Profile, int) {
	t.Helper()
	raw, status := PostCSVFile(t, e, object, body)
	var profile Profile
	if len(raw) > 0 && status == http.StatusOK {
		if err := json.Unmarshal(raw, &profile); err != nil {
			t.Fatalf("decoding %q: %v", raw, err)
		}
	}
	return profile, status
}

// PostCSVFile sends one file to the upload operation and returns the raw answer.
func PostCSVFile(t *testing.T, e *apptest.AppEnv, object, body string) ([]byte, int) {
	t.Helper()
	var buf bytes.Buffer
	form := multipart.NewWriter(&buf)
	if err := form.WriteField("object", object); err != nil {
		t.Fatalf("writing the object field: %v", err)
	}
	part, err := form.CreateFormFile("file", "estate.csv")
	if err != nil {
		t.Fatalf("creating the file part: %v", err)
	}
	if _, err := part.Write([]byte(body)); err != nil {
		t.Fatalf("writing the file part: %v", err)
	}
	if err := form.Close(); err != nil {
		t.Fatalf("closing the form: %v", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, e.TS.URL+"/v1/imports/sources", &buf)
	if err != nil {
		t.Fatalf("building the upload: %v", err)
	}
	req.Header.Set("Content-Type", form.FormDataContentType())
	//nolint:bodyclose // apptest.CloseBody closes it in the deferred call below, which the checker cannot follow across the helper.
	resp, err := e.Client.Do(req)
	if err != nil {
		t.Fatalf("uploading: %v", err)
	}
	defer apptest.CloseBody(t, resp)

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading the upload response: %v", err)
	}
	return raw, resp.StatusCode
}

// CallTestRun posts a request that starts a test run, then works the job it
// queued. out then holds the run as the worker left it, or the refusal.
//
//craft:ignore naked-any out is whichever shape the caller reads the run or the refusal into, as e.Call takes it
func CallTestRun(t *testing.T, e *apptest.AppEnv, path string, body, out any) int {
	t.Helper()
	var answer json.RawMessage
	status := e.Call(t, http.MethodPost, path, body, nil, &answer)
	if status == http.StatusAccepted {
		var queued struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		}
		if err := json.Unmarshal(answer, &queued); err != nil || queued.Status != "validating" {
			t.Fatalf("POST %s → 202 %s (%v), want the run at validating", path, answer, err)
		}
		Drain(t, e)
		if got := e.Call(t, http.MethodGet, "/v1/imports/"+queued.ID, nil, nil, &answer); got != http.StatusOK {
			t.Fatalf("GET the run after its test → %d, want 200", got)
		}
	}
	if out != nil && len(answer) > 0 {
		if err := json.Unmarshal(answer, out); err != nil {
			t.Fatalf("decoding the answer to POST %s: %v", path, err)
		}
	}
	return status
}
