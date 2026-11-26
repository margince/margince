// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration_test

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAMailboxThenAnotherMembersImportKeepsOneReading(t *testing.T) {
	e := integration.Setup(t)
	const messageID = "mailbox-before-admin@counterparty.example"
	connectAs(t, e, e.Rep1, "gmail", ownerAddr)
	captureWithTakeOver(t, e, e.Rep1, theSameMessage(messageID, "pat@counterparty.example", ownerAddr))
	before := columnOfMessage[string](t, e, "body", messageID)
	importThrough(t, e, e.AdminUser, importedFromAnotherCRM(messageID, "Imported subject", "stripped imported copy"))
	if got := activitiesDescribing(t, e, messageID); got != 1 {
		t.Fatalf("one message landed as %d activities", got)
	}
	if got := columnOfMessage[string](t, e, "body", messageID); got != before {
		t.Fatal("the importer replaced the mailbox reading")
	}
}

func TestProvenImportAddsLinksWithoutRowUpdateOrWideningHeldContent(t *testing.T) {
	e := integration.Setup(t)
	const message = "held-mailbox-before-import@counterparty.example"
	connectAs(t, e, e.Rep1, "gmail", ownerAddr)
	captureWithTakeOver(t, e, e.Rep1, theSameMessage(message, "pat@counterparty.example", ownerAddr))
	e.WsExec(t, `UPDATE activity SET audience='participants' WHERE source_id=$1`, message)
	contact := e.SeedContact(t, "Imported contact", nil)
	in, err := activities.LogActivityInputFrom(importedFromAnotherCRM(message, "Imported", "Stripped"))
	if err != nil {
		t.Fatal(err)
	}
	in.Links = []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}}
	ctx := seatCtx(t, e, e.AdminUser)
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("missing importer")
	}
	grant := actor.Permissions.Objects["activity"]
	grant.Update = false
	actor.Permissions.Objects["activity"] = grant
	ctx = principal.WithActor(ctx, actor)
	if _, _, err := activities.NewStore(e.DB()).WithIdentityProver(capture.ProvedUnambiguouslyTx).LogActivity(ctx, in); err != nil {
		t.Fatal(err)
	}
	if got := activitiesDescribing(t, e, message); got != 1 {
		t.Fatalf("mail copies=%d", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity_link l JOIN activity a ON a.id=l.activity_id WHERE a.source_id=$1 AND l.contact_id=$2`, message, contact); got != 1 {
		t.Fatal("imported link was lost")
	}
	if got := columnOfMessage[string](t, e, "audience", message); got != "participants" {
		t.Fatalf("held audience widened to %s", got)
	}
}
