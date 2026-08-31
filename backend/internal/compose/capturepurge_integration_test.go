// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What an owner's purge destroys, and — the harder half — what it leaves alone.
//
// A message is stored once however many mailboxes received it, so "delete what
// my connection brought in" cannot mean "delete every message matching the
// rule". These tests are mostly about the second half: a colleague's copy, a
// contact somebody did work against, a message under a statutory hold.

import (
	"context"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAPurgeDestroysOnlyWhatThisConnectionBroughtIn(t *testing.T) {
	e := integration.Setup(t)
	// Two messages from the same sender: one only this seat imported, one a
	// colleague imported too.
	mine := seedPurgeableMail(t, e, "anwalt@kanzlei.example", "nur meine", e.Rep1)
	shared := seedPurgeableMail(t, e, "anwalt@kanzlei.example", "auch bei der Kollegin", e.Rep1)
	addImporter(t, e, shared, e.Rep2)

	rule := seedOwnExclusion(t, e, e.Rep1, capture.ExclusionKindDomain, "kanzlei.example")
	outcome := runPurge(t, e, e.Rep1, rule, false)

	if outcome.Destroyed != 1 || outcome.Released != 1 {
		t.Fatalf("destroyed=%d released=%d, want 1 and 1 — the shared message is the colleague's too",
			outcome.Destroyed, outcome.Released)
	}
	// The message only this seat had: gone, text and all.
	if body := activityBody(t, e, mine); body != "" {
		t.Fatalf("a solely-imported message kept its body %q", body)
	}
	// The colleague's: untouched, and still theirs.
	if body := activityBody(t, e, shared); body == "" {
		t.Fatal("a message the colleague also imported lost its body — their correspondence is not this owner's to destroy")
	}
	if n := importCount(t, e, shared, e.Rep2); n != 1 {
		t.Fatalf("the colleague has %d import rows for the shared message, want 1", n)
	}
	// And this seat's own claim on it is gone, which is what they asked for.
	if n := importCount(t, e, shared, e.Rep1); n != 0 {
		t.Fatalf("the purging seat still holds %d import rows for the shared message, want 0", n)
	}
}

func TestAPreviewChangesNothing(t *testing.T) {
	e := integration.Setup(t)
	id := seedPurgeableMail(t, e, "reise@buero.example", "Flug", e.Rep1)
	rule := seedOwnExclusion(t, e, e.Rep1, capture.ExclusionKindAddress, "reise@buero.example")

	preview := runPurge(t, e, e.Rep1, rule, true)
	if preview.Destroyed != 1 || !preview.Preview {
		t.Fatalf("preview reported destroyed=%d preview=%v, want 1 and true", preview.Destroyed, preview.Preview)
	}
	if body := activityBody(t, e, id); body == "" {
		t.Fatal("a preview destroyed the message it was only supposed to count")
	}
	// The counts an owner is shown are the counts they get: both arms run the
	// same selection, so a preview that disagreed with the purge would be a
	// promise the product could not keep.
	real := runPurge(t, e, e.Rep1, rule, false)
	if real.Destroyed != preview.Destroyed {
		t.Fatalf("the purge destroyed %d where the preview said %d", real.Destroyed, preview.Destroyed)
	}
}

func TestAPurgeIsIdempotent(t *testing.T) {
	e := integration.Setup(t)
	seedPurgeableMail(t, e, "schule@example.test", "Elternabend", e.Rep1)
	rule := seedOwnExclusion(t, e, e.Rep1, capture.ExclusionKindAddress, "schule@example.test")

	if first := runPurge(t, e, e.Rep1, rule, false); first.Destroyed != 1 {
		t.Fatalf("the first purge destroyed %d, want 1", first.Destroyed)
	}
	// The activity survives as a tombstone with its content gone, so a second
	// pass still SELECTS it. What matters is that running twice is safe and
	// says so, not that the row disappears.
	second := runPurge(t, e, e.Rep1, rule, false)
	if second.Released != 0 {
		t.Fatalf("the second purge released %d, want 0", second.Released)
	}
}

func TestAPurgeRefusesARuleThatIsNotYours(t *testing.T) {
	e := integration.Setup(t)
	seedPurgeableMail(t, e, "privat@example.test", "Familie", e.Rep1)
	rule := seedOwnExclusion(t, e, e.Rep1, capture.ExclusionKindAddress, "privat@example.test")

	// A rule id is a guess anybody can make, so the id alone must buy nothing.
	if _, err := purgerFor(t, e).Purge(purgeCtx(e, e.Rep2), rule, false); err == nil {
		t.Fatal("a colleague purged another seat's rule — the id is not the authority")
	}
}

func TestAPurgeSkipsARestrictedMessageAndSaysSo(t *testing.T) {
	// A statutory hold or an open erasure outranks an owner's rule: the row is
	// somebody else's obligation until that lifts, and an owner told their mail
	// is gone must not find it still there.
	//
	// Restricted through the retention CLASS and the columns the product's own
	// restriction writer sets, not through a bare restricted_at — the table
	// refuses that, and a fixture the real writer would never produce proves
	// nothing about the real writer.
	e := integration.Setup(t)
	held := seedPurgeableMail(t, e, "gegner@example.test", "Klage", e.Rep1)
	restrict(t, e, held)
	rule := seedOwnExclusion(t, e, e.Rep1, capture.ExclusionKindAddress, "gegner@example.test")

	outcome := runPurge(t, e, e.Rep1, rule, false)
	if outcome.Skipped != 1 || outcome.Destroyed != 0 {
		t.Fatalf("skipped=%d destroyed=%d, want 1 and 0 — a statutory hold outranks an owner's rule",
			outcome.Skipped, outcome.Destroyed)
	}
	if body := activityBody(t, e, held); body == "" {
		t.Fatal("a restricted message was destroyed; the hold is somebody else's obligation")
	}
}

// purgeCtx is a seat asking for its own purge: a human principal, which is the
// authority the whole path turns on.
func purgeCtx(e *integration.Env, user ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"activity": {Read: true, Update: true, Delete: true},
				"person":   {Read: true, Update: true, Delete: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

func purgerFor(t *testing.T, e *integration.Env) *CapturePurger {
	t.Helper()
	return NewCapturePurger(e.Pool, NewRetentionServiceFor(InstallationDB(e.Pool), nil, slog.Default()))
}

func runPurge(t *testing.T, e *integration.Env, user, rule ids.UUID, preview bool) PurgeOutcome {
	t.Helper()
	outcome, err := purgerFor(t, e).Purge(purgeCtx(e, user), rule, preview)
	if err != nil {
		t.Fatalf("purging: %v", err)
	}
	return outcome
}

// seedOwnExclusion writes the rule through the store that writes it in
// production, so what the purge reads back is the row a person's own click
// would have made.
func seedOwnExclusion(t *testing.T, e *integration.Env, user ids.UUID, kind, value string) ids.UUID {
	t.Helper()
	store := capture.NewExclusionStore(InstallationDB(e.Pool))
	rule, err := store.Add(purgeCtx(e, user), capture.ExclusionScopeUser, kind, value)
	if err != nil {
		t.Fatalf("adding the exclusion rule: %v", err)
	}
	return rule.ID
}

// seedPurgeableMail lands one captured message with the import row that makes
// the seeding seat its importer — the row the purge's whole selection turns on.
func seedPurgeableMail(t *testing.T, e *integration.Env, from, subject string, user ids.UUID) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO activity (id, kind, subject, body, direction, source_system, source_id,
			                      source, captured_by, counterparty_email, audience)
			VALUES ($1, 'email', $2, 'the message body', 'inbound', 'gmail', $3,
			        'gmail:'||$3, 'connector:gmail', $4, 'participants')`,
			id, subject, "pg-"+id.String(), from); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_import (activity_id, user_id, posture_at_import)
			VALUES ($1, $2, 'classified')`, id, user)
		return err
	}); err != nil {
		t.Fatalf("seeding purgeable mail: %v", err)
	}
	return id
}

