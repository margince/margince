// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The initials task rewrites "Dv" to "dv" only where the minting mail's stored
// original shows the header gave the address no name.

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// captureFromStoredMail stores an inbound Gmail original whose From line is
// from. It then runs the capture ensure for its sender with the header's
// display name, as the Gmail sink does.
func captureFromStoredMail(t *testing.T, e *integration.Env, from, display, email string) ids.ContactID {
	t.Helper()
	owner := integration.OwnerConn(t)
	sourceID := "initials-" + ids.NewV7().String()
	activity := integration.SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, occurred_at, direction, source_system, source_id, source, captured_by, counterparty_email)
		VALUES ($1, 'email', 'hello', '2026-08-01T09:00:00Z', 'inbound', 'gmail', $2, 'gmail:'||$2, 'connector:gmail', $3)`, sourceID, email)
	raw := crlfMessage("From: "+from, "To: owner@initials-owner.test", "Subject: hello", "Message-ID: <"+sourceID+"@initials.test>", "", "hello")
	if _, err := owner.Exec(context.Background(), `
		INSERT INTO raw_capture (source_system, source_id, payload) VALUES ('gmail', $1, to_jsonb($2::text))`, sourceID, raw); err != nil {
		t.Fatalf("seeding the stored original: %v", err)
	}
	res, err := e.Contacts.EnsureCounterparty(e.ConnectorOwnerCtx(e.AdminUser, "gmail"), contacts.EnsureCounterpartyInput{
		Email: email, DisplayName: display, Domain: email[strings.IndexByte(email, '@')+1:],
		OwnerID: e.AdminUser, ActivityID: ids.From[ids.ActivityKind](activity),
		Source: "gmail:" + activity.String(), CapturedBy: "connector:gmail",
	})
	if err != nil {
		t.Fatalf("capturing %q <%s>: %v", display, email, err)
	}
	return res.ContactID
}

// legacyInitialsContact is a contact capture minted from a bare dv@ before the
// parser kept the local part, so it shows "Dv". No current writer stores that
// name, so it is planted without an audit row, as the old capture left it.
func legacyInitialsContact(t *testing.T, e *integration.Env, email string) ids.ContactID {
	t.Helper()
	id := captureFromStoredMail(t, e, email, "", email)
	plantInitialsGuess(t, e, id)
	return id
}

func plantInitialsGuess(t *testing.T, e *integration.Env, id ids.ContactID) {
	t.Helper()
	e.WsExec(t, `UPDATE contact SET full_name = 'Dv' WHERE id = $1`, id)
}

func runInitialsTask(t *testing.T, e *integration.Env) {
	t.Helper()
	runMachineAddressTask(datatask.Acting(e.Admin(), initialsNamesDataTask().ID), t, e, initialsNamesDataTask())
}

func TestTheInitialsTaskShowsTheLocalPartOfAnAddressTheHeaderDidNotName(t *testing.T) {
	e := integration.Setup(t)
	id := legacyInitialsContact(t, e, "dv@initials.test")

	runInitialsTask(t, e)

	if got := shownName(t, e, id); got != "dv" {
		t.Errorf("full_name = %q, want the local part in place of the initials guess", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1 AND actor_id = $2
		AND before ->> 'full_name' = 'Dv' AND after ->> 'full_name' = 'dv'`, id, contacts.InitialsRepairActor); got != 1 {
		t.Errorf("rewrite audit rows = %d, want one under the task's actor", got)
	}
}

func TestANameLearnedAfterTheInitialsTaskStillReplacesItsRewrite(t *testing.T) {
	e := integration.Setup(t)
	id := legacyInitialsContact(t, e, "dv@initials.test")
	runInitialsTask(t, e)
	learnSplitName(t, e, id, "Dana")

	runDisplayNameRepair(t, e)

	if got := shownName(t, e, id); got != "Dana Welter" {
		t.Errorf("full_name = %q, want the learned name: the task's rewrite is still capture's", got)
	}
}

func TestTheInitialsTaskKeepsANameTheHeaderSpelled(t *testing.T) {
	e := integration.Setup(t)
	id := captureFromStoredMail(t, e, "DV <dv@initials.test>", "DV", "dv@initials.test")
	if got := shownName(t, e, id); got != "Dv" {
		t.Fatalf("capture stored %q for the header DV, want the folded Dv this test is about", got)
	}

	runInitialsTask(t, e)

	if got := shownName(t, e, id); got != "Dv" {
		t.Errorf("full_name = %q, want Dv kept: the header named the contact", got)
	}
}

