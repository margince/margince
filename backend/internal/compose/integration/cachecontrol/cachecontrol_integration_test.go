// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package cachecontrol

// Every read the running application serves under /v1 tells caches not to keep
// it. The census drives the composed handler. So it also covers the session
// middleware's own answers and the extension routes.
//
// This is its own package because compose.RegisterExtensions is process-wide.
// The synthetic unit would otherwise show up in the parent package's censuses.

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"slices"
	"sync"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/pkg/extension"
)

const wantCache = "private, no-store"

// readVerb is a synthetic extension read, served on GET like a unit's list verb.
var readVerb = extension.Verb{
	Unit:           "cacheext",
	Contract:       "crm.yaml",
	OperationID:    "cacheextList",
	Route:          "/ext/cacheext/items",
	Method:         http.MethodGet,
	Tool:           "cacheext_list",
	Description:    "A synthetic read, composed only for this suite's own test binary.",
	Version:        "1.0.0",
	Tier:           extension.TierAutoExecute,
	RequestedScope: extension.ScopeRead,
}

func listItems(context.Context, extension.Runtime, json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`{"items":[]}`), nil
}

var (
	registerOnce sync.Once
	registerErr  error
)

func boot(t *testing.T) *apptest.AppEnv {
	t.Helper()
	registerOnce.Do(func() {
		registerErr = compose.RegisterExtensions([]extension.Extension{{
			Name:        "cacheext",
			Version:     "1.0.0",
			Description: "A synthetic read unit for the Cache-Control census.",
			Tools:       []extension.Tool{{Name: "cacheext_list", Handle: listItems}},
		}}, []extension.Verb{readVerb}, nil)
	})
	if registerErr != nil {
		t.Fatalf("registering the synthetic extension: %v", registerErr)
	}
	e := apptest.SetupApp(t)
	compose.BindExtensionRuntime(e.Pool, e.Vault)
	apptest.BootstrapWorkspaceSession(t, e, "Cache Census", "cache@census.test", "Admin")
	return e
}

var pathParam = regexp.MustCompile(`\{[^}]+\}`)

// readPaths lists the GET routes under /v1 from two sources: the contract's
// operations and the composed extension reads. It fails when either source is
// empty, because an empty census would pass over every read.
func readPaths(t *testing.T) []string {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromFile("../../../../api/crm.yaml")
	if err != nil {
		t.Fatalf("loading the contract: %v", err)
	}
	var paths []string
	for path, item := range doc.Paths.Map() {
		if item.Get != nil {
			paths = append(paths, httpserver.BaseURL+path)
		}
	}
	contractReads := len(paths)
	for _, verb := range compose.ComposedVerbs() {
		if verb.Method == http.MethodGet {
			paths = append(paths, verb.ServedPath())
		}
	}
	if contractReads == 0 || len(paths) == contractReads {
		t.Fatalf("the census holds %d contract reads and %d extension reads; both must be present",
			contractReads, len(paths)-contractReads)
	}
	slices.Sort(paths)
	return paths
}

func cacheControlOf(t *testing.T, client *http.Client, url string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("building GET %s: %v", url, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatalf("closing GET %s: %v", url, err)
	}
	return resp.StatusCode, resp.Header.Get("Cache-Control")
}

// repSeat signs in a rep, whose row scope is the seeded role's. The census then
// also walks what a bounded reader gets: 403s, and 404s for rows out of scope.
func repSeat(t *testing.T, e *apptest.AppEnv) *apptest.AppEnv {
	t.Helper()
	// An invitation needs the company described first.
	e.DescribeCompany(t)
	var invited struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, http.MethodPost, "/v1/users", map[string]any{
		"email": "rep@census.test", "display_name": "A Rep", "role": "rep",
	}, nil, &invited); status != http.StatusCreated {
		t.Fatalf("invite a rep → %d, want 201", status)
	}
	const password = "Test-cache-census-rep-password-41!"
	apptest.SetPasswordByLink(t, e, invited.ID, password)
	return e.AsSecondSeat(t, "rep@census.test", password)
}

func TestEveryReadUnderV1AnswersPrivateAndUnstored(t *testing.T) {
	e := boot(t)
	paths := readPaths(t)
	for reader, seat := range map[string]*apptest.AppEnv{"the admin": e, "a rep": repSeat(t, e)} {
		for _, route := range paths {
			path := pathParam.ReplaceAllString(route, "00000000-0000-0000-0000-000000000001")
			status, got := cacheControlOf(t, seat.Client, e.TS.URL+path)
			if got == "" {
				t.Errorf("GET %s as %s answered %d with no Cache-Control", route, reader, status)
			}
		}
	}
	status, got := cacheControlOf(t, e.Client, e.TS.URL+readVerb.ServedPath())
	if status != http.StatusOK || got != wantCache {
		t.Errorf("the extension read answered %d with Cache-Control %q, want 200 and %q", status, got, wantCache)
	}
}

func TestARefusalForNoSessionIsUnstored(t *testing.T) {
	e := boot(t)
	// A client of its own: the server's shared client carries the admin's cookie jar.
	anonymous := &http.Client{Transport: e.TS.Client().Transport}
	status, got := cacheControlOf(t, anonymous, e.TS.URL+"/v1/contacts")
	if status != http.StatusUnauthorized || got != wantCache {
		t.Fatalf("GET /v1/contacts with no session answered %d with Cache-Control %q, want 401 and %q",
			status, got, wantCache)
	}
}
