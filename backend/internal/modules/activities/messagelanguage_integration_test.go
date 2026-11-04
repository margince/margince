// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAHandLoggedNoteStoresTheLanguageItIsWrittenIn(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	subject := "Gespräch mit dem Einkauf"
	body := "Wir haben über die Verlängerung gesprochen. Sie möchten das Angebot bis Freitag sehen, " +
		"und der Einkauf ist nicht mehr gegen den Preis."
	in, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{
		Kind: "note", Subject: &subject, Body: &body, Source: "manual",
	})
	if err != nil {
		t.Fatalf("LogActivityInputFrom: %v", err)
	}
	logged, _, err := e.store(nil).LogActivity(ctx, in)
	if err != nil {
		t.Fatalf("LogActivity: %v", err)
	}
	if logged.Language == nil || *logged.Language != crmcontracts.ActivityLanguageDe {
		t.Fatalf("a German note stores language %v, want de", logged.Language)
	}

	english := "We talked about the renewal. They want to see the offer by Friday, and purchasing is " +
		"no longer against the price."
	edited, err := e.store(nil).UpdateActivity(ctx, ids.From[ids.ActivityKind](ids.UUID(logged.Id)),
		UpdateActivityInput{Body: &english, Subject: new("Call with purchasing")})
	if err != nil {
		t.Fatalf("UpdateActivity: %v", err)
	}
	if edited.Language == nil || *edited.Language != crmcontracts.ActivityLanguageEn {
		t.Fatalf("a note rewritten in English carries language %v, want en", edited.Language)
	}

	unplaced := "ok"
	cleared, err := e.store(nil).UpdateActivity(ctx, ids.From[ids.ActivityKind](ids.UUID(logged.Id)),
		UpdateActivityInput{Body: &unplaced, Subject: &unplaced})
	if err != nil {
		t.Fatalf("UpdateActivity: %v", err)
	}
	if cleared.Language != nil {
		t.Fatalf("a note whose words name no language still carries %v", *cleared.Language)
	}
}
