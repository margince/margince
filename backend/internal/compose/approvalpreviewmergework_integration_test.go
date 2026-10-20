// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"maps"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestMergePreviewCarriesClaimsAndProspectHandoffs(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	source, target := mergeContentPair(t, e, "contact")
	quote := "We will send the purchase order."
	activity, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{Kind: "note", Subject: new("Purchase order"), Body: &quote, Source: "manual", Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: source}}})
	if err != nil {
		t.Fatal(err)
	}
	claim, err := e.Contacts.RecordConversationClaim(ctx, contacts.ClaimInput{ContactID: ids.From[ids.ContactKind](source), Kind: "commitment_theirs", Body: "Send purchase order", ActivityID: ids.UUID(activity.Id), Quote: quote, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	handoff, err := e.Contacts.SubmitHandoff(ctx, contacts.NewSDRHandoff{ContactID: &source, Note: "Review the buying process"})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := e.Contacts.PreviewMergeSurvivorship(ctx, "contact", source, target, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Claims) != 1 || preview.Claims[0].Id != claim.Id || preview.Claims[0].SourceQuote != quote {
		t.Fatalf("claims=%+v", preview.Claims)
	}
	if len(preview.Handoffs) != 1 || preview.Handoffs[0].ID != handoff || preview.Handoffs[0].Note != "Review the buying process" {
		t.Fatalf("handoffs=%+v", preview.Handoffs)
	}
	denied := integration.AdminPerms
	denied.Objects = maps.Clone(denied.Objects)
	delete(denied.Objects, "lead")
	hidden, err := e.Contacts.PreviewMergeSurvivorship(e.As(e.AdminUser, nil, denied), "contact", source, target, nil, nil)
	if !errors.Is(err, apperrors.ErrPermissionDenied) || hidden.Handoffs != nil {
		t.Fatalf("hidden scope=%+v error=%v", hidden, err)
	}
	if _, err = e.Contacts.MergeContact(ctx, ids.From[ids.ContactKind](source), ids.From[ids.ContactKind](target), &preview.SourceVersion); err != nil {
		t.Fatal(err)
	}
	for _, carried := range []struct {
		query string
		id    ids.UUID
	}{
		{`SELECT contact_id::text FROM conversation_claim WHERE id=@id`, ids.UUID(claim.Id)},
		{`SELECT contact_id::text FROM sdr_handoff WHERE id=@id`, handoff},
	} {
		if actual := e.WsScalar(t, carried.query, pgx.StrictNamedArgs{"id": carried.id}); actual != target.String() {
			t.Fatalf("carried target=%s", actual)
		}
	}
}
