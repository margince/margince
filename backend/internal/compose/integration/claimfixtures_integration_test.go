// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration_test

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

var laneClock = time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)

func seedPromise(
	t *testing.T, e *integration.Env, contactID ids.UUID, body string, due *time.Time,
) ids.UUID {
	t.Helper()
	subject := "Rückfragen zum Angebot"
	occurred := laneClock.AddDate(0, 0, -7)
	message, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: &subject, OccurredAt: &occurred, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contactID}},
	})
	if err != nil {
		t.Fatalf("logging the message the promise was made in: %v", err)
	}
	claim := e.RecordClaim(e.Admin(), t, contacts.ClaimInput{
		ContactID: ids.From[ids.ContactKind](contactID), Kind: "commitment_ours",
		Body: body, ActivityID: ids.UUID(message.Id), Quote: body, DueAt: due,
		Source: "manual",
	})
	return ids.UUID(claim.Id)
}
