// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

import (
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/modules/identity"
)

// A route the contract declares `security: []` is reachable without a session.
//
// The two sides fail silently apart. A handler test drives the handler directly, so
// it passes while the middleware in front of the route answers 401 to every caller:
// POST /v1/auth/mfa shipped that way, and the 202 from the first sign-in step was a
// dead end for anyone holding an authenticator.
//
// One direction, and that is the whole invariant rather than half of one: this
// middleware decides whether a CRM HUMAN SESSION is required, so a route it admits
// may still prove another credential downstream — every /v1/public/rooms operation
// declares `dealRoomSession` and is authenticated by the deal-room middleware
// composed after this one. Asserting the converse would demand that twenty
// correctly-layered routes be broken.
func TestEveryPreAuthOperationIsAdmittedWithoutASession(t *testing.T) {
	t.Parallel()
	for _, op := range preAuthOperations(t) {
		probe := httptest.NewRequest(op.method, concretePath(op.path), nil)
		if !identity.AdmittedWithoutSession(probe) {
			t.Errorf("the contract declares %s %s pre-auth, but the admission rule requires a session: the route is unreachable in production however well its handler is tested", op.method, op.path)
		}
	}
}

// What a session-less admission means is proved here, so the gate above cannot pass
// by admitting everything.
func TestAdmissionStillRequiresASessionForOrdinaryRoutes(t *testing.T) {
	t.Parallel()
	for _, path := range []string{
		"/v1/contacts",
		"/v1/me/mfa/totp",
		// A deeper path under an admitted shape: neither the OIDC nor the connector
		// matcher may widen to one.
		"/v1/auth/oidc/google/start/extra",
		"/v1/connectors/google/callback/extra",
		// The consent DECISION lends the signed-in human's own authority.
		"/oauth/authorize",
	} {
		if identity.AdmittedWithoutSession(httptest.NewRequest(http.MethodPost, path, nil)) {
			t.Errorf("POST %s is admitted with no session, which the contract does not declare pre-auth", path)
		}
	}
}

// preAuthRoute is one operation crm.yaml declares with an empty security list.
type preAuthRoute struct {
	path   string
	method string
}

var preAuthPathTemplate = regexp.MustCompile(`\{[^}]*\}`)

// preAuthOperations reads the operations the contract declares need no credential.
// An empty security list says exactly that, while an absent one inherits the
// document default, so the empty list has to be told from a missing one.
//
// Derived from the contract, so a route added there joins this gate's corpus with
// nobody remembering to list it. Decoded rather than scanned, and with no vocabulary
// of HTTP verbs to leave one out of: a path item mixes operations with scalar keys
// (summary, description) and sequence keys (servers, shared parameters), and it is
// being a MAPPING that makes a key an operation.
func preAuthOperations(t *testing.T) []preAuthRoute {
	t.Helper()
	src, err := os.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatalf("reading api/crm.yaml: %v", err)
	}
	var doc struct {
		Paths map[string]map[string]yaml.Node `yaml:"paths"`
	}
	if err := yaml.Unmarshal(src, &doc); err != nil {
		t.Fatalf("parsing api/crm.yaml: %v", err)
	}
	// Under-recognition is the one way this gate must not break: a reader that finds
	// no operations asserts nothing and reports PASS.
	if len(doc.Paths) == 0 {
		t.Fatal("api/crm.yaml has no top-level paths map — the contract failed to parse as expected")
	}
	var routes []preAuthRoute
	for path, item := range doc.Paths {
		for key, node := range item {
			if node.Kind != yaml.MappingNode {
				continue
			}
			var op struct {
				Security *[]map[string][]string `yaml:"security"`
			}
			if err := node.Decode(&op); err != nil {
				t.Fatalf("api/crm.yaml %s %s: the operation does not decode as an OpenAPI operation: %v", key, path, err)
			}
			if op.Security != nil && len(*op.Security) == 0 {
				routes = append(routes, preAuthRoute{path: path, method: strings.ToUpper(key)})
			}
		}
	}
	if len(routes) == 0 {
		t.Fatal("api/crm.yaml declares no operation pre-auth, which would mean sign-in itself needs a session: the reader has drifted from the contract")
	}
	return routes
}

// concretePath fills a template segment, because the admission rule matches a single
// segment and a literal `{provider}` would pass a test the real request fails.
func concretePath(path string) string {
	return "/v1" + preAuthPathTemplate.ReplaceAllString(path, "probe")
}
