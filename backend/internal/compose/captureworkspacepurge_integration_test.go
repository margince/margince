// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What an admin's workspace-wide purge destroys, who may ask for it, and what it
// still refuses.
//
// The seat purge answers "delete what my connection brought in". This answers
// "delete what this workspace captured", across every colleague's mailbox — so
// the authority is the question these tests spend most of their length on.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAnAdminPurgesAWorkspaceRuleAcrossEverySeat(t *testing.T) {
	e := integration.Setup(t)
	mine := seedPurgeableMail(t, e, "spam@junk.example", "an uns", e.Rep1)
	theirs := seedPurgeableMail(t, e, "spam@junk.example", "an die Kollegin", e.Rep2)

	rule := seedWorkspaceExclusion(t, e, "junk.example")
	outcome := runPurgeAs(adminCtx(e), t, e, rule, false)

	// Both, and neither released: a workspace rule is not one seat reaching into
	// another's mail, so there is no colleague whose claim survives.
	if outcome.Destroyed != 2 || outcome.Released != 0 {
		t.Fatalf("destroyed=%d released=%d, want 2 and 0", outcome.Destroyed, outcome.Released)
	}
	for name, id := range map[string]ids.UUID{"this seat's": mine, "the colleague's": theirs} {
		if body := activityBody(t, e, id); body != "" {
			t.Errorf("%s message kept its body %q after a workspace purge", name, body)
		}
	}
}

func TestAnOpsSeatThatCanWriteTheRuleIsRefusedThePurge(t *testing.T) {
	// The gate that is easy to get wrong. Creating a workspace exclusion takes
	// capture_settings.update, which admin AND ops both hold, so inferring the
	// authority from "could they have written this rule" hands workspace-wide
	// destruction to an ops seat.
	e := integration.Setup(t)
	mail := seedPurgeableMail(t, e, "spam@junk.example", "an uns", e.Rep1)
	rule := seedWorkspaceExclusion(t, e, "junk.example")

	_, err := purgerFor(t, e).Purge(opsCtx(e), rule, false)
	if err == nil {
		t.Fatal("an ops seat purged a workspace rule: the grant that writes the rule is not " +
			"the authority that destroys every colleague's mail")
	}
	if body := activityBody(t, e, mail); body == "" {
		t.Error("the refused purge destroyed mail anyway")
	}
}

func TestARepIsRefusedAWorkspacePurge(t *testing.T) {
	e := integration.Setup(t)
	seedPurgeableMail(t, e, "spam@junk.example", "an uns", e.Rep1)
	rule := seedWorkspaceExclusion(t, e, "junk.example")

	_, err := purgerFor(t, e).Purge(purgeCtx(e, e.Rep1), rule, false)
	if err == nil {
		t.Fatal("an ordinary seat purged a workspace rule")
	}
}

func TestAWorkspacePurgeReachesMailTheRuleCaughtThroughCc(t *testing.T) {
	// Ingress keeps a message out if ANY address on it matches — sender,
	// recipients, copies. A purge that read only counterparty_email would miss
	// the history the rule caught through Cc and report mail as gone while it
	// sat there.
	e := integration.Setup(t)
	viaCc := seedPurgeableMail(t, e, "kunde@example.test", "Angebot", e.Rep1)
	addParticipantAddress(t, e, viaCc, "verkauf@junk.example")

	rule := seedWorkspaceExclusion(t, e, "junk.example")
	outcome := runPurgeAs(adminCtx(e), t, e, rule, false)

	if outcome.Destroyed != 1 {
		t.Fatalf("destroyed=%d, want 1 — the rule caught this message through its Cc line",
			outcome.Destroyed)
	}
	if body := activityBody(t, e, viaCc); body != "" {
		t.Error("a message the rule matched only through Cc survived the purge that reported it gone")
	}
}

