// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func childBatchFixture(t *testing.T) (*privacyEnv, context.Context, ids.ContactID, ids.ContactID) {
	t.Helper()
	e := setupCapturePrivacy(t)
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	e.store.SetClock(func() time.Time { return now })
	ctx := e.as(e.owner, principal.RowScopeOwn)
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("fixture has no actor")
	}
	actor.SeatType = principal.SeatFull
	ctx = principal.WithActor(ctx, actor)
	made, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: "Child batch owner", Source: "manual",
		Emails: []ContactEmailInput{
			{Email: "secondary@child-batch.test", EmailType: "work", Position: 0},
			{Email: "primary@child-batch.test", EmailType: "personal", IsPrimary: true, Position: 4},
		},
		Phones: []ContactPhoneInput{
			{Phone: "+493012345678", PhoneType: "work", Position: 3},
			{Phone: "+491701234567", PhoneType: "mobile", Position: 1},
		},
		Social: map[string]any{
			SocialLinkedIn: "https://www.linkedin.com/in/child-batch-owner",
			"github":       "child-batch-owner",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.ContactKind](ids.UUID(made.Id))
	empty, err := e.store.CreateContact(ctx, CreateContactInput{FullName: "No child rows", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	ci := connector.ChannelIdentity{Provider: telegramProvider, ChannelUserID: "child-batch-user"}
	err = e.store.tx(ctx, func(tx pgx.Tx) error {
		bound, err := ResolveOrCreateChannelIdentity(ctx, tx, id, ci)
		if err != nil {
			return err
		}
		if bound != id {
			t.Fatal("channel binding selected a different contact")
		}
		return e.store.SetChannelIdentityBlocked(ctx, tx, ci, true, "child-batch-bot", 1)
	})
	if err != nil {
		t.Fatal(err)
	}
	return e, ctx, id, ids.From[ids.ContactKind](ids.UUID(empty.Id))
}

func TestContactChildBatchUsesOneExchangeForFourReads(t *testing.T) {
	e, ctx, id, empty := childBatchFixture(t)
	err := e.store.tx(ctx, func(tx pgx.Tx) error {
		var one int
		if err := tx.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil {
			return err
		}
		if one != 1 {
			t.Fatal("transaction did not answer")
		}
		rows := []crmcontracts.Contact{{Id: openapi_types.UUID(id.UUID)}, {Id: openapi_types.UUID(empty.UUID)}}
		idx := map[openapi_types.UUID]*crmcontracts.Contact{rows[0].Id: &rows[0], rows[1].Id: &rows[1]}
		metered, meter := database.WithStatementMeter(ctx)
		if err := attachContactChannels(metered, tx, idx, []ids.UUID{id.UUID, empty.UUID}); err != nil {
			return err
		}
		cost := meter.Cost()
		if cost.Statements != 4 || cost.Exchanges != 1 || cost.Failed != 0 {
			t.Fatalf("child cost = %+v, want four statements in one successful exchange", cost)
		}
		assertChildBatchValues(t, rows[0])
		assertChildBatchEmpty(t, rows[1])
		if err := tx.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil {
			return err
		}
		if one != 1 {
			t.Error("batch left the connection unreadable")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := attachContactChannels(ctx, nil, nil, nil); err != nil {
		t.Fatalf("empty child page: %v", err)
	}
}

func TestContactChildBatchReachesStoreReadsAndReplaceSetEchoes(t *testing.T) {
	e, ctx, id, empty := childBatchFixture(t)
	got, err := e.store.GetContact(ctx, id, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	assertChildBatchValues(t, got)
	rows, _, err := e.store.ListContacts(ctx, ListContactsInput{})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[ids.UUID]bool{}
	for _, row := range rows {
		switch ids.UUID(row.Id) {
		case id.UUID:
			seen[id.UUID] = true
			assertChildBatchValues(t, row)
		case empty.UUID:
			seen[empty.UUID] = true
			assertChildBatchEmpty(t, row)
		}
	}
	if len(seen) != 2 {
		t.Fatal("list did not exercise both populated and empty contacts")
	}
	echo, err := e.store.UpdateContact(ctx, id, UpdateContactInput{
		Emails: []ContactEmailInput{{Email: "replacement@child-batch.test", EmailType: "work", IsPrimary: true}},
		Phones: []ContactPhoneInput{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if echo.Emails == nil || len(*echo.Emails) != 1 || string((*echo.Emails)[0].Email) != "replacement@child-batch.test" ||
		echo.PrimaryEmail == nil || string(*echo.PrimaryEmail) != "replacement@child-batch.test" || echo.Phones != nil {
		t.Fatalf("replace-set echo leaked retired children: %+v", echo)
	}
	if !reflect.DeepEqual(echo.Social, got.Social) || !reflect.DeepEqual(echo.Reachability, got.Reachability) {
		t.Fatal("replacing addresses changed unrelated social or reachability facts")
	}
}

func TestContactChildBatchStaysBehindVisibilityAndReadGrant(t *testing.T) {
	e, ctx, id, _ := childBatchFixture(t)
	ownerOnly := "owner"
	if _, err := e.store.UpdateContact(ctx, id, UpdateContactInput{Visibility: &ownerOnly}); err != nil {
		t.Fatal(err)
	}
	other := e.as(e.teammate, principal.RowScopeAll)
	if _, err := e.store.GetContact(other, id, storekit.LiveOnly); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("capture-private read = %v, want not found", err)
	}
	rows, _, err := e.store.ListContacts(other, ListContactsInput{})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if ids.UUID(row.Id) == id.UUID {
			t.Fatal("list disclosed the private contact and its children")
		}
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("fixture has no actor")
	}
	actor.Permissions.Objects = map[string]principal.ObjectGrant{}
	denied, meter := database.WithStatementMeter(principal.WithActor(ctx, actor))
	if _, err := e.store.GetContact(denied, id, storekit.LiveOnly); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("object-denied read = %v, want permission denied", err)
	}
	if meter.Cost().Statements != 0 {
		t.Fatal("object denial reached child queries")
	}
}

func assertChildBatchValues(t *testing.T, row crmcontracts.Contact) {
	t.Helper()
	if row.Emails == nil || len(*row.Emails) != 2 || row.PrimaryEmail == nil {
		t.Fatal("missing email rows or primary email")
	}
	if string((*row.Emails)[0].Email) != "primary@child-batch.test" ||
		string((*row.Emails)[1].Email) != "secondary@child-batch.test" ||
		string(*row.PrimaryEmail) != "primary@child-batch.test" {
		t.Fatal("email ordering no longer gives primary priority over position")
	}
	for _, email := range *row.Emails {
		if ids.UUID(email.Id) == ids.Nil || email.Source != "manual" || (email.CapturedBy == nil || *email.CapturedBy == "") {
			t.Fatal("email identity or provenance lost")
		}
	}
	if (*row.Emails)[0].EmailType != "personal" || !(*row.Emails)[0].IsPrimary || (*row.Emails)[0].Position != 4 {
		t.Fatal("primary email metadata changed")
	}
	if row.Phones == nil || len(*row.Phones) != 2 || (*row.Phones)[0].Position != 1 || (*row.Phones)[1].Position != 3 {
		t.Fatal("phone rows did not retain position order")
	}
	for _, phone := range *row.Phones {
		if ids.UUID(phone.Id) == ids.Nil || phone.Source != "manual" || (phone.CapturedBy == nil || *phone.CapturedBy == "") || phone.Phone == "" {
			t.Fatal("phone value, identity or provenance lost")
		}
	}
	if row.Social == nil || len(*row.Social) != 2 || (*row.Social)["github"] != "child-batch-owner" ||
		row.LinkedinUrl == nil || *row.LinkedinUrl != "https://www.linkedin.com/in/child-batch-owner" {
		t.Fatal("social map or linkedin alias lost")
	}
	if row.Reachability == nil || len(*row.Reachability) != 1 || (*row.Reachability)[0].Provider != telegramProvider ||
		(*row.Reachability)[0].Reachable || (*row.Reachability)[0].Since.IsZero() {
		t.Fatal("blocked channel must remain visible with reachable=false and a timestamp")
	}
}

func assertChildBatchEmpty(t *testing.T, row crmcontracts.Contact) {
	t.Helper()
	if row.Emails != nil || row.Phones != nil || row.Social != nil || row.LinkedinUrl != nil ||
		row.PrimaryEmail != nil || row.Reachability != nil {
		t.Fatal("empty children changed from absent pointers to populated fields")
	}
}
