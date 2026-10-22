// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"maps"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestCompletingOneSourceRequestPreservesOtherDocumentsAndRecipients(t *testing.T) {
	for _, test := range []struct{ name, subject, first, second, otherParty string }{
		{"different documents", "Signature requested", "Please sign document alpha.", "Please sign document beta.", requestCounterparty},
		{"missing document identity", "Signature requested", "Please sign the form.", "Please sign the form.", requestCounterparty},
		{"separate recipients", "Send the recording", "Please send me the workshop recording.", "Please send me the workshop recording.", "second@customer.test"},
	} {
		t.Run(test.name, func(t *testing.T) {
			e := setupLoad(t)
			store := storeKnowing(e)
			first := seedEmailRequest(t, e, test.subject, "commitment", OwedVerdictAsksUs)
			second := seedEmailRequestWithCounterparty(t, e, test.subject, "commitment", OwedVerdictAsksUs, test.otherParty)
			actor, ok := principal.Actor(e.as())
			if !ok {
				t.Fatal("missing request owner")
			}
			actor.Permissions.Objects = maps.Clone(actor.Permissions.Objects)
			actor.Permissions.Objects["activity"] = principal.ObjectGrant{Read: true, Update: true}
			ctx := principal.WithActor(e.as(), actor)
			for source, body := range map[ids.UUID]string{first: test.first, second: test.second} {
				if _, err := store.UpdateActivity(ctx, ids.From[ids.ActivityKind](source), UpdateActivityInput{Body: &body}); err != nil {
					t.Fatal(err)
				}
			}
			mintReminder(t, e, requestInstant.Add(time.Hour))
			task, _, _, _ := taskFor(t, e, first)
			other, _, _, _ := taskFor(t, e, second)
			if task == other {
				t.Fatal("distinct source requests share a task")
			}
			if _, err := store.UpdateActivity(ctx, ids.From[ids.ActivityKind](task), UpdateActivityInput{IsDone: new(true)}); err != nil {
				t.Fatal(err)
			}
			mintReminder(t, e, requestInstant.Add(2*time.Hour))
			again, _, done, _ := taskFor(t, e, first)
			if again != task || !done {
				t.Fatal("completed request created fresh work")
			}
			if _, _, done, _ := taskFor(t, e, second); done {
				t.Fatal("completion crossed a document or recipient boundary")
			}
			summaries, err := store.EmailSummariesByID(ctx, []ids.UUID{first, second})
			if err != nil {
				t.Fatal(err)
			}
			if summaries[first].Move == crmcontracts.EmailSummaryMoveNeedsReply || summaries[second].Move != crmcontracts.EmailSummaryMoveNeedsReply {
				t.Fatal("source summaries disagree with their separate task states")
			}
		})
	}
}