func TestAWorkspacePurgeLeavesActivityNobodyCaptured(t *testing.T) {
	// A capture exclusion governs what capture stores. A meeting somebody logged
	// by hand was never captured and no rule ever kept it out, so destroying it
	// takes away work a person did.
	e := integration.Setup(t)
	handLogged := seedHandLoggedMail(t, e, "spam@junk.example", "selbst notiert")
	rule := seedWorkspaceExclusion(t, e, "junk.example")

	outcome := runPurgeAs(adminCtx(e), t, e, rule, false)
	if outcome.Destroyed != 0 {
		t.Fatalf("destroyed=%d, want 0 — nothing here was ever captured", outcome.Destroyed)
	}
	if body := activityBody(t, e, handLogged); body == "" {
		t.Error("a hand-logged activity was destroyed by a CAPTURE exclusion rule")
	}
}

func TestAWorkspacePurgeReportsRestrictedMailRatherThanDestroyingIt(t *testing.T) {
	e := integration.Setup(t)
	held := seedPurgeableMail(t, e, "spam@junk.example", "unter Aufbewahrung", e.Rep1)
	restrict(t, e, held)

	rule := seedWorkspaceExclusion(t, e, "junk.example")
	outcome := runPurgeAs(adminCtx(e), t, e, rule, false)

	if outcome.Destroyed != 0 || outcome.Skipped != 1 {
		t.Fatalf("destroyed=%d skipped=%d, want 0 and 1: an admin told their mail is gone must "+
			"not find it still there", outcome.Destroyed, outcome.Skipped)
	}
	if body := activityBody(t, e, held); body == "" {
		t.Error("a restricted message was destroyed by a workspace purge")
	}
}

func TestAWorkspacePurgePreviewChangesNothing(t *testing.T) {
	e := integration.Setup(t)
	mail := seedPurgeableMail(t, e, "spam@junk.example", "an uns", e.Rep1)
	rule := seedWorkspaceExclusion(t, e, "junk.example")

	outcome := runPurgeAs(adminCtx(e), t, e, rule, true)
	if !outcome.Preview || outcome.Destroyed != 1 {
		t.Fatalf("preview=%v destroyed=%d, want true and 1", outcome.Preview, outcome.Destroyed)
	}
	if body := activityBody(t, e, mail); body == "" {
		t.Error("the preview destroyed the mail it was only supposed to count")
	}
}

func TestAnAdminStillPurgesTheirOwnSeatRuleAsASeat(t *testing.T) {
	// The admin arm must not swallow the seat arm: an admin's own rule is still
	// their own, so it destroys what THEY imported and releases the rest.
	e := integration.Setup(t)
	mine := seedPurgeableMail(t, e, "spam@junk.example", "nur meine", e.AdminUser)
	shared := seedPurgeableMail(t, e, "spam@junk.example", "auch die Kollegin", e.AdminUser)
	addImporter(t, e, shared, e.Rep2)

	rule := seedOwnExclusion(t, e, e.AdminUser, capture.ExclusionKindDomain, "junk.example")
	outcome := runPurgeAs(adminCtx(e), t, e, rule, false)

	if outcome.Destroyed != 1 || outcome.Released != 1 {
		t.Fatalf("destroyed=%d released=%d, want 1 and 1 — an admin's OWN rule is still a seat rule",
			outcome.Destroyed, outcome.Released)
	}
	if body := activityBody(t, e, shared); body == "" {
		t.Error("the colleague's copy was destroyed by an admin acting on their own seat rule")
	}
	if body := activityBody(t, e, mine); body != "" {
		t.Error("the admin's own sole-imported message survived their own seat rule")
	}
}

