// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The decay lane's two reads against a real database.
//
// Both claims are about the contact row scope, which is SQL: the candidate read
// scopes at source so the cap is spent on rows the reader may see, and the
// batched derivation admits only readable contacts. A unit test with
// hand-built rows cannot fail on either — nor on a column name — so the
// refusal AND the admission are both asserted here.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// seedLapsedPair seeds an exchange old enough to be past the §4 quiet
// threshold, then folds it through the REAL projection writer.
//
// The edge row is never inserted directly. A hand-written row is a row the
// fold never produced, so a test seeded that way can pass over a projection
// that no longer writes what the read expects — which is the whole failure
// this lane's candidate half would hide.
func seedLapsedPair(t *testing.T, e *Env, colleague, contact ids.UUID, subject string) {
	t.Helper()
	seedLapsedPairAt(t, e, colleague, contact, subject, relstrength.QuietDays+35)
}

// seedLapsedPairAt is the same, with the silence's AGE named — for the cases
// that turn on which contact is oldest rather than only on being past the
// threshold.
//
// TWO messages, one each way. The candidate read admits a RELATIONSHIP rather
// than a single contact, because a lone touch is the cold mail nobody answered
// and there is nothing there to revive — so a one-message fixture seeds exactly
// the pair that read is right to refuse, and every caller above would be
// resting its premise on the refusal.
//
// The reply is dated a day EARLIER than the send, so last_at stays the instant
// daysAgo names and the cases that turn on whose silence is oldest still turn
// on what they say they do.
func seedLapsedPairAt(t *testing.T, e *Env, colleague, contact ids.UUID, subject string, daysAgo int) {
	t.Helper()
	owner := OwnerConn(t)
	ctx := context.Background()
	quietSince := time.Now().UTC().AddDate(0, 0, -daysAgo)

	folded := []ids.UUID{}
	for _, message := range []struct {
		at                         time.Time
		direction                  string
		colleagueRole, contactRole string
	}{
		{quietSince.AddDate(0, 0, -1), "inbound", "to", "from"},
		{quietSince, "outbound", "from", "to"},
	} {
		activity := ids.NewV7()
		if _, err := owner.Exec(ctx, `
			INSERT INTO activity (id, kind, subject, occurred_at, direction, source, captured_by)
			VALUES ($1, 'email', $2, $3, $4, 'manual', 'human:x')`,
			activity, subject, message.at, message.direction); err != nil {
			t.Fatal(err)
		}
		LinkActivity(t, owner, activity, "contact", contact)
		for _, seed := range []struct {
			column string
			id     ids.UUID
			role   string
		}{{"user_id", colleague, message.colleagueRole}, {"contact_id", contact, message.contactRole}} {
			if _, err := owner.Exec(ctx, `
				INSERT INTO activity_participant (activity_id, `+seed.column+`, role) VALUES ($1, $2, $3)`,
				activity, seed.id, seed.role); err != nil {
				t.Fatal(err)
			}
		}
		folded = append(folded, activity)
	}
	wsCtx := principal.WithWorkspaceID(ctx, e.WS)
	if err := database.WithWorkspaceTx(wsCtx, e.Pool, func(tx pgx.Tx) error {
		return search.RecomputeEdgesForActivities(wsCtx, tx, folded)
	}); err != nil {
		t.Fatalf("folding the seeded exchange into the projection: %v", err)
	}
}

// A quiet edge to a contact outside the caller's row scope is not a candidate.
// The refusal alone cannot show the read works — a query that returned nothing
// for everyone would pass it — so the same call must admit the caller's own
// lapsed contact.
func TestQuietCandidatesCarryOnlyContactsInsideRowScope(t *testing.T) {
	e := Setup(t)
	mine := e.SeedContact(t, "Anna Weber", &e.Rep1)
	theirs := e.SeedContact(t, "Their Contact", &e.Rep3)
	e.MakeCapturePrivate(t, "contact", theirs, e.Rep3)
	seedLapsedPair(t, e, e.Rep1, mine, "the thread that lapsed")
	seedLapsedPair(t, e, e.Rep1, theirs, "their private thread")

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, graphPerms)
	now := time.Now().UTC()
	var quiet []search.QuietContact
	err := database.WithWorkspaceTx(rep, e.Pool, func(tx pgx.Tx) error {
		var err error
		quiet, err = search.QuietContacts(rep, tx, search.QuietRead{
			QuietBefore: now.AddDate(0, 0, -relstrength.QuietDays), TouchedAfter: now.AddDate(0, 0, -180), Limit: 10,
		})
		return err
	})
	if err != nil {
		t.Fatalf("reading quiet candidates: %v", err)
	}
	if len(quiet) != 1 {
		t.Fatalf("candidates = %d, want only the readable lapsed contact", len(quiet))
	}
	if quiet[0].Edge.ContactID != mine {
		t.Errorf("candidate is %s, want the caller's own contact %s", quiet[0].Edge.ContactID, mine)
	}
}

