// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// What a merge tells the surviving record's history, read through the real
// history read. The value the survivor took obeys the field mask like any
// other column. The event stays on the archived record.

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const foldedInTitle = "Chief Merge Officer"

// mergedPair merges a contact holding a title into a blank survivor, as admin.
func mergedPair(t *testing.T, e *Env) (survivor, source ids.UUID) {
	t.Helper()
	title := foldedInTitle
	taker, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Ada Survivor", Source: "manual"})
	if err != nil {
		t.Fatalf("creating the survivor: %v", err)
	}
	giver, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "A. Survivor", Title: &title, Source: "manual"})
	if err != nil {
		t.Fatalf("creating the contact to fold in: %v", err)
	}
	survivor, source = ids.UUID(taker.Id), ids.UUID(giver.Id)
	if _, err := e.Contacts.MergeContact(e.Admin(),
		ids.From[ids.ContactKind](source), ids.From[ids.ContactKind](survivor), nil); err != nil {
		t.Fatalf("merging: %v", err)
	}
	return survivor, source
}

func mergeEntryOf(t *testing.T, reader context.Context, e *Env, record ids.UUID) privacy.RecordHistoryEntry {
	t.Helper()
	page, err := privacy.ListRecordHistory(reader, e.DB(), privacy.RecordHistoryFilter{EntityType: "contact", EntityID: record})
	if err != nil {
		t.Fatalf("reading the history: %v", err)
	}
	for _, entry := range page.Entries {
		if entry.Action == "merge" {
			return entry
		}
	}
	t.Fatalf("the history holds no merge entry: %+v", page.Entries)
	return privacy.RecordHistoryEntry{}
}

func TestTheSurvivorsHistoryTellsWhatItTookAndHonoursTheFieldMask(t *testing.T) {
	e := Setup(t)
	survivor, source := mergedPair(t, e)

	admin := mergeEntryOf(t, e.Admin(), e, survivor)
	if admin.After["merged_from_id"] != source.String() {
		t.Errorf("the entry names %v as the record folded in, want %s", admin.After["merged_from_id"], source)
	}
	if admin.After["title"] != foldedInTitle {
		t.Errorf("the entry holds title %v, want the filled %q as its own key", admin.After["title"], foldedInTitle)
	}

	perms := RepPerms
	perms.FieldMasks = []principal.FieldMask{{Object: "contact", Field: "title", Condition: principal.MaskAlways}}
	masked := mergeEntryOf(t, e.As(e.Rep1, []ids.UUID{e.Team1}, perms), e, survivor)
	encoded, err := json.Marshal(masked)
	if err != nil {
		t.Fatalf("encoding the entry: %v", err)
	}
	if _, leaked := masked.After["title"]; leaked || containsText(encoded, foldedInTitle) {
		t.Errorf("a role masked from title reads %q in the survivor's merge entry: %s", foldedInTitle, encoded)
	}

	archived := mergeEntryOf(t, e.Admin(), e, source)
	if encoded, err := json.Marshal(archived); err != nil || containsText(encoded, foldedInTitle) {
		t.Errorf("the archived record's merge entry carries the filled value (%v): %s", err, encoded)
	}
}

func TestAMergeEmitsOneMergedEventWhateverRowsItAudits(t *testing.T) {
	e := Setup(t)
	_, source := mergedPair(t, e)

	var merged int
	if err := OwnerConn(t).QueryRow(t.Context(), `SELECT count(*) FROM event_outbox
		WHERE envelope->>'type' = 'contact.merged' AND envelope::text LIKE '%' || $1 || '%'`,
		source.String()).Scan(&merged); err != nil {
		t.Fatalf("reading the outbox: %v", err)
	}
	if merged != 1 {
		t.Errorf("the outbox holds %d contact.merged events for the merge, want exactly 1", merged)
	}
}

func containsText(encoded []byte, text string) bool { return bytes.Contains(encoded, []byte(text)) }
