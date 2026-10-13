// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// importPartProfileDTO is one file of an uploaded ZIP as the profile reads it.
type importPartProfileDTO struct {
	File              string                       `json:"file"`
	Object            string                       `json:"object"`
	RowsProfiled      int                          `json:"rows_profiled"`
	SuggestedMappings map[string]map[string]string `json:"suggested_mappings"`
}

// importBundleProfileDTO is the upload's answer for a ZIP.
type importBundleProfileDTO struct {
	SourceRef           string                 `json:"source_ref"`
	FileName            string                 `json:"file_name"`
	SuggestedSourceName string                 `json:"suggested_source_name"`
	Parts               []importPartProfileDTO `json:"parts"`
	Ignored             []struct {
		File   string `json:"file"`
		Reason string `json:"reason"`
	} `json:"ignored"`
}

// zipBytes writes the files into one ZIP, keyed by entry name.
func zipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatalf("adding %s to the zip: %v", name, err)
		}
		if _, err := f.Write([]byte(body)); err != nil {
			t.Fatalf("writing %s into the zip: %v", name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("closing the zip: %v", err)
	}
	return buf.Bytes()
}

// mustJSON decodes a response body or fails the test naming it.
//
//craft:ignore naked-any out is json.Unmarshal's own target parameter, which takes any pointer a caller decodes into
func mustJSON(t *testing.T, raw []byte, out any) {
	t.Helper()
	if err := json.Unmarshal(raw, out); err != nil {
		t.Fatalf("decoding %q: %v", raw, err)
	}
}

// uploadZip posts a ZIP, requires a 200, and returns its profile.
func uploadZip(t *testing.T, e *apptest.AppEnv, fileName string, body []byte) importBundleProfileDTO {
	t.Helper()
	raw, status := postZipFile(t, e, fileName, body)
	if status != http.StatusOK {
		t.Fatalf("zip upload → %d %s, want 200", status, raw)
	}
	var profile importBundleProfileDTO
	mustJSON(t, raw, &profile)
	return profile
}

// postZipFile sends a file with no object field, the way a ZIP is uploaded.
func postZipFile(t *testing.T, e *apptest.AppEnv, fileName string, body []byte) ([]byte, int) {
	t.Helper()
	var buf bytes.Buffer
	form := multipart.NewWriter(&buf)
	part, err := form.CreateFormFile("file", fileName)
	if err != nil {
		t.Fatalf("creating the file part: %v", err)
	}
	if _, err := part.Write(body); err != nil {
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
