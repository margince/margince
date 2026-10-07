// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package apptest_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func TestRebootServesTheSameDatabaseThroughAFreshPool(t *testing.T) {
	before := apptest.SetupApp(t)
	before.BootstrapWorkspace(t)
	var contact struct {
		ID string `json:"id"`
	}
	if status := before.Call(t, "POST", "/v1/contacts", map[string]string{"full_name": "Survives Restart"}, nil, &contact); status != http.StatusCreated {
		t.Fatalf("creating contact → %d", status)
	}

	after := before.Reboot(t)

	if after.Pool == before.Pool {
		t.Fatal("Reboot reused the app pool: its pooled connections are not the empty ones a restart starts with")
	}
	if after.TS.URL == before.TS.URL {
		t.Fatalf("Reboot served from the old server %s", before.TS.URL)
	}
	if status := after.Call(t, "GET", "/v1/contacts/"+contact.ID, nil, nil, nil); status != http.StatusOK {
		t.Fatalf("reading the contact after Reboot → %d, want 200: the database was not kept", status)
	}
	// A seat client built for the first server carries its session over, as
	// a browser does across a restart: the jar keys cookies by host, not port.
	if status := getStatus(t, before.Client, after.TS.URL+"/v1/contacts/"+contact.ID); status != http.StatusOK {
		t.Fatalf("the pre-reboot client reading through the new server → %d, want 200", status)
	}
	if status := before.Call(t, "GET", "/v1/contacts/"+contact.ID, nil, nil, nil); status != http.StatusOK {
		t.Fatalf("the original env after Reboot → %d, want 200", status)
	}
}

func getStatus(t *testing.T, client *http.Client, url string) int {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	apptest.CloseBody(t, resp)
	return resp.StatusCode
}
