// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestPersonalWaitingRequiresTheAddressedSeatNotTheRecordOwner(t *testing.T) {
	e := integration.Setup(t)
	at := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
	addresses := map[ids.UUID]string{}
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		for _, seat := range []ids.UUID{e.AdminUser, e.Rep1} {
			var address string
			if err := tx.QueryRow(context.Background(), `SELECT email FROM app_user WHERE id=$1`, seat).Scan(&address); err != nil {
				return err
			}
			addresses[seat] = address
			if _, err := tx.Exec(context.Background(), `INSERT INTO capture_connection (user_id, provider, mail_posture, account_label) VALUES ($1,'gmail','shared',$2)`, seat, address); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	aliasReader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	if _, err := capture.NewOwnerIdentityStore(e.DB()).Add(aliasReader, "address", "sales-alias@ourco.test"); err != nil {
		t.Fatal(err)
	}
	addresses[e.Rep1] = "sales-alias@ourco.test"
	contact := e.SeedContact(t, "Shared buyer", &e.AdminUser)
	subject, body, direction := "Please send the specification", "Please send the specification for review.", "inbound"
	source, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "email", Subject: &subject, Body: &body, Direction: &direction, OccurredAt: &at, Source: "manual", ThreadKey: "recipient-duty", EmailFrom: "buyer@customer.test", EmailTo: []string{addresses[e.Rep1]}, EmailCc: []string{addresses[e.AdminUser]}, Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}}})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.UUID(source.Id)
	classifier := principal.WithActor(e.Admin(), principal.Principal{Type: principal.PrincipalSystem, ID: "system:owed_verdict", Permissions: principal.Permissions{RowScope: principal.RowScopeAll}})
	if _, err := e.Activities.SetOwedVerdict(classifier, id, activities.OwedVerdictAsksUs, "fixture", at); err != nil {
		t.Fatal(err)
	}
	other, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "email", Subject: &subject, Body: &body, Direction: &direction, OccurredAt: &at, Source: "manual", ThreadKey: "recipient-duty-without-copy", EmailFrom: "buyer@customer.test", EmailTo: []string{addresses[e.Rep1]}, Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}}})
	if err != nil {
		t.Fatal(err)
	}
	otherID := ids.UUID(other.Id)
	if _, err := e.Activities.SetOwedVerdict(classifier, otherID, activities.OwedVerdictAsksUs, "fixture", at); err != nil {
		t.Fatal(err)
	}
	feed := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return at.Add(time.Hour) })
	for _, tc := range []struct {
		name   string
		reader ids.UUID
		scope  string
		owner  ids.UUID
		want   bool
	}{
		{"admin copied owner", e.AdminUser, "mine", ids.Nil, false},
		{"direct recipient", e.Rep1, "mine", ids.Nil, true},
		{"selected recipient", e.AdminUser, "all", e.Rep1, true},
		{"selected copied owner", e.AdminUser, "all", e.AdminUser, false},
		{"team monitoring", e.AdminUser, "all", ids.Nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := e.As(tc.reader, []ids.UUID{e.Team1}, integration.AdminPerms)
			page, err := feed.Worklist(ctx, tc.scope, "all", tc.owner, 100, "")
			if err != nil {
				t.Fatal(err)
			}
			found := 0
			for _, row := range page.Queue {
				if row.Source == "customer_waiting" && (row.Id == id.String() || row.Id == otherID.String()) {
					found++
					if row.Owner == nil || row.Owner.Id == nil || ids.UUID(*row.Owner.Id) != e.Rep1 {
						t.Fatalf("waiting row claimed record owner instead of direct recipient: %+v", row.Owner)
					}
				}
			}
			expected := 0
			if tc.want {
				expected = 2
			}
			if page.Summary.Total != expected {
				t.Fatalf("summary total=%d, want %d", page.Summary.Total, expected)
			}
			if found != expected {
				t.Fatalf("waiting=%v, want %v: %+v", found, tc.want, page.Queue)
			}
			if !tc.want && page.Focus != nil {
				for _, row := range page.Focus.Items {
					if row.Id == id.String() || row.Id == otherID.String() {
						t.Fatal("unowned request entered Focus")
					}
				}
			}
		})
	}
	board, err := feed.TeamBoard(aliasReader)
	if err != nil {
		t.Fatal(err)
	}
	seenRecipient := false
	for _, member := range board.Members {
		if ids.UUID(member.UserId) == e.AdminUser && member.Counts.Waiting != 0 {
			t.Fatal("record owner inherited the recipient's waiting count")
		}
		if ids.UUID(member.UserId) == e.Rep1 {
			seenRecipient = true
			if member.Counts.Waiting != 2 {
				t.Fatalf("recipient board waiting=%d, want2", member.Counts.Waiting)
			}
		}
	}
	if !seenRecipient {
		t.Fatal("recipient missing from roster")
	}
	if err := identity.NewService(e.Pool).DeactivateUser(e.Admin(), adminIdentity(e), identity.DeactivateUserInput{UserID: ids.From[ids.UserKind](e.Rep1)}); err != nil {
		t.Fatal(err)
	}
	waiting := NewOwedClassifier(e.Pool, nil, func() time.Time { return at.Add(time.Hour) }, nil).store
	rows, err := waiting.WaitingRepliesForOwnerBefore(e.Admin(), at.Add(time.Hour), activities.WaitingPosition{}, e.Rep1)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatal("a deactivated seat still owns waiting mail through its alias")
	}
}

