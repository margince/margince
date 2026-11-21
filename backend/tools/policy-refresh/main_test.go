// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/messagingpolicy"
	"github.com/margince/margince/backend/internal/platform/outbound"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const releaseCheck = "../../../scripts/release-policy-sources-fresh.sh"

// Go's test cache never rechecks a file outside this module. Without this, a
// replayed pass would outlive a change to the release check these tests run.
func TestTheTestCacheKeysOnTheScriptsOutsideThisModule(t *testing.T) {
	if err := gatekit.DeclareInputs(os.Getenv, "../../../scripts"); err != nil {
		t.Fatal(err)
	}
}

var refreshedOn = time.Date(2026, 3, 9, 8, 0, 0, 0, time.UTC)

const layer = `---
jurisdiction: aa
version: 1
created: 2026-01-02
instruments:
  - name: Act A
    source-url: https://example.org/a
  - name: Act B
    source-url: https://example.org/b
fixtures:
  - finding: objection_route
    triggers: fixtures/a.md
    clears: fixtures/b.md
---

Why the layer says so.
`

// repository plants one unit with one pack under a temporary root.
func repository(t *testing.T) (root, layerPath string) {
	t.Helper()
	root = t.TempDir()
	policy := filepath.Join(root, "extensions", "unit", "policy")
	for name, text := range map[string]string{
		"index.md":      "---\nlayers: [aa.md]\n---\nThe layers.\n",
		"aa.md":         layer,
		"fixtures/a.md": "---\nadvertising: true\n---\nHello\n",
		"fixtures/b.md": "---\nadvertising: true\n---\nHello\n",
	} {
		path := filepath.Join(policy, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root, filepath.Join(policy, "aa.md")
}

// published serves each address its law text, and refuses one it does not hold.
func published(texts map[string]string) messagingpolicy.FetchSource {
	return func(_ context.Context, address string) (messagingpolicy.SourcePage, error) {
		text, held := texts[address]
		if !held {
			return messagingpolicy.SourcePage{}, errors.New("the host answered 403")
		}
		body := "<html><body><p>" + text + strings.Repeat(" No message without consent.", 30) + "</p></body></html>"
		return messagingpolicy.SourcePage{ContentType: "text/html", Body: []byte(body)}, nil
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path) // #nosec G304 -- a file this test planted
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestARefreshStampsTheLayersAndReportsEachSource(t *testing.T) {
	root, layerPath := repository(t)
	var out bytes.Buffer
	both := published(map[string]string{"https://example.org/a": "Act A", "https://example.org/b": "Act B"})
	if err := run(t.Context(), &out, root, false, both, refreshedOn); err != nil {
		t.Fatal(err)
	}
	stamped := read(t, layerPath)
	if strings.Count(stamped, "    text-fetched: 2026-03-09\n") != 2 || !strings.Contains(stamped, "\nsource-checked: 2026-03-09\n") {
		t.Errorf("the layer was not stamped:\n%s", stamped)
	}
	if !strings.HasSuffix(stamped, "---\n\nWhy the layer says so.\n") {
		t.Errorf("the stamp moved the body:\n%s", stamped)
	}
	for _, want := range []string{"aa.md\n", "first digest  Act A\n", "https://example.org/b  (", "   2  first digest\n", "gen-messaging-rules"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the report lacks %q:\n%s", want, out.String())
		}
	}
}

func TestAReportWritesNothingAndNamesWhatItCouldNotRead(t *testing.T) {
	root, layerPath := repository(t)
	var out bytes.Buffer
	onlyA := published(map[string]string{"https://example.org/a": "Act A"})
	if err := run(t.Context(), &out, root, true, onlyA, refreshedOn); err != nil {
		t.Fatal(err)
	}
	if read(t, layerPath) != layer {
		t.Error("a report changed the layer")
	}
	for _, want := range []string{"not read      Act B\n", "(the host answered 403)", "   1  not read\n", "text-url"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the report lacks %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "gen-messaging-rules") {
		t.Error("a report that wrote nothing asks for a regeneration")
	}
}

func TestAChangedSourceIsNamedWithWhatToDo(t *testing.T) {
	root, _ := repository(t)
	texts := map[string]string{"https://example.org/a": "Act A", "https://example.org/b": "Act B"}
	if err := run(t.Context(), &bytes.Buffer{}, root, false, published(texts), refreshedOn); err != nil {
		t.Fatal(err)
	}
	texts["https://example.org/b"] = "Act B, as amended"
	var out bytes.Buffer
	if err := run(t.Context(), &out, root, true, published(texts), refreshedOn.AddDate(0, 1, 0)); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"unchanged     Act A\n", "changed       Act B\n", "Read each changed source"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the report lacks %q:\n%s", want, out.String())
		}
	}
}

func TestARootWithNoPolicyFolderOrABrokenPackFails(t *testing.T) {
	if err := run(t.Context(), &bytes.Buffer{}, t.TempDir(), true, published(nil), refreshedOn); err == nil {
		t.Error("a root with no policy folder read as a clean refresh")
	}
	root, layerPath := repository(t)
	if err := os.WriteFile(layerPath, []byte("---\nno-such-key: 1\n---\nWhy.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run(t.Context(), &bytes.Buffer{}, root, true, published(nil), refreshedOn); err == nil {
		t.Error("a pack that does not compile was refreshed")
	}
}

// The release check and the refresh are two sides of one date: what one
// writes, the other must read.
func TestTheReleaseCheckReadsTheDateARefreshWrites(t *testing.T) {
	root, _ := repository(t)
	both := published(map[string]string{"https://example.org/a": "Act A", "https://example.org/b": "Act B"})
	if err := run(t.Context(), &bytes.Buffer{}, root, false, both, refreshedOn); err != nil {
		t.Fatal(err)
	}
	check := func(tagDate string) error {
		//nolint:gosec // the repository's own release check, on a planted tree
		return exec.CommandContext(t.Context(), "bash", releaseCheck, "v1.0.0", tagDate, "false",
			filepath.Join(root, "extensions")).Run()
	}
	if err := check("2026-03-20"); err != nil {
		t.Errorf("a tag eleven days after the refresh was refused: %v", err)
	}
	if err := check("2026-09-01"); err == nil {
		t.Error("a tag half a year after the refresh was accepted")
	}
}

func TestTheHTTPSourceReadsAPageAndRefusesAnErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/law" || r.Header.Get("User-Agent") != outbound.PolicySourceHeader {
			http.Error(w, "refused", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		if _, err := w.Write([]byte("the act")); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	fetch := httpSource(server.Client())

	page, err := fetch(t.Context(), server.URL+"/law")
	if err != nil || string(page.Body) != "the act" || page.ContentType != "application/pdf" {
		t.Errorf("page %+v, error %v", page, err)
	}
	if _, err := fetch(t.Context(), server.URL+"/elsewhere"); err == nil || !strings.Contains(err.Error(), "403") {
		t.Errorf("a refused fetch reads %v, want the status it answered", err)
	}
	if _, err := fetch(t.Context(), "://no address"); err == nil {
		t.Error("a string that is no address was fetched")
	}
	server.Close()
	if _, err := fetch(t.Context(), server.URL+"/law"); err == nil {
		t.Error("a host that is gone read as a page")
	}
}
