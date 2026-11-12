// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A contact's top-level linkedin_url is the social.linkedin row under a second
// name: written through either, read back under both, and seen by the
// duplicate detection.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (e *dedupeEnv) createOverTheWire(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	Handlers{store: e.store}.CreateContact(rec, httptest.NewRequestWithContext(e.as(), http.MethodPost, "/v1/contacts",
		strings.NewReader(body)), crmcontracts.CreateContactParams{})
	return rec
}

func TestALinkedInURLWriteReadsBackUnderBothNames(t *testing.T) {
	e := setupDedupe(t)
	rec := e.createOverTheWire(t, `{"full_name":"Lina Url","source":"manual","linkedin_url":"http://linkedin.com/in/lina-url/"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create with linkedin_url answered %d %s", rec.Code, rec.Body.String())
	}
	var created crmcontracts.Contact
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	const stored = "https://www.linkedin.com/in/lina-url"
	if created.LinkedinUrl == nil || *created.LinkedinUrl != stored || created.Social == nil || (*created.Social)[SocialLinkedIn] != stored {
		t.Fatalf("read back linkedin_url %v and social %v, want %s under both", created.LinkedinUrl, created.Social, stored)
	}

	// A null linkedin_url removes the one row.
	patch := httptest.NewRecorder()
	Handlers{store: e.store}.UpdateContact(patch, httptest.NewRequestWithContext(e.as(), http.MethodPatch, "/v1/contacts/"+created.Id.String(),
		strings.NewReader(`{"linkedin_url":null}`)), crmcontracts.Id(created.Id), crmcontracts.UpdateContactParams{})
	if patch.Code != http.StatusOK {
		t.Fatalf("clearing linkedin_url answered %d %s", patch.Code, patch.Body.String())
	}
	if got := storedLinkedinHandle(e.as(), t, e, ids.From[ids.ContactKind](ids.UUID(created.Id))); got != "" {
		t.Errorf("after a null linkedin_url the slot holds %q, want it removed", got)
	}
}

func TestLinkedInURLAndSocialLinkedInNamingTwoProfilesAreRefused(t *testing.T) {
	e := setupDedupe(t)
	rec := e.createOverTheWire(t, `{"full_name":"Two Profiles","source":"manual",`+
		`"linkedin_url":"https://www.linkedin.com/in/one","social":{"linkedin":"https://www.linkedin.com/in/two"}}`)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "linkedin_url_conflicts_with_social") {
		t.Fatalf("two profiles answered %d %s, want 422 linkedin_url_conflicts_with_social", rec.Code, rec.Body.String())
	}
}

func TestADuplicateWrittenThroughLinkedInURLIsFiledAsATwin(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	first := e.createWithLinkedIn(ctx, t, "Tove Twin", "tove@example.com", "https://www.linkedin.com/in/tove-twin")
	rec := e.createOverTheWire(t, `{"full_name":"T. Twin","source":"manual","linkedin_url":"https://de.linkedin.com/in/tove-twin/"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create with linkedin_url answered %d %s", rec.Code, rec.Body.String())
	}
	var second crmcontracts.Contact
	if err := json.Unmarshal(rec.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	if n := e.openLinkedInPairs(ctx, t, first, ids.From[ids.ContactKind](ids.UUID(second.Id))); n != 1 {
		t.Errorf("open LinkedIn pairs = %d, want the twin written through linkedin_url filed", n)
	}
}