// seedColleagueMessage files a recent mail to a colleague, sent by `sender`,
// with `copied` on the thread, and folds it through the real projection writer.
func seedColleagueMessage(t *testing.T, e *Env, colleague, sender, copied ids.UUID) {
	t.Helper()
	owner := OwnerConn(t)
	ctx := context.Background()
	activity := ids.NewV7()
	if _, err := owner.Exec(ctx, `
		INSERT INTO activity (id, kind, subject, occurred_at, direction, source, captured_by)
		VALUES ($1, 'email', 'the thread they are copied on', $2, 'inbound', 'manual', 'human:x')`,
		activity, clocktest.Now(t).UTC().AddDate(0, 0, -2)); err != nil {
		t.Fatal(err)
	}
	LinkActivity(t, owner, activity, "contact", copied)
	LinkActivity(t, owner, activity, "contact", sender)
	for _, p := range []struct {
		column string
		id     ids.UUID
		role   string
	}{{"user_id", colleague, "to"}, {"contact_id", sender, "from"}, {"contact_id", copied, "cc"}} {
		if _, err := owner.Exec(ctx, `
			INSERT INTO activity_participant (activity_id, `+p.column+`, role) VALUES ($1, $2, $3)`,
			activity, p.id, p.role); err != nil {
			t.Fatal(err)
		}
	}
	wsCtx := principal.WithWorkspaceID(ctx, e.WS)
	if err := database.WithWorkspaceTx(wsCtx, e.Pool, func(tx pgx.Tx) error {
		return search.RecomputeEdgesForActivities(wsCtx, tx, []ids.UUID{activity})
	}); err != nil {
		t.Fatalf("folding the colleague message: %v", err)
	}
}

// A contact who is only copied on a colleague's recent mail has not been heard
// from. That holds when the same colleague holds the contact's older exchange.
// The contact who wrote that mail has been heard from, so the read drops them.
func TestQuietCandidatesIgnoreAColleaguesMailTheContactOnlyReceived(t *testing.T) {
	e := Setup(t)
	copied := e.SeedContact(t, "Copied Only", &e.Rep1)
	writer := e.SeedContact(t, "Wrote Recently", &e.Rep1)
	seedLapsedPair(t, e, e.Rep1, copied, "the lapsed thread")
	seedLapsedPair(t, e, e.Rep1, writer, "the other lapsed thread")
	seedColleagueMessage(t, e, e.Rep2, writer, copied)
	sameColleague := e.SeedContact(t, "Copied Same Seat", &e.Rep1)
	seedLapsedPair(t, e, e.Rep1, sameColleague, "the third lapsed thread")
	seedColleagueMessage(t, e, e.Rep1, writer, sameColleague)

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, graphPerms)
	now := clocktest.Now(t).UTC()
	var quiet []search.QuietContact
	err := database.WithWorkspaceTx(rep, e.Pool, func(tx pgx.Tx) error {
		var err error
		quiet, err = search.QuietContacts(rep, tx, search.QuietRead{
			QuietBefore: now.AddDate(0, 0, -relstrength.QuietDays), TouchedAfter: now.AddDate(0, 0, -180), Limit: 10,
		})
		return err
	})
	if err != nil {
		t.Fatalf("reading quiet candidates: %v", err)
	}
	got := map[ids.UUID]bool{}
	for _, candidate := range quiet {
		got[candidate.Edge.ContactID] = true
	}
	if len(got) != 2 || !got[copied] || !got[sameColleague] {
		t.Fatalf("candidates = %+v, want only the two contacts who were copied", quiet)
	}
}

