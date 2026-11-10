// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestPreviewEvidenceReferencesRespectRecordAccessAndOmitTransientSubjects(t *testing.T) {
	e := setupSend(t)
	id := ids.NewV7()
	if _, err := e.owner.Exec(t.Context(), `INSERT INTO contact(id, full_name, source, captured_by, owner_id)
 VALUES($1,'Readable contact','manual','human:x',$2)`, id, e.rep); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(t.Context(), `INSERT INTO lead(id, full_name, source, captured_by, owner_id)
 VALUES($1,'Private lead','manual','human:x',$2)`, id, e.other); err != nil {
		t.Fatal(err)
	}
	ctx := e.as(principal.RowScopeOwn)
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("fixture has no principal")
	}
	actor.Permissions.Objects["lead"] = principal.ObjectGrant{Read: true}
	ctx = principal.WithActor(ctx, actor)
	decisions := []commsauthz.Decision{
		{SubjectKind: "contact", SubjectID: id, Recipient: connector.Recipient{Email: "visible@example.test"}},
		{SubjectKind: "lead", SubjectID: id, Recipient: connector.Recipient{Email: "hidden@example.test"}},
		{SubjectKind: "contact", SubjectID: ids.NewV7(), Recipient: connector.Recipient{Email: "new@example.test"}},
	}
	for _, tc := range []struct {
		name     string
		wantLead bool
		scope    principal.RowScope
		leadRead bool
	}{
		{"own rows", false, principal.RowScopeOwn, true},
		{"all rows", true, principal.RowScopeAll, true},
		{"no lead grant", false, principal.RowScopeAll, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actor.Permissions.RowScope = tc.scope
			actor.Permissions.Objects["lead"] = principal.ObjectGrant{Read: tc.leadRead}
			answer, err := e.store(nil).withReadablePreviewSubjects(principal.WithActor(ctx, actor), SendPreview{
				Decisions: commsauthz.DecisionSet{Decisions: decisions}, NewContacts: map[string]bool{"new@example.test": true},
			})
			if err != nil {
				t.Fatal(err)
			}
			wire := previewResponse(answer)
			if wire.Recipients[0].EvidenceSubject == nil {
				t.Fatal("readable contact reference missing")
			}
			if got := wire.Recipients[1].EvidenceSubject != nil; got != tc.wantLead {
				t.Fatalf("lead reference present=%v, want %v", got, tc.wantLead)
			}
			if wire.Recipients[2].EvidenceSubject != nil {
				t.Fatal("preview returned a transient recipient id")
			}
		})
	}
}
