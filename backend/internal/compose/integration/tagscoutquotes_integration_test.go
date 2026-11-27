// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration_test

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestTheOfflineTagScoutIgnoresOrdinaryReplyQuotations(t *testing.T) {
	e := integration.Setup(t)
	tags := collections.NewStore(e.DB())
	description := "product x pricing"
	tag, err := tags.CreateTag(e.Admin(), "Product X", nil, &description)
	if err != nil {
		t.Fatal(err)
	}
	yes := true
	if _, err := tags.UpdateTag(e.Admin(), tag.ID, collections.TagUpdate{Suggestible: &yes}, 0); err != nil {
		t.Fatal(err)
	}
	contact := e.SeedContact(t, "Reply writer", &e.Rep1)
	now := clocktest.Now(t)
	at := now.Add(-time.Hour)
	subject := "Re: Product X"
	body := "Thank you, we are keeping our current system.\n\nOn Tuesday, Sales wrote:\n> Product X pricing is attached."
	inbound := "inbound"
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "email", Subject: &subject, Body: &body, Direction: &inbound, OccurredAt: &at, Source: "manual", Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}}}); err != nil {
		t.Fatal(err)
	}
	ctx := principal.SystemActing(e.Admin(), "system:tag-scout")
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		pass, err := compose.RunTagScout(ctx, tx, now)
		if err == nil && pass.Raised != 0 {
			t.Errorf("quoted pricing raised %d suggestions", pass.Raised)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