func addImporter(t *testing.T, e *integration.Env, activityID, user ids.UUID) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_import (activity_id, user_id, posture_at_import)
			VALUES ($1, $2, 'classified')`, activityID, user)
		return err
	}); err != nil {
		t.Fatalf("adding a second importer: %v", err)
	}
}

// restrict puts a message under a statutory hold, setting every column the
// table's own CHECKs require together: a restriction carries a reason, an end
// date, a retention class and an archive stamp, or it is not a restriction.
//
// Written here rather than through privacy's restriction writer because that
// path needs a retention policy and a qualifying event this test has no reason
// to build — what is under test is that the PURGE respects the state, not how
// the state is reached.
func restrict(t *testing.T, e *integration.Env, activityID ids.UUID) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		// The EVIDENCE first. A restriction that recorded no reason for
		// existing would be a hold nobody could later justify or lift, so the
		// table refuses one — a controller's pin, attributed and reasoned.
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO activity_retention_evidence
			  (activity_id, basis, decided_by_name, reason, qualified_at)
			VALUES ($1, 'controller_pin', 'Test Controller', 'statutory retention under test', now())`,
			activityID); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `
			UPDATE activity
			   SET retention_class = 'commercial_correspondence',
			       retention_class_at = now(),
			       archived_at = coalesce(archived_at, now()),
			       restricted_at = now(),
			       restricted_reason = 'statutory retention',
			       restricted_until = now() + interval '6 years'
			 WHERE id = $1`, activityID)
		return err
	}); err != nil {
		t.Fatalf("restricting the message: %v", err)
	}
}

func activityBody(t *testing.T, e *integration.Env, id ids.UUID) string {
	t.Helper()
	var body string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT coalesce(body, '') FROM activity WHERE id = $1`, id).Scan(&body)
	}); err != nil {
		t.Fatalf("reading the message body: %v", err)
	}
	return body
}

func importCount(t *testing.T, e *integration.Env, activityID, user ids.UUID) int {
	t.Helper()
	var n int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM capture_import WHERE activity_id = $1 AND user_id = $2`,
			activityID, user).Scan(&n)
	}); err != nil {
		t.Fatalf("counting import rows: %v", err)
	}
	return n
}