func TestAWorkspaceAddressRuleMatchesThatAddressAlone(t *testing.T) {
	// An address rule is exact where a domain rule covers subdomains. Both reach
	// the participant arm, so this also pins that the Cc match is not domain-only.
	e := integration.Setup(t)
	named := seedPurgeableMail(t, e, "verkauf@junk.example", "direkt", e.Rep1)
	sibling := seedPurgeableMail(t, e, "andere@junk.example", "gleiche Domain", e.Rep1)

	rule := seedWorkspaceExclusion(t, e, "verkauf@junk.example")
	outcome := runPurgeAs(adminCtx(e), t, e, rule, false)

	if outcome.Destroyed != 1 {
		t.Fatalf("destroyed=%d, want 1 — an address rule is that address and no other",
			outcome.Destroyed)
	}
	if body := activityBody(t, e, named); body != "" {
		t.Error("the named address survived its own rule")
	}
	if body := activityBody(t, e, sibling); body == "" {
		t.Error("an address rule destroyed mail from a different address on the same domain")
	}
}

func TestAWorkspacePurgeLeavesNoClaimOnTheMailItDestroyed(t *testing.T) {
	// What a colleague is left holding. The workspace arm releases nothing,
	// because the workspace is not one seat reaching into another's mailbox —
	// but "released nothing" must not mean every seat keeps an import row and a
	// participant row pointing at a body-nulled activity. That is a stub on
	// their timeline they cannot get rid of, and the contract says the mail is
	// gone.
	e := integration.Setup(t)
	mail := seedPurgeableMail(t, e, "spam@junk.example", "an uns", e.Rep1)
	addImporter(t, e, mail, e.Rep2)

	rule := seedWorkspaceExclusion(t, e, "junk.example")
	runPurgeAs(adminCtx(e), t, e, rule, false)

	for name, seat := range map[string]ids.UUID{"the first seat": e.Rep1, "the colleague": e.Rep2} {
		if n := importCount(t, e, mail, seat); n != 0 {
			t.Errorf("%s still holds %d import rows for destroyed mail, want 0", name, n)
		}
	}
	if n := participantRows(t, e, mail); n != 0 {
		t.Errorf("%d participant rows survive the destroyed message, want 0: each one is a stub "+
			"on somebody's timeline for mail the workspace was told is gone", n)
	}
}

// participantRows counts what still points at a message.
func participantRows(t *testing.T, e *integration.Env, activityID ids.UUID) int {
	t.Helper()
	var n int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM activity_participant WHERE activity_id = $1`, activityID).Scan(&n)
	}); err != nil {
		t.Fatalf("counting participant rows: %v", err)
	}
	return n
}

func TestAColleagueOnTheCcLineIsNotTheCounterpartyARuleIsAbout(t *testing.T) {
	// The blast radius an ops seat can arm. Writing a workspace rule takes
	// capture_settings.update, which ops holds; only pressing purge takes admin.
	// So the rule's REACH is chosen by whoever wrote it, and if a colleague's own
	// address on a Cc line counts as a match, a rule naming the workspace's own
	// mail domain matches essentially every captured message — and the admin who
	// presses it sees a plausible domain cleanup and a count.
	e := integration.Setup(t)
	customer := seedPurgeableMail(t, e, "kunde@example.test", "Angebot", e.Rep1)
	addColleagueParticipant(t, e, customer, "kollegin@ourcompany.test", e.Rep2)

	rule := seedWorkspaceExclusion(t, e, "ourcompany.test")
	outcome := runPurgeAs(adminCtx(e), t, e, rule, false)

	if outcome.Destroyed != 0 {
		t.Fatalf("destroyed=%d, want 0 — a colleague copied on a customer's mail is not the "+
			"counterparty an exclusion rule is about", outcome.Destroyed)
	}
	if body := activityBody(t, e, customer); body == "" {
		t.Error("a customer's mail was destroyed because a colleague was Cc'd on it")
	}
}

// addColleagueParticipant records a COLLEAGUE on a message, the way
// StampFurtherParticipants does when the provider attests our own side.
func addColleagueParticipant(t *testing.T, e *integration.Env, activityID ids.UUID, address string, seat ids.UUID) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO activity_participant (activity_id, user_id, address, role)
			VALUES ($1, $3, $2, 'cc') ON CONFLICT DO NOTHING`, activityID, address, seat)
		return err
	}); err != nil {
		t.Fatalf("stamping a colleague participant: %v", err)
	}
}

