// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"fmt"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func suggestionScanFixture(t *testing.T) (*integration.Env, *capture.Sink, datasource.EntityRef) {
	t.Helper()
	e := integration.Setup(t)
	alice, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Alice Buyer", Source: "manual", Emails: []contacts.ContactEmailInput{{Email: "alice@buyer.test", EmailType: "work", IsPrimary: true}}})
	if err != nil {
		t.Fatal(err)
	}
	return e, newCaptureSink(e.Pool, CaptureConfig{}), datasource.EntityRef{Type: datasource.EntityContact, ID: ids.UUID(alice.Id)}
}

func captureSuggestionCopy(t *testing.T, e *integration.Env, sink *capture.Sink, address string) ids.ActivityID {
	t.Helper()
	rec := partiesMailRecord(t, e, partyMailProof{outbound: false, sentByOwner: false}, "Buyer <"+address+">", "", "Could we discuss the product requirements?")
	ref, err := sink.Upsert(threadConnectorCtx(e, e.AdminUser), rec)
	if err != nil {
		t.Fatal(err)
	}
	return ids.From[ids.ActivityKind](ref.ID)
}

func TestConversationSuggestionsFillTheVisiblePageAfterFilteringNewerMessages(t *testing.T) {
	e, sink, anchor := suggestionScanFixture(t)
	var newest []string
	for index := range 35 {
		address := fmt.Sprintf("buyer-%02d@buyer.test", index)
		captureSuggestionCopy(t, e, sink, address)
		newest = append([]string{address}, newest...)
	}
	for range 3 {
		captureSuggestionCopy(t, e, sink, "newest@buyer.test")
	}
	newest = append([]string{"newest@buyer.test"}, newest...)
	for range 10 {
		captureSuggestionCopy(t, e, sink, "blocked@private.test")
	}
	if _, err := capture.NewExclusionStore(InstallationDB(e.Pool)).Add(e.Admin(), capture.ExclusionScopeUser, capture.ExclusionKindAddress, "blocked@private.test"); err != nil {
		t.Fatal(err)
	}
	out, err := sink.ConversationSuggestions(e.Admin(), anchor)
	if err != nil {
		t.Fatal(err)
	}
	addresses := []string{}
	for _, suggestion := range out.Data {
		addresses = append(addresses, suggestion.Address)
	}
	if !slices.Equal(addresses, newest[:25]) {
		t.Fatalf("newest eligible addresses=%v, wanted %v", addresses, newest[:25])
	}
	if !out.ScanLimited {
		t.Fatal("a filled bounded list claimed that no older suggestions may exist")
	}
}

func TestConversationSuggestionsDiscloseNoAddressFromUnreadableMessageContent(t *testing.T) {
	e, sink, anchor := suggestionScanFixture(t)
	captureSuggestionCopy(t, e, sink, "bob@buyer.test")
	owner, err := sink.ConversationSuggestions(e.Admin(), anchor)
	if err != nil || len(owner.Data) != 1 {
		t.Fatalf("owner suggestions=%+v: %v", owner, err)
	}
	outsider := e.As(e.Rep3, []ids.UUID{e.Team2}, integration.AdminPerms)
	out, err := sink.ConversationSuggestions(outsider, anchor)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Data) != 0 {
		t.Fatalf("unreadable mail disclosed addresses: %+v", out)
	}
}

func TestConversationSuggestionsReportTheExhaustedScanBudget(t *testing.T) {
	e, sink, anchor := suggestionScanFixture(t)
	captureSuggestionCopy(t, e, sink, "older@buyer.test")
	// One copy is captured and the rest are copies of its rows. Each is a
	// moment newer than the last, so the scan reads them in a fixed order.
	blocked := captureSuggestionCopy(t, e, sink, "blocked@private.test")
	e.CloneActivity(t, blocked, 199, map[string]string{
		"created_at":   "r.created_at + n.ord * interval '1 millisecond'",
		"source_id":    "r.source_id || '-' || n.ord",
		"identity_key": "r.identity_key || '-' || n.ord",
	})
	if _, err := capture.NewExclusionStore(InstallationDB(e.Pool)).Add(e.Admin(), capture.ExclusionScopeUser, capture.ExclusionKindAddress, "blocked@private.test"); err != nil {
		t.Fatal(err)
	}
	out, err := sink.ConversationSuggestions(e.Admin(), anchor)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Data) != 0 || !out.ScanLimited {
		t.Fatalf("bounded scan=%+v", out)
	}
}
