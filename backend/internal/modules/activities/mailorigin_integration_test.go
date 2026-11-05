// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The origin reader asks the content gate itself, so it names no mail the
// caller may not read even when a list hands it one.
func TestAMailOriginNamesOnlyAMailTheCallerMayRead(t *testing.T) {
	e := setupPromises(t)
	open, held, meeting := ids.NewV7(), ids.NewV7(), ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'inbound', 'Signed order', now(), 'seed', 'system'),
		       ($2, 'email', 'inbound', 'Salary talk', now(), 'seed', $3)`,
		open, held, "connector:imap:"+e.other.String())
	e.exec(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'meeting', 'Kick-off', now(), 'seed', 'system')`, meeting)
	e.exec(t, `UPDATE activity SET audience = 'participants' WHERE id = $1`, held)
	e.exec(t, `INSERT INTO activity_participant (id, activity_id, role, address, display_name)
		VALUES ($1, $2, 'from', 'buyer@origin.test', 'Bea Buyer'), ($3, $4, 'from', 'hr@origin.test', 'HR')`,
		ids.NewV7(), open, ids.NewV7(), held)

	ctx := e.reading("activity", "contact", "company", "deal")
	origins := e.mailOrigins(ctx, t, open, held, meeting)
	if got := origins[open]; got == nil || got.Subject == nil || *got.Subject != "Signed order" ||
		got.Sender == nil || got.Sender.Address != "buyer@origin.test" {
		t.Errorf("the readable mail's origin is %+v, want subject Signed order from buyer@origin.test", got)
	}
	if got, named := origins[held]; named {
		t.Errorf("a mail outside the caller's audience is named: %+v", got)
	}
	if got, named := origins[meeting]; named {
		t.Errorf("a meeting is named as a file's mail: %+v", got)
	}
}

// A mail logged by its day hands its precision on. Its file is then dated by
// that day, not by the stored noon read on a clock.
func TestAMailOriginCarriesTheMailsPrecision(t *testing.T) {
	e := setupPromises(t)
	day, moment := ids.NewV7(), ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, occurred_precision, source, captured_by)
		VALUES ($1, 'email', 'inbound', 'Posted order', '2026-08-21T12:00:00Z', 'day', 'seed', 'system'),
		       ($2, 'email', 'inbound', 'Signed order', now(), 'instant', 'seed', 'system')`, day, moment)

	origins := e.mailOrigins(e.reading("activity"), t, day, moment)
	for mail, want := range map[ids.UUID]crmcontracts.OccurredPrecision{
		day:    crmcontracts.OccurredPrecisionOccurredPrecisionDay,
		moment: crmcontracts.OccurredPrecisionOccurredPrecisionInstant,
	} {
		if got := origins[mail]; got == nil || got.OccurredPrecision != want {
			t.Errorf("mail %s: origin is %+v, want precision %s", mail, got, want)
		}
	}
}

// A reader without contact read sees the sender the header states, and no
// contact behind it. A file's origin and the email viewer agree.
func TestASenderNamesNoContactToAReaderWithoutContactRead(t *testing.T) {
	e := setupPromises(t)
	mail, contact := ids.NewV7(), ids.NewV7()
	e.exec(t, `INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Record Name', 'seed', 'system')`, contact)
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'inbound', 'Signed order', now(), 'seed', 'system')`, mail)
	e.exec(t, `INSERT INTO activity_participant (id, activity_id, role, contact_id, address, display_name)
		VALUES ($1, $2, 'from', $3, 'buyer@origin.test', 'Header Name')`, ids.NewV7(), mail, contact)

	for name, tc := range map[string]struct {
		ctx      context.Context
		wantName string
		named    bool
	}{
		"with contact read":    {e.reading("activity", "contact", "company"), "Record Name", true},
		"without contact read": {e.reading("activity", "company"), "Header Name", false},
	} {
		sender := e.mailOrigins(tc.ctx, t, mail)[mail].Sender
		email, err := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws))).
			GetEmailPresentation(tc.ctx, ids.From[ids.ActivityKind](mail), nil)
		if err != nil {
			t.Fatalf("%s: reading the email: %v", name, err)
		}
		if len(email.From) != 1 {
			t.Fatalf("%s: the email names %d senders, want 1", name, len(email.From))
		}
		for surface, party := range map[string]*crmcontracts.EmailParty{"origin": sender, "email viewer": &email.From[0]} {
			if party == nil || party.Address != "buyer@origin.test" || party.DisplayName == nil || *party.DisplayName != tc.wantName {
				t.Errorf("%s, %s: sender is %+v, want %s <buyer@origin.test>", name, surface, party, tc.wantName)
				continue
			}
			if (party.ContactId != nil) != tc.named {
				t.Errorf("%s, %s: names contact %v, want named = %v", name, surface, party.ContactId, tc.named)
			}
		}
	}
}

// reading binds the rep at all row scope, with read on the named objects only.
func (e *promiseEnv) reading(objects ...string) context.Context {
	grants := map[string]principal.ObjectGrant{}
	for _, object := range objects {
		grants[object] = principal.ObjectGrant{Read: true}
	}
	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.ws), ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.rep.String(), UserID: e.rep,
		Permissions: principal.Permissions{Objects: grants, RowScope: principal.RowScopeAll},
	})
}

// mailOrigins reads the origins of mails as the caller in ctx.
func (e *promiseEnv) mailOrigins(ctx context.Context, t *testing.T, mails ...ids.UUID) map[ids.UUID]*crmcontracts.AttachmentOrigin {
	t.Helper()
	var origins map[ids.UUID]*crmcontracts.AttachmentOrigin
	err := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws))).tx(ctx, func(tx pgx.Tx) (err error) {
		origins, err = mailOrigins(ctx, tx, mails)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return origins
}