func TestADomainRuleDoesNotReachALookalikeDomain(t *testing.T) {
	// `kanzlei.example` must reach `mail.kanzlei.example` and must NOT reach
	// `evil-kanzlei.example`. The leading `@` and `.` in the LIKE pattern are
	// the whole difference: without them a rule would match any domain ENDING
	// in the named one, and an attacker who registered a lookalike could have
	// somebody else's correspondence destroyed by their own rule.
	e := integration.Setup(t)
	named := seedPurgeableMail(t, e, "anwalt@kanzlei.example", "genannt", e.Rep1)
	sub := seedPurgeableMail(t, e, "anwalt@mail.kanzlei.example", "unterdomain", e.Rep1)
	lookalike := seedPurgeableMail(t, e, "anwalt@evil-kanzlei.example", "nachgemacht", e.Rep1)

	rule := seedOwnExclusion(t, e, e.Rep1, capture.ExclusionKindDomain, "kanzlei.example")
	outcome := runPurge(t, e, e.Rep1, rule, false)

	if outcome.Destroyed != 2 {
		t.Fatalf("destroyed=%d, want 2 — the domain and its subdomain, and nothing else", outcome.Destroyed)
	}
	if body := activityBody(t, e, named); body != "" {
		t.Error("the named domain's message survived")
	}
	if body := activityBody(t, e, sub); body != "" {
		t.Error("a subdomain of the named domain survived; a rule covers what sits under it")
	}
	if body := activityBody(t, e, lookalike); body == "" {
		t.Fatal("a lookalike domain's message was destroyed — the rule named kanzlei.example, not everything ending in it")
	}
}

func TestAPurgeAsksTheStatutoryFloorBeforeDestroying(t *testing.T) {
	// GoBD §147 AO: a Handelsbrief must be kept for years, and the nightly
	// retention evaluator refuses to touch one inside its window. A purge is a
	// destructive activity path like any other and applies the same shield —
	// without it this would be the one path in the tree that lets an owner
	// destroy what the law requires the installation to hold.
	//
	// What this test can assert depends on the build. The floor comes from a
	// compiled-in jurisdiction pack, and a build with none (the default) has a
	// zero-length window: nothing is shielded, and a purge destroying a
	// Handelsbrief is then correct rather than a bypass. So the assertion
	// follows the pack: with a window, the message is withheld; without one, it
	// goes. Asserting "withheld" unconditionally would fail on the default
	// build for a reason that is not a defect.
	e := integration.Setup(t)
	brief := seedPurgeableMail(t, e, "einkauf@kunde.example", "Auftragsbestätigung", e.Rep1)
	stampCommercialCorrespondence(t, e, brief)
	rule := seedOwnExclusion(t, e, e.Rep1, capture.ExclusionKindAddress, "einkauf@kunde.example")

	shielded := statutoryWindowIsOpen(t, e, brief)
	outcome := runPurge(t, e, e.Rep1, rule, false)

	if shielded {
		if outcome.Skipped != 1 || outcome.Destroyed != 0 {
			t.Fatalf("skipped=%d destroyed=%d, want 1 and 0 — the statutory window is not an owner's to close",
				outcome.Skipped, outcome.Destroyed)
		}
		if body := activityBody(t, e, brief); body == "" {
			t.Fatal("a Handelsbrief inside its retention window was destroyed")
		}
		return
	}
	if outcome.Destroyed != 1 {
		t.Fatalf("destroyed=%d, want 1 — this build declares no retention window, so nothing shields the message",
			outcome.Destroyed)
	}
}

// statutoryWindowIsOpen asks the DATABASE the same question the purge asks, so
// the test's expectation comes from the installation's own floor rather than
// from an assumption about which jurisdiction packs this binary carries.
func statutoryWindowIsOpen(t *testing.T, e *integration.Env, activityID ids.UUID) bool {
	t.Helper()
	interval, anchor := privacy.StatutoryFloorArgs()
	var open bool
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT (`+privacy.StatutoryFloorShield(2, 3)+`) FROM activity a WHERE a.id = $1`,
			activityID, interval, anchor).Scan(&open)
	}); err != nil {
		t.Fatalf("asking whether the statutory window is open: %v", err)
	}
	return open
}

// stampCommercialCorrespondence marks a message as the kind of correspondence
// the law requires keeping, dated so its window is still open.
func stampCommercialCorrespondence(t *testing.T, e *integration.Env, activityID ids.UUID) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			UPDATE activity
			   SET retention_class = 'commercial_correspondence',
			       retention_class_at = now(),
			       occurred_at = now() - interval '30 days'
			 WHERE id = $1`, activityID)
		return err
	}); err != nil {
		t.Fatalf("stamping commercial correspondence: %v", err)
	}
}