func TestTheInitialsTaskKeepsANameWhoseOriginalIsGone(t *testing.T) {
	e := integration.Setup(t)
	id := legacyInitialsContact(t, e, "dv@initials.test")
	owner := integration.OwnerConn(t)
	if _, err := owner.Exec(context.Background(), `
		DELETE FROM raw_capture rc USING activity a, activity_link l
		 WHERE l.contact_id = $1 AND a.id = l.activity_id AND rc.source_id = a.source_id`, id); err != nil {
		t.Fatal(err)
	}

	runInitialsTask(t, e)

	if got := shownName(t, e, id); got != "Dv" {
		t.Errorf("full_name = %q, want Dv kept: nothing proves where the name came from", got)
	}
}

func TestTheInitialsTaskKeepsANameWhenTwoMailsCouldHaveMintedIt(t *testing.T) {
	e := integration.Setup(t)
	id := legacyInitialsContact(t, e, "dv@initials.test")
	other := seedReplayableMail(t, e, "initials-twin-"+ids.NewV7().String(), crlfMessage("From: dv@initials.test", "", "x"))
	owner := integration.OwnerConn(t)
	if _, err := owner.Exec(context.Background(), `
		INSERT INTO activity_link (activity_id, entity_type, contact_id, created_at)
		SELECT $1, 'contact', p.id, p.created_at FROM contact p WHERE p.id = $2`, other, id); err != nil {
		t.Fatal(err)
	}

	runInitialsTask(t, e)

	if got := shownName(t, e, id); got != "Dv" {
		t.Errorf("full_name = %q, want Dv kept: the minting mail is not one exact message", got)
	}
}

func TestTheInitialsTaskKeepsAnInitialsNameAHumanRenamed(t *testing.T) {
	e := integration.Setup(t)
	id := captureFromStoredMail(t, e, "dv@initials.test", "", "dv@initials.test")
	typed := "Dvx"
	if _, err := e.Contacts.UpdateContact(e.Admin(), id, contacts.UpdateContactInput{FullName: &typed}); err != nil {
		t.Fatalf("a human renaming the contact: %v", err)
	}
	plantInitialsGuess(t, e, id)

	runInitialsTask(t, e)

	if got := shownName(t, e, id); got != "Dv" {
		t.Errorf("full_name = %q, want Dv left alone: a human touched the name", got)
	}
}

func TestTheInitialsTaskKeepsAnImportedInitialsName(t *testing.T) {
	e := integration.Setup(t)
	id := legacyInitialsContact(t, e, "dv@initials.test")
	e.WsExec(t, `UPDATE contact SET source_system = 'crm_import' WHERE id = $1`, id)

	runInitialsTask(t, e)

	if got := shownName(t, e, id); got != "Dv" {
		t.Errorf("full_name = %q, want Dv left alone: an import named it", got)
	}
}

// Five contacts are eligible and none needs a rewrite. Each batch inspects at
// most its limit, and the next one starts after the last contact inspected.
func TestTheInitialsTaskInspectsAtMostItsLimitAndResumesAfterIt(t *testing.T) {
	e := integration.Setup(t)
	var eligible []ids.UUID
	for n := range 5 {
		email := fmt.Sprintf("dv@initials-%d.test", n)
		id := captureFromStoredMail(t, e, "DV <"+email+">", "DV", email)
		eligible = append(eligible, id.UUID)
	}
	ctx := datatask.Acting(e.Admin(), initialsNamesDataTask().ID)
	step := initialsNamesDataTask().Steps[0]
	in := datatask.BatchIn{Limit: 2, RunID: ids.NewV7()}
	var inspected []ids.UUID
	for range 50 {
		var page contacts.InitialsNamePage
		var out datatask.BatchOut
		err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			after, err := datatask.UUIDCursor(in.Cursor)
			if err != nil {
				return err
			}
			if page, err = contacts.InitialsNamePageTx(ctx, tx, after, in.Limit); err != nil {
				return err
			}
			out, err = step.Batch(ctx, tx, in)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		if out.Scanned > in.Limit || len(page.Scanned) > in.Limit {
			t.Fatalf("a batch inspected %d contacts, want at most its limit %d", out.Scanned, in.Limit)
		}
		if out.Changed != 0 {
			t.Errorf("a batch changed %d names, want none: every header named its contact", out.Changed)
		}
		for _, c := range page.Candidates {
			if !slices.Contains(page.Scanned, c.ContactID.UUID) {
				t.Errorf("candidate %s is outside the page that inspected %v", c.ContactID, page.Scanned)
			}
		}
		for _, id := range page.Scanned {
			if slices.Contains(inspected, id) {
				t.Fatalf("contact %s was inspected twice: the walk did not resume after its cursor", id)
			}
		}
		inspected = append(inspected, page.Scanned...)
		if out.Done {
			break
		}
		in.Cursor = out.Cursor
	}
	for _, id := range eligible {
		if !slices.Contains(inspected, id) {
			t.Errorf("eligible contact %s was never inspected", id)
		}
	}
}
