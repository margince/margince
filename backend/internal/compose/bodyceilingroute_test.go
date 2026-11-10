// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/httpserver"
)

// sampleID fills every path parameter: each upload route's parameter is one segment.
const sampleID = "01a1a9ff-1625-723c-906e-02eddcd5eb56"

// A file past the JSON bound and within its route's own ceiling is read whole by
// the chassis. A route whose path carries an id is held to that as much as any.
func TestAFileWithinItsRoutesCeilingPassesTheChassis(t *testing.T) {
	limits := DefaultUploadLimits()
	for _, path := range []string{
		"/v1/legal-entities/" + sampleID + "/logo",
		"/v1/knowledge/corpora/" + sampleID + "/documents",
	} {
		var read int64
		var readErr error
		chassis := httpserver.LimitBodies(bodyCeilingFor(uploadCeilings(limits), contractRoutes()),
			http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				read, readErr = io.Copy(io.Discard, r.Body)
			}))
		body := make([]byte, 2<<20)
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
		chassis.ServeHTTP(httptest.NewRecorder(), req)
		var overflow *http.MaxBytesError
		if errors.As(readErr, &overflow) || read != int64(len(body)) {
			t.Errorf("POST %s with %d bytes: read %d, stopped by %v", path, len(body), read, readErr)
		}
	}
}

// Every upload route the contract declares, addressed at a real path, rides the
// ceiling its pattern declares, through the chassis and the agent door alike.
func TestEveryUploadRouteResolvesToItsOwnCeiling(t *testing.T) {
	limits := UploadLimits{Attachment: 31_000_000, CSVImport: 12_000_000, LinkedInImport: 7_000_000, KnowledgeDocument: 9_000_000}
	declared := uploadCeilings(limits)
	ceiling := bodyCeilingFor(declared, contractRoutes())
	door := &viewDoor{ceiling: ceiling}
	for pattern, operation := range uploadOperations(t) {
		want, ok := declared[pattern]
		if !ok || want == httperr.MaxBodyBytes {
			t.Errorf("%s (%s) declares %d, which cannot tell its own ceiling from the JSON bound", pattern, operation, want)
			continue
		}
		req := httptest.NewRequest(http.MethodPost, pathParameter.ReplaceAllString(pattern, sampleID), http.NoBody)
		req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
		if got := ceiling(req); got != want {
			t.Errorf("POST %s (%s) rode %d through the chassis, want its route's %d", req.URL.Path, operation, got, want)
		}
		if got := door.bodyCeiling(req); got != want {
			t.Errorf("POST %s (%s) rode %d through the agent door, want its route's %d", req.URL.Path, operation, got, want)
		}
	}
}