func TestAWorkspaceRuleIsNotAnExistenceOracleForANonAdmin(t *testing.T) {
	// One answer for three questions: a rule that does not exist, one that is
	// somebody else's, and one this caller may not act on. Two different answers
	// let a rep probe ids and read which workspace rules exist — what the
	// workspace has decided to keep out is itself something to learn.
	e := integration.Setup(t)
	existing := seedWorkspaceExclusion(t, e, "junk.example")

	_, onReal := purgerFor(t, e).Purge(purgeCtx(e, e.Rep1), existing, true)
	_, onInvented := purgerFor(t, e).Purge(purgeCtx(e, e.Rep1), ids.NewV7(), true)
	if onReal == nil || onInvented == nil {
		t.Fatal("a rep was allowed to purge a workspace rule")
	}
	if !errors.Is(onReal, apperrors.ErrNotFound) || !errors.Is(onInvented, apperrors.ErrNotFound) {
		t.Fatalf("onReal=%v onInvented=%v, want both ErrNotFound: a 403 on the real id and a 500 "+
			"on the invented one tells a prober which workspace rules exist", onReal, onInvented)
	}
}

// seedWorkspaceExclusion writes the rule through the store production writes it
// with, as the admin who may.
func seedWorkspaceExclusion(t *testing.T, e *integration.Env, value string) ids.UUID {
	t.Helper()
	store := capture.NewExclusionStore(InstallationDB(e.Pool))
	kind := capture.ExclusionKindDomain
	if strings.Contains(value, "@") {
		kind = capture.ExclusionKindAddress
	}
	rule, err := store.Add(adminCtx(e), capture.ExclusionScopeWorkspace, kind, value)
	if err != nil {
		t.Fatalf("adding the workspace exclusion rule: %v", err)
	}
	return rule.ID
}

// adminCtx is purgeCtx carrying the workspace admin role.
func adminCtx(e *integration.Env) context.Context {
	return roleCtx(e, e.AdminUser, "admin")
}

// opsCtx carries the ops role, which holds capture_settings.update — the grant
// that writes a workspace rule and, deliberately, not the authority to purge it.
func opsCtx(e *integration.Env) context.Context {
	return roleCtx(e, e.Rep3, "ops")
}

func roleCtx(e *integration.Env, user ids.UUID, role string) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"activity":         {Read: true, Update: true, Delete: true},
				"person":           {Read: true, Create: true, Update: true, Delete: true},
				"capture_settings": {Read: true, Create: true, Update: true},
			},
			RowScope: principal.RowScopeAll,
			RoleKeys: []string{role},
		},
	})
}

func runPurgeAs(ctx context.Context, t *testing.T, e *integration.Env, rule ids.UUID, preview bool) PurgeOutcome {
	t.Helper()
	outcome, err := purgerFor(t, e).Purge(ctx, rule, preview)
	if err != nil {
		t.Fatalf("purging: %v", err)
	}
	return outcome
}

// addParticipantAddress records one more address on a message, the way
// StampFurtherParticipants records a Cc line at capture.
func addParticipantAddress(t *testing.T, e *integration.Env, activityID ids.UUID, address string) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO activity_participant (activity_id, address, role)
			VALUES ($1, $2, 'cc') ON CONFLICT DO NOTHING`, activityID, address)
		return err
	}); err != nil {
		t.Fatalf("stamping a further participant: %v", err)
	}
}

// seedHandLoggedMail lands an activity with NO capture_import row: somebody
// typed it in, and no connector ever brought it.
func seedHandLoggedMail(t *testing.T, e *integration.Env, from, subject string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO activity (id, kind, subject, body, direction, source, captured_by,
			                      counterparty_email, audience)
			VALUES ($1, 'email', $2, 'the message body', 'inbound', 'manual',
			        'human:'||$3, $4, 'workspace')`,
			id, subject, e.Rep1.String(), from)
		return err
	}); err != nil {
		t.Fatalf("seeding hand-logged mail: %v", err)
	}
	return id
}
