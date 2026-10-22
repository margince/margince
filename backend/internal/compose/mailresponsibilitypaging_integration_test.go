// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestPersonalWaitingFiltersRecipientsBeforeItsScanCap(t *testing.T) {
	e := integration.Setup(t)
	at := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
	var address string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT email FROM app_user WHERE id=$1`, e.Rep1).Scan(&address)
	}); err != nil {
		t.Fatal(err)
	}
	contact := e.SeedContact(t, "Buyer", &e.AdminUser)
	classifier := principal.WithActor(e.Admin(), principal.Principal{Type: principal.PrincipalSystem, ID: "system:owed_verdict", Permissions: principal.Permissions{RowScope: principal.RowScopeAll}})
	var personal ids.UUID
	for i := 0; i <= activities.WaitingScanCap; i++ {
		recipient := "colleague@work.test"
		if i == 0 {
			recipient = address
		}
		occurred := at.Add(time.Duration(i) * time.Second)
		subject := fmt.Sprintf("Specification %d", i)
		body, direction := "Please send the specification.", "inbound"
		source, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "email", Subject: &subject, Body: &body, Direction: &direction, OccurredAt: &occurred, Source: "manual", ThreadKey: fmt.Sprintf("bounded-recipient-%d", i), EmailFrom: "buyer@customer.test", EmailTo: []string{recipient}, Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}}})
		if err != nil {
			t.Fatal(err)
		}
		id := ids.UUID(source.Id)
		if i == 0 {
			personal = id
		}
		if _, err := e.Activities.SetOwedVerdict(classifier, id, activities.OwedVerdictAsksUs, "fixture", occurred); err != nil {
			t.Fatal(err)
		}
	}
	feed := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return at.Add(time.Hour) })
	page, err := feed.Worklist(e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms), "mine", "all", ids.Nil, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Queue) != 1 || page.Queue[0].Id != personal.String() || page.Summary.Total != 1 {
		t.Fatalf("personal page lost behind colleagues: %+v", page)
	}
	if page.Focus == nil || page.Focus.Total != 1 || len(page.Focus.Items) != 1 || page.Focus.Items[0].Id != personal.String() {
		t.Fatalf("personal Focus differs from queue: %+v", page.Focus)
	}
}
