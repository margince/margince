// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gmail

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func labelStub(t *testing.T, body map[string]any) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/labels", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, body) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// Both kinds, because both are places mail sits: CATEGORY_PROMOTIONS is
// Gmail's and "Privat" is the owner's, and a picker offering only the second
// leaves the noisiest folders unexcludable.
func TestListLabelsReturnsSystemAndUserLabelsAlike(t *testing.T) {
	srv := labelStub(t, map[string]any{"labels": []map[string]string{
		{"id": "CATEGORY_PROMOTIONS", "name": "Promotions", "type": "system"},
		{"id": "Label_7", "name": "Privat", "type": "user"},
	}})

	got, err := NewAPI(srv.Client(), srv.URL).ListLabels(context.Background(), "access-1")
	if err != nil {
		t.Fatalf("ListLabels: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d labels, want both the system and the user one: %+v", len(got), got)
	}
	if got[0].ID != "CATEGORY_PROMOTIONS" || got[0].Name != "Promotions" {
		t.Errorf("first = %+v, want the system label's id and display name", got[0])
	}
	if got[1].ID != "Label_7" || got[1].Name != "Privat" {
		t.Errorf("second = %+v, want the user label's opaque id and its readable name", got[1])
	}
}

// A label with no name falls back to its id rather than travelling empty: a
// picker row showing nothing is worse than one showing an opaque token.
func TestAnUnnamedLabelFallsBackToItsID(t *testing.T) {
	srv := labelStub(t, map[string]any{"labels": []map[string]string{
		{"id": "Label_9", "name": ""},
	}})

	got, err := NewAPI(srv.Client(), srv.URL).ListLabels(context.Background(), "access-1")
	if err != nil {
		t.Fatalf("ListLabels: %v", err)
	}
	if len(got) != 1 || got[0].Name != "Label_9" {
		t.Fatalf("got %+v, want the id standing in for the missing name", got)
	}
}

// A label with no id is dropped: a rule keyed on nothing would match nothing,
// and offering it is offering a choice that excludes no mail.
func TestALabelWithNoIDIsDropped(t *testing.T) {
	srv := labelStub(t, map[string]any{"labels": []map[string]string{
		{"id": "", "name": "Ghost"},
		{"id": "Label_1", "name": "Real"},
	}})

	got, err := NewAPI(srv.Client(), srv.URL).ListLabels(context.Background(), "access-1")
	if err != nil {
		t.Fatalf("ListLabels: %v", err)
	}
	if len(got) != 1 || got[0].ID != "Label_1" {
		t.Fatalf("got %+v, want only the label a rule could name", got)
	}
}

// The connector's own verb: it opens the auth state, mints a token and asks.
func TestListContainersReadsTheMailboxsLabels(t *testing.T) {
	api := &fakeAPI{labels: []connector.NamedContainer{{ID: "Label_3", Name: "Familie"}}}
	c := New(fakeOAuth{access: "access-1"}, api)

	got, err := c.ListContainers(context.Background(), authBytes(t))
	if err != nil {
		t.Fatalf("ListContainers: %v", err)
	}
	if len(got) != 1 || got[0].Name != "Familie" {
		t.Fatalf("got %+v, want the mailbox's own label", got)
	}
}

// Malformed auth is a fault the caller must see rather than an empty list: an
// empty picker reads as "this mailbox has no folders", which is a different and
// wrong answer.
func TestListContainersRefusesMalformedAuth(t *testing.T) {
	c := New(fakeOAuth{access: "access-1"}, &fakeAPI{})
	if _, err := c.ListContainers(context.Background(), []byte("not json")); err == nil {
		t.Fatal("a malformed auth bundle listed containers instead of failing")
	}
}
