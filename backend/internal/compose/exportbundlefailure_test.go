// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type fakeBundleWriter struct {
	body string
	err  error
}

func (f fakeBundleWriter) WriteBundle(_ context.Context, dst io.Writer) (BundleSummary, error) {
	if f.err != nil {
		return BundleSummary{}, f.err
	}
	_, err := io.WriteString(dst, f.body)
	return BundleSummary{}, err
}

func adminExportRequest() *http.Request {
	user := ids.NewV7()
	return exportRequest(principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			RoleKeys: []string{"admin"},
			Objects:  map[string]principal.ObjectGrant{"installation_settings": {Read: true, Update: true}},
			RowScope: principal.RowScopeAll,
		},
	})
}

// A bundle that cannot be built answers an error before any archive byte is
// sent, so a client never mistakes half a zip for a complete export.
func TestDownloadExportBundleAnswersAnErrorWhenTheBundleCannotBeBuilt(t *testing.T) {
	h := exportBundleHandlers{writer: fakeBundleWriter{err: errors.New("build failed")}, log: slog.New(slog.DiscardHandler)}
	w := httptest.NewRecorder()

	h.DownloadExportBundle(w, adminExportRequest())

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	if got := w.Header().Get("Content-Disposition"); got != "" {
		t.Errorf("Content-Disposition = %q: a failed build must not look like a download", got)
	}
}

func TestDownloadExportBundleDeclaresTheArchiveSize(t *testing.T) {
	h := exportBundleHandlers{writer: fakeBundleWriter{body: "PK-bytes"}, log: slog.New(slog.DiscardHandler)}
	w := httptest.NewRecorder()

	h.DownloadExportBundle(w, adminExportRequest())

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got, want := w.Header().Get("Content-Length"), strconv.Itoa(len("PK-bytes")); got != want {
		t.Errorf("Content-Length = %q, want %q so a cut-off download is detectable", got, want)
	}
}

// One stored instant outside years 0-9999 must not stop the bundle from being
// written: encoding/json refuses such a time, so it travels as text.
func TestWriteZipKeepsAnInstantOutsideTheJSONYearRange(t *testing.T) {
	outOfRange := time.Date(-4712, time.January, 1, 0, 0, 0, 0, time.UTC)
	members := []memberData{{
		table:   "contact",
		columns: []string{"id", "born_on"},
		rows:    [][]any{{[16]byte(ids.NewV7()), outOfRange}},
	}}
	var archive bytes.Buffer

	err := writeZip(&archive, principal.Principal{ID: "human:test"}, ids.NewV7(), members, BundleSummary{})
	if err != nil {
		t.Fatalf("writeZip: %v", err)
	}

	reader, err := zip.NewReader(bytes.NewReader(archive.Bytes()), int64(archive.Len()))
	if err != nil {
		t.Fatalf("the archive is not a valid zip: %v", err)
	}
	dump := readZipEntry(t, reader, "data.json")
	if !strings.Contains(dump, "-4712-01-01T00:00:00Z") {
		t.Errorf("data.json does not carry the instant as text:\n%s", dump)
	}
}

func readZipEntry(t *testing.T, reader *zip.Reader, name string) string {
	t.Helper()
	for _, f := range reader.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("opening %s: %v", name, err)
		}
		defer func() {
			if cerr := rc.Close(); cerr != nil {
				t.Errorf("closing %s: %v", name, cerr)
			}
		}()
		body, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		return string(body)
	}
	t.Fatalf("the archive has no %s", name)
	return ""
}