func TestCapturedMailboxRoleSurvivesSecondaryToWithoutInventingCcOrBccDuty(t *testing.T) {
	e := integration.Setup(t)
	at := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
	var address string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(context.Background(), `SELECT email FROM app_user WHERE id=$1`, e.Rep1).Scan(&address); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `INSERT INTO capture_connection (user_id, provider, mail_posture, account_label) VALUES ($1,'gmail','shared',$2)`, e.Rep1, address)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	contact := e.SeedContact(t, "Shared buyer", &e.AdminUser)
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	if _, err := capture.NewOwnerIdentityStore(e.DB()).Add(reader, "address", "reply-alias@ourco.test"); err != nil {
		t.Fatal(err)
	}
	address = "reply-alias@ourco.test"
	connector := principal.WithActor(reader, principal.Principal{Type: principal.PrincipalConnector, ID: "connector:gmail", UserID: e.Rep1, OnBehalfOf: e.Rep1, Permissions: integration.AdminPerms})
	classifier := principal.WithActor(e.Admin(), principal.Principal{Type: principal.PrincipalSystem, ID: "system:owed_verdict", Permissions: principal.Permissions{RowScope: principal.RowScopeAll}})
	feed := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return at.Add(time.Hour) })
	for i, tc := range []struct {
		name, to, cc string
		want         bool
	}{
		{"secondary To", "first@customer.test, " + address, "", true},
		{"legacy secondary To", "first@customer.test, " + address, "", true},
		{"copied only", "first@customer.test", address, false},
		{"edited body", "first@customer.test", "", false},
		{"shared alias", address, "", false},
		{"blind or forwarded", "first@customer.test", "", false},
		{"no named recipient", "undisclosed-recipients:;", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := []byte(strings.Join([]string{"From: buyer@customer.test", "To: " + tc.to, "Cc: " + tc.cc, "Subject: Specification request", fmt.Sprintf("Message-ID: <mail-duty-%d@customer.test>", i), "Date: " + at.Format(time.RFC1123Z), "Content-Type: text/plain; charset=utf-8", "", "Please send the specification."}, "\r\n"))
			parsed, err := mailmap.Parse(raw, address)
			if err != nil {
				t.Fatal(err)
			}
			ref, err := capture.NewSink(e.DB()).Upsert(connector, parsed.ToRecord("gmail", raw))
			if err != nil {
				t.Fatal(err)
			}
			if tc.name == "edited body" {
				if _, err := e.Activities.UpdateActivity(reader, ids.From[ids.ActivityKind](ref.ID), activities.UpdateActivityInput{Body: new("From: buyer@customer.test\nTo: " + address + "\n\nPlease send the specification.")}); err != nil {
					t.Fatal(err)
				}
			}
			if tc.name == "shared alias" {
				if _, err := capture.NewOwnerIdentityStore(e.DB()).Add(e.Admin(), "address", address); err != nil {
					t.Fatal(err)
				}
			}
			if tc.name == "legacy secondary To" {
				restoreLegacyRecipientThroughEngine(classifier, t, e, ref.ID, address, at)
			}
			var directEvidence int
			if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
				return tx.QueryRow(context.Background(), `SELECT count(*) FROM activity_participant WHERE activity_id=$1 AND role='to' AND address=$2`, ref.ID, address).Scan(&directEvidence)
			}); err != nil {
				t.Fatal(err)
			}
			expectedEvidence := 0
			if tc.want || tc.name == "shared alias" {
				expectedEvidence = 1
			}
			if directEvidence != expectedEvidence {
				t.Fatalf("parsed owner To evidence=%d, want %d", directEvidence, expectedEvidence)
			}
			if _, err := e.Activities.RelinkActivity(e.Admin(), ids.From[ids.ActivityKind](ref.ID), activities.RelinkActivityInput{EntityType: "contact", EntityID: contact}); err != nil {
				t.Fatal(err)
			}
			if _, err := e.Activities.SetOwedVerdict(classifier, ref.ID, activities.OwedVerdictAsksUs, "fixture", at); err != nil {
				t.Fatal(err)
			}
			page, err := feed.Worklist(reader, "mine", "all", ids.Nil, 100, "")
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, row := range page.Queue {
				if row.Source == "customer_waiting" && row.Id == ref.ID.String() {
					found = true
				}
			}
			if found != tc.want {
				t.Fatalf("captured request in Mine=%v, want %v", found, tc.want)
			}
			if err := NewOwedClassifier(e.Pool, nil, func() time.Time { return at.Add(time.Hour) }, nil).store.CaptureEmailRequests(classifier, at.Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
			var tasks int
			if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
				return tx.QueryRow(context.Background(), `SELECT count(*) FROM activity WHERE source_system='email_request' AND source_activity_id=$1`, ref.ID).Scan(&tasks)
			}); err != nil {
				t.Fatal(err)
			}
			expected := 0
			if tc.want {
				expected = 1
			}
			if tasks != expected {
				t.Fatalf("request tasks=%d, want %d", tasks, expected)
			}
		})
	}
}
