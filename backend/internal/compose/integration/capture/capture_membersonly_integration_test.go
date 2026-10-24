// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// Mail between colleagues, through the production sink: whichever addresses
// they used, the workspace does not read it unless a member shares it.

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// colleagueLogin is Rep3's login address in the search harness. It is on no
// own domain, so capture keeps mail to it: a second work domain, as a member
// at a sister company has.
const colleagueLogin = "rep1@search.test"

// colleaguePrivate is an address Rep3 declares as their own.
const colleaguePrivate = "b.private@home.example"

// readsContent asks the content gate itself whether one seat reads the mail.
func readsContent(t *testing.T, e *integration.SearchEnv, user, activityID ids.UUID) bool {
	t.Helper()
	ctx := principal.WithActor(principal.WithWorkspaceID(context.Background(), e.WS), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"activity": {Read: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	var reads bool
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		args := []any{activityID}
		clause, err := auth.ActivityContentClause(ctx, "a", func(v any) int { args = append(args, v); return len(args) })
		if err != nil {
			return err
		}
		return tx.QueryRow(ctx, fmt.Sprintf(
			`SELECT EXISTS (SELECT 1 FROM activity a WHERE a.id = $1 AND %s)`, clause), args...).Scan(&reads)
	}); err != nil {
		t.Fatalf("asking the content gate: %v", err)
	}
	return reads
}

// sentToColleague is the owner's own mail to Rep3, attested as sent.
func sentToColleague(env captureEnv, t *testing.T, to, cc, msgID string) ids.UUID {
	t.Helper()
	raw := email(captureOwner, "Owner", to, msgID, "")
	if cc != "" {
		raw = emailCC(captureOwner, "Owner", to, cc, msgID)
	}
	env.syncSent(t, map[string]bool{msgID: true}, raw)
	return activityIDOf(t, env.e, msgID)
}

func TestMailBetweenColleaguesIsHeldFromTheWorkspace(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	colleagueMail := sentToColleague(env, t, colleagueLogin, "", "members-1@myco.example")

	if got, reason := audienceOf(t, e, colleagueMail); got != "participants" || reason != activities.ReasonMembersOnly {
		t.Fatalf("mail between two members is %q / %q, want participants / members_only", got, reason)
	}
	if !readsContent(t, e, e.Rep1, colleagueMail) {
		t.Error("the mailbox owner cannot read their own mail")
	}
	if !readsContent(t, e, e.Rep3, colleagueMail) {
		t.Error("the colleague it was sent to cannot read it")
	}
	if readsContent(t, e, ids.NewV7(), colleagueMail) {
		t.Error("a third seat reads mail between two colleagues")
	}

	recordThreadVerdict(t, e, e.Rep1, colleagueMail, capturemod.VerdictCleared, "")
	if got, reason := audienceOf(t, e, colleagueMail); got != "participants" || reason != activities.ReasonMembersOnly {
		t.Fatalf("a cleared verdict moved mail between colleagues to %q / %q", got, reason)
	}

	shareThreadAsOwner(t, e, e.Rep1, colleagueMail)
	if got, _ := audienceOf(t, e, colleagueMail); got != "workspace" {
		t.Fatalf("the owner shared mail between colleagues and it stayed %q", got)
	}
}

// A declared identity is a claim nobody proved, so it makes no member. If it
// did, one seat claiming an outsider's address would hold every colleague's
// mail with that outsider.
func TestAnUnprovedClaimMakesNoMember(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	declareIdentity(t, e, e.Rep3, capturemod.IdentityKindAddress, colleaguePrivate)
	private := sentToColleague(env, t, colleaguePrivate, "", "members-private@myco.example")
	if _, reason := audienceOf(t, e, private); reason == activities.ReasonMembersOnly {
		t.Error("a colleague's declared address alone made the mail members-only")
	}

	syncColleague := secondMailbox(t, e, e.Rep3)
	declareIdentity(t, e, e.Rep1, capturemod.IdentityKindAddress, "x@outside.example")
	syncColleague(t, email("x@outside.example", "X Outside", secondSeatAddress, "members-claimed@outside.example", ""))
	claimed := activityIDOf(t, e, "members-claimed@outside.example")
	if _, reason := audienceOf(t, e, claimed); reason == activities.ReasonMembersOnly {
		t.Error("one seat claiming an outsider's address held a colleague's mail with that outsider")
	}
}