// The batched derivation filters the whole candidate set through the caller's
// row scope in one pass: an unreadable contact is absent from the answer, and
// the readable one arrives derived and named.
func TestBatchedChangesAdmitOnlyReadableContacts(t *testing.T) {
	e := Setup(t)
	mine := e.SeedContact(t, "Anna Weber", &e.Rep1)
	theirs := e.SeedContact(t, "Their Contact", &e.Rep3)
	e.MakeCapturePrivate(t, "contact", theirs, e.Rep3)
	seedLapsedPair(t, e, e.Rep1, mine, "the thread that lapsed")
	seedLapsedPair(t, e, e.Rep1, theirs, "their private thread")

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, graphPerms)
	store := contacts.NewStore(e.DB())
	var changed []contacts.ContactChanges
	err := database.WithWorkspaceTx(rep, e.Pool, func(tx pgx.Tx) error {
		candidates := []ids.ContactID{ids.From[ids.ContactKind](mine), ids.From[ids.ContactKind](theirs)}
		touches, zone, err := contactTouches(rep, tx, candidates...)
		if err != nil {
			return err
		}
		changed, err = store.RelationshipChangesForContacts(rep, tx, candidates, time.Now().UTC(), touches, zone)
		return err
	})
	if err != nil {
		t.Fatalf("deriving the candidate set: %v", err)
	}
	if len(changed) != 1 {
		t.Fatalf("derived contacts = %d, want only the readable one", len(changed))
	}
	if changed[0].ContactID.UUID != mine {
		t.Errorf("derived %s, want the caller's own contact %s", changed[0].ContactID.UUID, mine)
	}
	if changed[0].DisplayName != "Anna Weber" {
		t.Errorf("the contact is named %q, want the contact record's own name", changed[0].DisplayName)
	}
	sawQuiet := false
	for _, change := range changed[0].Changes {
		if change.Kind == relstrength.ChangeWentQuiet {
			sawQuiet = true
		}
	}
	if !sawQuiet {
		t.Errorf("changes = %v, want the lapse the seeding arranged", changed[0].Changes)
	}
}

// A contact this reader set aside is removed BEFORE the candidate cap, so it
// cannot evict a real lapse from the budget.
//
// This is the whole reason the exclusion is a predicate the projection embeds
// rather than a filter over what it returned. The read takes the oldest
// silences up to a bound; a contact dropped afterwards has already spent a
// slot, so a rep who set several aside would lose real lapses off the bottom of
// their lane with nothing to say so. The waiting queue applies its own
// set-aside rule before its cap for the same reason.
//
// The cap is ONE here, which is what makes the two behaviours tell apart: with
// the exclusion before the cut the reader gets their live lapse, and with it
// after they get an empty lane.
func TestADismissedContactDoesNotSpendACandidateSlot(t *testing.T) {
	e := Setup(t)
	setAside := e.SeedContact(t, "Dana Weiss", &e.Rep1)
	standing := e.SeedContact(t, "Ines Sommer", &e.Rep1)
	// The dismissed contact went quiet LONGER ago, so it sorts first and is
	// exactly the row that would fill a cap of one.
	seedLapsedPairAt(t, e, e.Rep1, setAside, "the older thread", 120)
	seedLapsedPairAt(t, e, e.Rep1, standing, "the newer thread", 90)

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, graphPerms)
	store := contacts.NewStore(database.BindTo(e.Pool, ids.From[ids.WorkspaceKind](e.WS)))
	if err := store.DismissRelationshipNudge(rep, ids.From[ids.ContactKind](setAside), 30); err != nil {
		t.Fatalf("dismissing: %v", err)
	}

	now := time.Now().UTC()
	var quiet []search.QuietContact
	err := database.WithWorkspaceTx(rep, e.Pool, func(tx pgx.Tx) error {
		var err error
		quiet, err = search.QuietContacts(rep, tx, search.QuietRead{
			QuietBefore: now.AddDate(0, 0, -relstrength.QuietDays), TouchedAfter: now.AddDate(0, 0, -180), Limit: 1,
			Exclude: func(arg func(any) int) (string, error) {
				return contacts.NotDismissedClause(rep, "e", now, arg)
			},
		})
		return err
	})
	if err != nil {
		t.Fatalf("reading quiet candidates: %v", err)
	}

	if len(quiet) != 1 {
		t.Fatalf("candidates = %d, want the one live lapse the budget has room for", len(quiet))
	}
	if quiet[0].Edge.ContactID != standing {
		t.Errorf("the candidate is %s, want %s — the dismissed contact spent the slot",
			quiet[0].Edge.ContactID, standing)
	}
}

