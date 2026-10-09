// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/compose/agentbundle"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func bundleRequest(actor principal.Principal) *http.Request {
	ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())
	ctx = principal.WithActor(ctx, actor)
	return httptest.NewRequest(http.MethodGet, "/v1/agent-bundle", nil).WithContext(ctx)
}

func TestTheSkillBundleIsRefusedToAnAgent(t *testing.T) {
	h := newAgentBundleHandlers(agentAPIOrigin{public: "https://crm.example.test"}, slog.New(slog.DiscardHandler))
	w := httptest.NewRecorder()
	h.DownloadAgentSkillBundle(w, bundleRequest(principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:probe", UserID: ids.NewV7(), SeatType: principal.SeatFull,
	}))
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403: the bundle is human-only", w.Code)
	}
}

func TestTheSkillBundleIsAZipNamingTheAPIBase(t *testing.T) {
	h := newAgentBundleHandlers(agentAPIOrigin{public: "https://crm.example.test/"}, slog.New(slog.DiscardHandler))
	w := httptest.NewRecorder()
	h.DownloadAgentSkillBundle(w, bundleRequest(principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:ada", UserID: ids.NewV7(), SeatType: principal.SeatFull,
	}))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "application/zip" {
		t.Errorf("Content-Type = %q, want application/zip", got)
	}
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store: the answer may carry this request's own origin", got)
	}
	if spec := bundledSpec(t, w.Body.Bytes()); !strings.Contains(spec, "url: https://crm.example.test/v1\n") {
		t.Error("openapi.yaml does not name https://crm.example.test/v1 as its server")
	}
}

func TestThePassportBaseIsTheAPIsThenThePublicThenTheRequestsOrigin(t *testing.T) {
	plain := httptest.NewRequest(http.MethodGet, "http://arrived.example.test/v1/passports", nil)
	secure := httptest.NewRequest(http.MethodGet, "https://arrived.example.test/v1/passports", nil)
	secure.TLS = &tls.ConnectionState{}
	forwarded := httptest.NewRequest(http.MethodGet, "http://arrived.example.test/v1/passports", nil)
	forwarded.Header.Set("X-Forwarded-Proto", "https")
	for _, tc := range []struct {
		name   string
		origin agentAPIOrigin
		r      *http.Request
		want   string
	}{
		{"the API's own base wins", agentAPIOrigin{api: "https://api.example.test/", public: "https://app.example.test"}, plain, "https://api.example.test/v1"},
		{"the public base serves the API on one origin", agentAPIOrigin{public: "https://app.example.test"}, plain, "https://app.example.test/v1"},
		{"no base: the request's origin", agentAPIOrigin{}, plain, "http://arrived.example.test/v1"},
		{"no base, over TLS", agentAPIOrigin{}, secure, "https://arrived.example.test/v1"},
		{"no base, behind a TLS proxy", agentAPIOrigin{}, forwarded, "https://arrived.example.test/v1"},
	} {
		if got := tc.origin.baseFor(tc.r); got != tc.want {
			t.Errorf("%s: baseFor = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// bundledSpec returns the openapi.yaml of a bundle.
func bundledSpec(t *testing.T, archive []byte) string {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatalf("not a ZIP: %v", err)
	}
	for _, f := range reader.File {
		if f.Name != agentbundle.Folder+"/openapi.yaml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("opening openapi.yaml: %v", err)
		}
		body, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("reading openapi.yaml: %v", err)
		}
		if err := rc.Close(); err != nil {
			t.Fatalf("closing openapi.yaml: %v", err)
		}
		return string(body)
	}
	t.Fatal("the bundle has no openapi.yaml")
	return ""
}

// passportOperation is one operation of api/crm.yaml as the router sees it.
type passportOperation struct {
	id, method, route string
	security          []map[string][]string
}

// passportOperations reads every operation of api/crm.yaml, and the document's
// default security.
func passportOperations(t *testing.T) ([]passportOperation, []map[string][]string) {
	t.Helper()
	src, err := os.ReadFile("../../api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	var doc struct {
		Security []map[string][]string           `yaml:"security"`
		Paths    map[string]map[string]yaml.Node `yaml:"paths"`
	}
	if err := yaml.Unmarshal(src, &doc); err != nil {
		t.Fatalf("parsing the contract: %v", err)
	}
	var ops []passportOperation
	for path, item := range doc.Paths {
		for _, key := range httpMethods {
			node, present := item[key]
			if !present {
				continue
			}
			method := strings.ToUpper(key)
			var op struct {
				ID       string                 `yaml:"operationId"`
				Security *[]map[string][]string `yaml:"security"`
			}
			if err := node.Decode(&op); err != nil {
				t.Fatalf("%s %s: %v", method, path, err)
			}
			entry := passportOperation{id: op.ID, method: method, route: "/v1" + path, security: doc.Security}
			if op.Security != nil {
				entry.security = *op.Security
			}
			ops = append(ops, entry)
		}
	}
	return ops, doc.Security
}

// The skill's contract carries the operations a passport can call: the ones
// the production agent gate admits whose security accepts a bearer passport.
// Derived from crm.yaml and the gate, never from the generator's own filter,
// so a generator that drops or keeps one too many fails here.
func TestTheSkillListsEveryOperationAPassportCanCall(t *testing.T) {
	ops, _ := passportOperations(t)
	routes := map[string]bool{}
	want := map[string]bool{}
	for _, op := range ops {
		routes[op.method+" "+op.route] = true
		_, admitted := routeAdmitsAgents(op.method, op.route)
		if admitted && acceptsPassport(op.security) {
			want[op.id] = true
		}
	}
	for route := range agentPolicies {
		if !routes[route] {
			t.Fatalf("the contract walk did not reach %s, which the policy table carries; the census is short", route)
		}
	}

	archive, err := (&agentbundle.Builder{}).Build("https://crm.example.test/v1")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	got := operationIDsIn(t, bundledSpec(t, archive))
	for id := range want {
		if !got[id] {
			t.Errorf("%s is callable by a passport and the skill does not list it", id)
		}
	}
	for id := range got {
		if !want[id] {
			t.Errorf("the skill lists %s, which the agent gate or its security refuses a passport", id)
		}
	}
}

func acceptsPassport(security []map[string][]string) bool {
	for _, requirement := range security {
		if _, ok := requirement["bearerAuth"]; ok {
			return true
		}
	}
	return false
}

func operationIDsIn(t *testing.T, spec string) map[string]bool {
	t.Helper()
	var doc struct {
		Paths map[string]map[string]yaml.Node `yaml:"paths"`
	}
	if err := yaml.Unmarshal([]byte(spec), &doc); err != nil {
		t.Fatalf("parsing the skill's contract: %v", err)
	}
	ids := map[string]bool{}
	for path, item := range doc.Paths {
		for _, method := range httpMethods {
			node, present := item[method]
			if !present {
				continue
			}
			var op struct {
				ID string `yaml:"operationId"`
			}
			if err := node.Decode(&op); err != nil {
				t.Fatalf("%s %s: %v", method, path, err)
			}
			ids[op.ID] = true
		}
	}
	if len(ids) == 0 {
		t.Fatal("the skill's contract lists no operation")
	}
	return ids
}

// sortedIDs is for a stable failure message.
func sortedIDs(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
