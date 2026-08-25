// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What language the tool surface tells an agent to write stored prose in.
//
// The answer must never be empty, because the failure it replaces is an agent
// with no instruction inferring a language from the workspace around it: an
// English conversation in an English workspace produced German notes, since
// nobody had chosen a locale and nothing named the installation's own.

import (
	"context"
	"testing"

	"github.com/gradionhq/margince/backend/internal/compose/integration"
)

func TestTheAgentIsAlwaysToldWhichLanguageToWriteStoredProseIn(t *testing.T) {
	e := integration.Setup(t)
	read := actingIdentity(e.Pool)

	// A member who never chose a language is the common case, and the one the
	// German-note bug was found in. The installation's base language answers
	// for them rather than nothing at all.
	e.WsExec(t, `UPDATE app_user SET locale = NULL WHERE id = $1`, e.AdminUser)
	who, err := read(e.Admin())
	if err != nil {
		t.Fatalf("reading the acting identity: %v", err)
	}
	if who.Locale != "" {
		t.Errorf("locale is %q, want empty — a person who chose nothing must not read as a choice", who.Locale)
	}
	if who.ProseLanguage != "English" {
		t.Errorf("prose_language is %q for a member with no locale, want the installation's English", who.ProseLanguage)
	}

	// A member's own choice wins over the installation's, and is answered as a
	// language name rather than a code: the reader is a model writing a
	// sentence, not a lookup table.
	e.WsExec(t, `UPDATE app_user SET locale = 'de' WHERE id = $1`, e.AdminUser)
	who, err = read(e.Admin())
	if err != nil {
		t.Fatalf("reading the acting identity after choosing German: %v", err)
	}
	if who.ProseLanguage != "German" {
		t.Errorf("prose_language is %q for a member who chose de, want German", who.ProseLanguage)
	}
}

// A principal with no human behind it answers no identity at all, and the
// language must still be a language: the field is declared always-present, and
// a caller that reads it into a prompt would otherwise instruct the model in
// nothing.
func TestAnUnattributedCallStillNamesALanguage(t *testing.T) {
	e := integration.Setup(t)
	who, err := actingIdentity(e.Pool)(context.Background())
	if err != nil {
		t.Fatalf("reading the acting identity with no principal: %v", err)
	}
	if who.ProseLanguage == "" {
		t.Error("prose_language is empty for an unattributed call; the field is promised on every answer")
	}
}