// seedConnectorMailFrom files a recent mail the contact wrote to a colleague,
// as a connector captured it. Its only seat row names the colleague by user_id
// alone. With made set, the connector's create audit says it captured the mail
// on that colleague's behalf, and the row stands. Without it, the row is the
// shape the legacy backfill guessed, and auth.ParticipantStandsSQL refuses it.
func seedConnectorMailFrom(t *testing.T, e *Env, contact, colleague ids.UUID, made bool) {
	t.Helper()
	owner := OwnerConn(t)
	ctx := context.Background()
	activity := ids.NewV7()
	if _, err := owner.Exec(ctx, `
		INSERT INTO activity (id, kind, subject, occurred_at, direction, source, captured_by)
		VALUES ($1, 'email', 'a new message', $2, 'inbound', 'gmail:x', 'connector:gmail')`,
		activity, clocktest.Now(t).UTC().AddDate(0, 0, -2)); err != nil {
		t.Fatal(err)
	}
	LinkActivity(t, owner, activity, "contact", contact)
	for _, p := range []struct {
		column string
		id     ids.UUID
		role   string
	}{{"user_id", colleague, "to"}, {"contact_id", contact, "from"}} {
		if _, err := owner.Exec(ctx, `
			INSERT INTO activity_participant (activity_id, `+p.column+`, role) VALUES ($1, $2, $3)`,
			activity, p.id, p.role); err != nil {
			t.Fatal(err)
		}
	}
	if made {
		if _, err := owner.Exec(ctx, `
			INSERT INTO audit_log (id, actor_type, actor_id, on_behalf_of, action, entity_type, entity_id)
			VALUES ($1, 'connector', 'connector:gmail', $2, 'create', 'activity', $3)`,
			ids.NewV7(), colleague, activity); err != nil {
			t.Fatal(err)
		}
	}
	wsCtx := principal.WithWorkspaceID(ctx, e.WS)
	if err := database.WithWorkspaceTx(wsCtx, e.Pool, func(tx pgx.Tx) error {
		return search.RecomputeEdgesForActivities(wsCtx, tx, []ids.UUID{activity})
	}); err != nil {
		t.Fatalf("folding the connector mail: %v", err)
	}
}

// A guessed seat row does not end a silence. The contact's new mail reached a
// mailbox nobody can name, so no live colleague has heard from them, and the
// relationship stays quiet. The same mail captured for the colleague ends it.
func TestAGuessedSeatRowDoesNotResetTheQuietClock(t *testing.T) {
	e := Setup(t)
	guessed := e.SeedContact(t, "Wrote To A Guessed Seat", &e.Rep1)
	captured := e.SeedContact(t, "Wrote To Rep One", &e.Rep1)
	seedLapsedPair(t, e, e.Rep1, guessed, "the lapsed thread")
	seedLapsedPair(t, e, e.Rep1, captured, "the other lapsed thread")
	seedConnectorMailFrom(t, e, guessed, e.Rep1, false)
	seedConnectorMailFrom(t, e, captured, e.Rep1, true)

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, graphPerms)
	now := clocktest.Now(t).UTC()
	var quiet []search.QuietContact
	err := database.WithWorkspaceTx(rep, e.Pool, func(tx pgx.Tx) error {
		var err error
		quiet, err = search.QuietContacts(rep, tx, search.QuietRead{
			QuietBefore: now.AddDate(0, 0, -relstrength.QuietDays), TouchedAfter: now.AddDate(0, 0, -180), Limit: 10,
		})
		return err
	})
	if err != nil {
		t.Fatalf("reading quiet candidates: %v", err)
	}
	got := map[ids.UUID]bool{}
	for _, candidate := range quiet {
		got[candidate.Edge.ContactID] = true
	}
	if !got[guessed] {
		t.Error("a guessed seat row reset the quiet clock of a relationship nobody answered")
	}
	if got[captured] {
		t.Error("mail captured for the colleague did not end the silence")
	}
}