func TestAProvedMailboxAddressCountsAsAMember(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	seedCaptureRoleFor(t, e, e.Rep3)
	mailboxFor(t, e, e.Rep3, "rep3@sister.example", compose.CaptureConfig{})
	toMailbox := sentToColleague(env, t, "rep3@sister.example", "", "members-mailbox@myco.example")
	if _, reason := audienceOf(t, e, toMailbox); reason != activities.ReasonMembersOnly {
		t.Fatalf("mail to a colleague's connected mailbox reads %q, want members_only", reason)
	}

	archiveSeat(t, e, e.Rep3)
	afterArchive := sentToColleague(env, t, "rep3@sister.example", "", "members-mailbox-archived@myco.example")
	if _, reason := audienceOf(t, e, afterArchive); reason == activities.ReasonMembersOnly {
		t.Fatal("an archived seat's mailbox still made the mail members-only")
	}
}

func archiveSeat(t *testing.T, e *integration.SearchEnv, seat ids.UUID) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `UPDATE app_user SET archived_at = now() WHERE id = $1`, seat)
		return err
	}); err != nil {
		t.Fatalf("archiving the colleague: %v", err)
	}
}

func TestOneOutsidePartyKeepsTheMailAsBefore(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	withOutsider := sentToColleague(env, t, colleagueLogin, "ext@example.org", "members-ext@myco.example")

	if _, reason := audienceOf(t, e, withOutsider); reason == activities.ReasonMembersOnly {
		t.Fatal("mail copied to an outside party was held as mail between colleagues")
	}
	recordThreadVerdict(t, e, e.Rep1, withOutsider, capturemod.VerdictCleared, "")
	if got, _ := audienceOf(t, e, withOutsider); got != "workspace" {
		t.Fatalf("a cleared verdict left mail with an outside party %q, want workspace", got)
	}
}

// A reply from a party a cleared thread's verdict never saw re-opens the
// question and inherits `pending`. The row must name the verdict-proof hold
// rather than the wait: the recompute keeps only the former once the verdict
// clears again.
func TestAColleaguesReplyOnAReopenedThreadStaysHeldOnceItClears(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	env.sync(t, customerMail("members-open@acme.example"))
	recordThreadVerdict(t, e, e.Rep1, activityIDOf(t, e, "members-open@acme.example"), capturemod.VerdictCleared, "")
	env.syncSent(t, map[string]bool{"members-reply@myco.example": true},
		email(captureOwner, "Owner", colleagueLogin, "members-reply@myco.example", "members-open@acme.example"))
	if status := threadVerdictStatus(t, e, e.Rep1, "members-open@acme.example"); status != capturemod.VerdictPending {
		t.Fatalf("the fixture needs the reply to re-open the thread, got %q", status)
	}
	reply := activityIDOf(t, e, "members-reply@myco.example")
	if got, reason := audienceOf(t, e, reply); got != "participants" || reason != activities.ReasonMembersOnly {
		t.Fatalf("a colleague's reply on a re-opened thread is %q / %q, want participants / members_only", got, reason)
	}

	recordThreadVerdict(t, e, e.Rep1, reply, capturemod.VerdictCleared, "")
	if got, reason := audienceOf(t, e, reply); got != "participants" || reason != activities.ReasonMembersOnly {
		t.Fatalf("the cleared verdict opened a colleague's reply: %q / %q", got, reason)
	}
}

func TestAnOwnDomainAndADepartedSeatCountAsMembersAndAnArchivedSeatDoesNot(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	sharedBox := sentToColleague(env, t, colleagueLogin, "sales@myco.example", "members-ownbox@myco.example")
	if _, reason := audienceOf(t, e, sharedBox); reason != activities.ReasonMembersOnly {
		t.Fatalf("mail copied to an address on the workspace's own domain reads %q, want members_only", reason)
	}

	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `UPDATE app_user SET status = 'deactivated' WHERE id = $1`, e.Rep3)
		return err
	}); err != nil {
		t.Fatalf("deactivating the colleague: %v", err)
	}
	departed := sentToColleague(env, t, colleagueLogin, "", "members-departed@myco.example")
	if _, reason := audienceOf(t, e, departed); reason != activities.ReasonMembersOnly {
		t.Fatalf("mail to a departed colleague's login reads %q, want members_only: it is still a company address", reason)
	}

	archiveSeat(t, e, e.Rep3)
	archived := sentToColleague(env, t, colleagueLogin, "", "members-archived@myco.example")
	if _, reason := audienceOf(t, e, archived); reason == activities.ReasonMembersOnly {
		t.Fatal("mail to an archived seat's address was held as mail between colleagues")
	}
}
