// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// Admin user administration over a real migrated Postgres: an invite creates an
// invited, passwordless member with the one target role and a single-use
// set-password token and emits user.invited; a reactivate returns a deactivated
// member to the status their credential justifies and emits user.reactivated.
// Both are admin-only.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The roster read gates the role-key lookup on WithRoles, and the wire mapping
// withholds the keys again for a non-admin. Those are two independent defences,
// so the HTTP deny arm passes even with this one forced open — which is exactly
// why the read needs its own test rather than borrowing that one's coverage.
func TestRosterReadFetchesRoleKeysOnlyWhenAsked(t *testing.T) {
	e := setupRevocationEnv(t, "roster-role-keys")

	withheld, _, err := e.svc.ListUsers(e.wsCtx(e.admin), ListUsersInput{})
	if err != nil {
		t.Fatalf("list without roles: %v", err)
	}
	if len(withheld) == 0 {
		t.Fatal("roster is empty; the assertions below would hold vacuously")
	}
	for _, u := range withheld {
		// NIL, not empty: an empty list would claim the member holds no role,
		// which is a different and false statement.
		if u.Roles != nil {
			t.Errorf("member %q carries roles %v on a read that did not ask for them", u.Email, u.Roles)
		}
	}

	asked, _, err := e.svc.ListUsers(e.wsCtx(e.admin), ListUsersInput{WithRoles: true})
	if err != nil {
		t.Fatalf("list with roles: %v", err)
	}
	var adminRow, memberRow *userRow
	for i := range asked {
		switch asked[i].ID {
		case e.admin.UserID.UUID:
			adminRow = &asked[i]
		case e.member.UserID.UUID:
			memberRow = &asked[i]
		}
	}
	if adminRow == nil || len(adminRow.Roles) != 1 || adminRow.Roles[0] != roleAdmin {
		t.Errorf("bootstrap admin roles = %v, want [admin] once the read asks", adminRow)
	}
	// The other arm of the same distinction: a seat holding NO role, read WITH
	// the flag, comes back empty-but-present. Nil here would be indistinguishable
	// from "never asked", which is what the COALESCE exists to prevent.
	if memberRow == nil {
		t.Fatal("the seeded member is missing from the roster")
	}
	if memberRow.Roles == nil {
		t.Error("a member holding no role read back nil, want an empty non-nil list")
	}
	if len(memberRow.Roles) != 0 {
		t.Errorf("seeded member roles = %v, want empty", memberRow.Roles)
	}
}

// The reactivate refusal enumerates the states it can be reached from, and its
// contract description does the same. Both are only true for a KNOWN status set:
// 'active' returns early as a no-op and 'deactivated' is the happy path, so
// every other value lands in that refusal and is described by it. A new status
// added without revisiting the copy would make both quietly wrong — this test
// exists because exactly that had already happened (0055 added 'invited' while
// the refusal still said "this member is suspended").
func TestReactivateRefusalNamesEveryStateItCanBeReachedFrom(t *testing.T) {
	e := setupRevocationEnv(t, "reactivate-states")

	var check string
	if err := e.owner.QueryRow(context.Background(), `
		SELECT pg_get_constraintdef(c.oid)
		FROM pg_constraint c JOIN pg_class t ON t.oid = c.conrelid
		WHERE t.relname = 'app_user' AND c.contype = 'c'
		  AND pg_get_constraintdef(c.oid) LIKE '%status%'`).Scan(&check); err != nil {
		t.Fatalf("reading the app_user status constraint: %v", err)
	}
	// The statuses the refusal and the contract account for. Compared as a SET
	// against the literals the constraint actually admits — counting quotes
	// would also count any cast, collation or quoted identifier Postgres puts in
	// its normalised text, and would miss a rename that kept the same count.
	known := map[string]bool{"invited": true, "active": true, "suspended": true, "deactivated": true}
	admitted := map[string]bool{}
	for _, m := range regexp.MustCompile(`'([^']*)'`).FindAllStringSubmatch(check, -1) {
		admitted[m[1]] = true
	}
	for status := range known {
		if !admitted[status] {
			t.Errorf("status %q is no longer admitted by %s", status, check)
		}
	}
	for status := range admitted {
		if !known[status] {
			t.Errorf("app_user.status admits %q, which nothing here accounts for: %s\n"+
				"the reactivate refusal names the states it can be reached from, and its "+
				"contract description repeats them — both need updating with the new state",
				status, check)
		}
	}
}

func TestInviteUserCreatesInvitedMemberWithRoleTokenAndEvent(t *testing.T) {
	e := setupRevocationEnv(t, "invite-user")

	userID, rawToken, err := e.svc.InviteUser(e.wsCtx(e.admin), e.admin, InviteUserInput{
		Email: "Newbie@Acme.test", DisplayName: "New Bie", Role: "rep",
	})
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	if userID.IsZero() || rawToken == "" {
		t.Fatalf("invite returned userID=%v token-empty=%v; want both set", userID, rawToken == "")
	}

	member, err := e.svc.GetUser(e.wsCtx(e.admin), userID)
	if err != nil {
		t.Fatalf("get invited member: %v", err)
	}
	// INVITED, not active: the row has no password and no linked identity, so
	// it signs in nowhere, and reporting it active would tell the roster that
	// somebody can enter who cannot.
	if member.Status != "invited" || member.Email != "newbie@acme.test" {
		t.Fatalf("invited member = %+v; want invited + lowercased email", member)
	}
	// The member read carries the assigned role keys — the admin card renders
	// the current role from them, so the aggregate must survive the round trip
	// and not merely be correct in the assignment table.
	if len(member.Roles) != 1 || member.Roles[0] != "rep" {
		t.Errorf("invited member Roles = %v, want [rep]", member.Roles)
	}

	var role string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT r.key FROM role_assignment ra JOIN role r ON r.id = ra.role_id WHERE ra.user_id = $1`,
		userID).Scan(&role); err != nil {
		t.Fatalf("role lookup: %v", err)
	}
	if role != "rep" {
		t.Errorf("invited member role = %q, want rep", role)
	}

	var liveTokens int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM auth_token WHERE user_id = $1 AND purpose = 'password_reset' AND used_at IS NULL`,
		userID).Scan(&liveTokens); err != nil {
		t.Fatal(err)
	}
	if liveTokens != 1 {
		t.Errorf("invite minted %d live set-password tokens, want exactly 1", liveTokens)
	}

	envs := e.identityEvents(t, "user.invited", userID.UUID)
	if len(envs) != 1 {
		t.Fatalf("user.invited staged %d times, want once", len(envs))
	}
	var payload struct {
		UserID ids.UserID `json:"user_id"`
		Role   string     `json:"role"`
		By     ids.UserID `json:"by"`
	}
	if err := json.Unmarshal(envs[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.UserID != userID || payload.Role != "rep" || payload.By != e.admin.UserID {
		t.Errorf("user.invited payload = %+v, want {invited id, rep, admin}", payload)
	}
	if envs[0].Trace.AuditLogID.IsZero() {
		t.Error("user.invited carries no audit_log_id — the write shape demands the linked audit row")
	}

	// A duplicate email refuses; an unknown role is a 404; a non-admin cannot invite.
	if _, _, err := e.svc.InviteUser(e.wsCtx(e.admin), e.admin, InviteUserInput{
		Email: "newbie@acme.test", DisplayName: "Dupe", Role: "rep",
	}); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("duplicate-email invite: err = %v, want conflict", err)
	}
	if _, _, err := e.svc.InviteUser(e.wsCtx(e.admin), e.admin, InviteUserInput{
		Email: "other@acme.test", DisplayName: "X", Role: "no-such-role",
	}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("unknown-role invite: err = %v, want not found", err)
	}
	if _, _, err := e.svc.InviteUser(e.wsCtx(e.member), e.member, InviteUserInput{
		Email: "sneaky@acme.test", DisplayName: "X", Role: "rep",
	}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("non-admin invite: err = %v, want permission denied", err)
	}
}

// TestInviteUserHandlerEmailsTheSetPasswordLink drives InviteUser through the
// HTTP handler rather than the service directly, because sendInvite — and the
// passwordLink call inside it — only runs on that path: the service layer
// mints the token but never sends the mail.
func TestInviteUserHandlerEmailsTheSetPasswordLink(t *testing.T) {
	e := setupRevocationEnv(t, "invite-mail")
	mail := &capturedMail{}
	h := NewHandlers(e.svc).WithPasswordReset(mail).WithPasswordLinkBase("https://crm.example.test")

	ctx := withIdentity(e.wsCtx(e.admin), e.admin)
	body := `{"email":"invited@acme.test","display_name":"Invited Rep","role":"rep"}`
	rec := httptest.NewRecorder()
	h.InviteUser(rec, httptest.NewRequest(http.MethodPost, "/v1/users", strings.NewReader(body)).WithContext(ctx))
	if rec.Code != http.StatusCreated {
		t.Fatalf("invite status = %d, want 201: %s", rec.Code, rec.Body)
	}

	if mail.sent != 1 || mail.to != "invited@acme.test" {
		t.Fatalf("mail = %+v, want one message to the invited address", mail)
	}
	// Same shape as the reset link: the token rides the fragment, never the
	// server-visible query — passwordLink is the single builder for both.
	serverVisible, fragment, found := strings.Cut(mail.body, "#")
	if !found || strings.Contains(serverVisible, "token=") {
		t.Fatalf("invite link puts the token in the server-visible query: %q", mail.body)
	}
	if !strings.Contains(fragment, "/reset-password?token=") {
		t.Fatalf("invite mail carries no set-password link: %q", mail.body)
	}
}

func TestReactivateUserRestoresActiveAndEmits(t *testing.T) {
	e := setupRevocationEnv(t, "reactivate-user")

	if err := e.svc.DeactivateUser(e.wsCtx(e.admin), e.admin, DeactivateUserInput{UserID: e.member.UserID}); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	if err := e.svc.ReactivateUser(e.wsCtx(e.admin), e.admin, e.member.UserID); err != nil {
		t.Fatalf("reactivate: %v", err)
	}

	member, err := e.svc.GetUser(e.wsCtx(e.admin), e.member.UserID)
	if err != nil {
		t.Fatalf("get reactivated member: %v", err)
	}
	if member.Status != "active" {
		t.Errorf("reactivated member status = %q, want active", member.Status)
	}

	envs := e.identityEvents(t, "user.reactivated", e.member.UserID.UUID)
	if len(envs) != 1 {
		t.Fatalf("user.reactivated staged %d times, want once", len(envs))
	}

	// Idempotent on an already-active member: no error, no duplicate event.
	if err := e.svc.ReactivateUser(e.wsCtx(e.admin), e.admin, e.member.UserID); err != nil {
		t.Fatalf("repeat reactivate: %v", err)
	}
	if again := e.identityEvents(t, "user.reactivated", e.member.UserID.UUID); len(again) != 1 {
		t.Errorf("repeat reactivation staged a duplicate event (%d total)", len(again))
	}

	// An unknown member is a 404; a non-admin cannot reactivate.
	if err := e.svc.ReactivateUser(e.wsCtx(e.admin), e.admin, ids.UserID{UUID: ids.NewV7()}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("reactivate unknown: err = %v, want not found", err)
	}
	if err := e.svc.ReactivateUser(e.wsCtx(e.member), e.member, e.admin.UserID); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("non-admin reactivate: err = %v, want permission denied", err)
	}
}

// TestConcurrentLastAdminDeactivationsKeepOneAdmin proves the admin-guard is
// race-safe: with two admins, two transactions each deactivating a DIFFERENT
// one must not both succeed (that would zero out admins). The per-workspace
// advisory lock serializes them, so exactly one is refused.
func TestConcurrentLastAdminDeactivationsKeepOneAdmin(t *testing.T) {
	e := setupRevocationEnv(t, "admin-race")

	// Promote the member to admin — now the workspace has exactly two admins.
	if err := e.svc.ChangeUserRole(e.wsCtx(e.admin), e.admin, e.member.UserID, "admin"); err != nil {
		t.Fatalf("promote member to admin: %v", err)
	}

	targets := []ids.UserID{e.admin.UserID, e.member.UserID}
	errs := make([]error, len(targets))
	var wg sync.WaitGroup
	for i, target := range targets {
		wg.Add(1)
		go func(i int, target ids.UserID) {
			defer wg.Done()
			errs[i] = e.svc.DeactivateUser(e.wsCtx(e.admin), e.admin, DeactivateUserInput{UserID: target})
		}(i, target)
	}
	wg.Wait()

	conflicts := 0
	for _, err := range errs {
		switch {
		case err == nil:
		case errors.Is(err, apperrors.ErrConflict):
			conflicts++
		default:
			t.Fatalf("unexpected deactivation error: %v", err)
		}
	}
	if conflicts != 1 {
		t.Fatalf("concurrent double-admin deactivation refused %d times, want exactly 1 — the race would zero out admins", conflicts)
	}
}

// agentSeatOf answers the workspace's agent seat, the one bootstrap wrote.
func agentSeatOf(t *testing.T, e *revocationEnv) ids.UserID {
	t.Helper()
	var seat ids.UserID
	if err := e.owner.QueryRow(context.Background(),
		`SELECT id FROM app_user WHERE is_agent`).Scan(&seat); err != nil {
		t.Fatalf("reading the workspace's agent seat: %v", err)
	}
	return seat
}

func TestTheAgentSeatCannotBeGivenARole(t *testing.T) {
	e := setupRevocationEnv(t, "agent-role")
	seat := agentSeatOf(t, e)

	err := e.svc.ChangeUserRole(e.wsCtx(e.admin), e.admin, seat, "admin")
	if !errors.Is(err, errAgentSeatHoldsNoRole) {
		t.Fatalf("granting the agent seat a role returned %v, want the agent-seat refusal. What an "+
			"agent may do is the passport granting it intersected with the human that passport names, "+
			"so a role on its own row is authority nothing bounds", err)
	}
	var grants int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM role_assignment WHERE user_id = $1`, seat).Scan(&grants); err != nil {
		t.Fatalf("counting the seat's role assignments: %v", err)
	}
	if grants != 0 {
		t.Errorf("the refused grant left %d role assignment(s) on the agent seat", grants)
	}
}

// The lockout the seat could otherwise cause, and the reason the admin count
// carries an exclusion of its own rather than resting on the refusal above.
//
// The role assignment here is written directly, because the product refuses to
// create it — which is the point. An installation that hand-inserted its agent
// seat while no product path existed can hold exactly this row already, and a
// refusal added afterwards reaches no grant that has already happened.
func TestTheAgentSeatIsNotTheOtherAdminWhoCouldRecoverTheOrganization(t *testing.T) {
	e := setupRevocationEnv(t, "agent-lockout")
	seat := agentSeatOf(t, e)
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO role_assignment (role_id, user_id)
		 SELECT r.id, $1 FROM role r WHERE r.key = 'admin'`,
		seat); err != nil {
		t.Fatalf("granting the agent seat the admin role directly: %v", err)
	}

	err := e.svc.DeactivateUser(e.wsCtx(e.admin), e.admin, DeactivateUserInput{UserID: e.admin.UserID})
	if !errors.Is(err, errLastActiveAdmin) {
		t.Fatalf("deactivating the only human administrator returned %v, want the last-admin refusal. "+
			"The agent seat administers nothing — it carries no password and signs in nowhere — so "+
			"counting it leaves the organization with no way back into user administration at all", err)
	}
}

// An invitation is a seat that cannot yet be entered, and redeeming its link is
// what makes it enterable. The transition is the thing under test: the status
// moves, and — unlike a password reset, which changes a credential and no domain
// state — it carries the audit row and the event every other status change in
// this module commits.
func TestRedeemingAnInvitationActivatesTheMemberAndEmitsIt(t *testing.T) {
	e := setupRevocationEnv(t, "invite-activation")

	userID, rawToken, err := e.svc.InviteUser(e.wsCtx(e.admin), e.admin, InviteUserInput{
		Email: "pending@acme.test", DisplayName: "Pending Person", Role: "rep",
	})
	if err != nil {
		t.Fatalf("invite: %v", err)
	}

	// Before redemption the seat is charged and the member signs in nowhere.
	if _, _, err := e.svc.Login(e.wsCtx(e.admin), "pending@acme.test", "whatever-they-guess"); err == nil {
		t.Fatal("an invited member signed in with a password they never set")
	}
	var seatsBefore int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM app_user WHERE seat_type = 'full' AND status NOT IN ('suspended', 'deactivated')`).
		Scan(&seatsBefore); err != nil {
		t.Fatal(err)
	}

	if err := e.svc.RedeemPasswordReset(e.wsCtx(e.admin), rawToken, "a-password-they-chose-1"); err != nil {
		t.Fatalf("redeem invitation: %v", err)
	}

	member, err := e.svc.GetUser(e.wsCtx(e.admin), userID)
	if err != nil {
		t.Fatalf("get member: %v", err)
	}
	if member.Status != "active" {
		t.Fatalf("after redemption status = %q, want active", member.Status)
	}
	if _, _, err := e.svc.Login(e.wsCtx(e.admin), "pending@acme.test", "a-password-they-chose-1"); err != nil {
		t.Fatalf("activated member cannot sign in: %v", err)
	}

	// The seat count does not move: an invitation already held it.
	var seatsAfter int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM app_user WHERE seat_type = 'full' AND status NOT IN ('suspended', 'deactivated')`).
		Scan(&seatsAfter); err != nil {
		t.Fatal(err)
	}
	if seatsAfter != seatsBefore {
		t.Errorf("licensed seats in use moved %d -> %d across an activation; an invitation already holds its seat", seatsBefore, seatsAfter)
	}

	envs := e.identityEvents(t, "user.activated", userID.UUID)
	if len(envs) != 1 {
		t.Fatalf("user.activated staged %d times, want once", len(envs))
	}
	if envs[0].Trace.AuditLogID.IsZero() {
		t.Error("user.activated carries no audit_log_id — the write shape demands the linked audit row")
	}
}

// The recovery path for an invitation nobody redeemed in time. Without it the
// account is unenterable by any route: it has no password, so the self-service
// reset refuses it, and the admin-issued link is the only way back.
func TestAnExpiredInvitationIsRecoveredByAnAdminIssuedLink(t *testing.T) {
	e := setupRevocationEnv(t, "invite-expired-recovery")

	userID, _, err := e.svc.InviteUser(e.wsCtx(e.admin), e.admin, InviteUserInput{
		Email: "stale@acme.test", DisplayName: "Stale Invite", Role: "rep",
	})
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE auth_token SET expires_at = now() - interval '1 day' WHERE user_id = $1`, userID); err != nil {
		t.Fatal(err)
	}

	token, _, err := e.svc.IssuePasswordLink(e.wsCtx(e.admin), e.admin, userID)
	if err != nil {
		t.Fatalf("an admin cannot re-issue a set-password link to an invited member, so the account is stranded: %v", err)
	}
	if err := e.svc.RedeemPasswordReset(e.wsCtx(e.admin), token, "recovered-password-9"); err != nil {
		t.Fatalf("redeem re-issued link: %v", err)
	}
	member, err := e.svc.GetUser(e.wsCtx(e.admin), userID)
	if err != nil {
		t.Fatal(err)
	}
	if member.Status != "active" {
		t.Errorf("after recovery status = %q, want active", member.Status)
	}
}

// InviteUser puts a member on teams at invite time, so an admin correcting that
// choice before redemption must not be refused.
func TestAnInvitedMemberJoinsATeam(t *testing.T) {
	e := setupRevocationEnv(t, "invite-team-join")

	userID, _, err := e.svc.InviteUser(e.wsCtx(e.admin), e.admin, InviteUserInput{
		Email: "teamless@acme.test", DisplayName: "Teamless", Role: "rep",
	})
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	team, err := e.svc.CreateTeam(e.wsCtx(e.admin), e.admin, "Late Additions")
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	if err := e.svc.SetTeamMember(e.wsCtx(e.admin), e.admin, team.ID, userID.UUID, true); err != nil {
		t.Fatalf("an admin cannot fix the teams on an unredeemed invitation: %v", err)
	}
}

// Reactivating someone who never set a password returns them to INVITED. Sending
// them to active would restate the falsehood this status exists to remove: the
// row still has no credential and still signs in nowhere.
func TestReactivatingAMemberWhoNeverSetAPasswordReturnsThemToInvited(t *testing.T) {
	e := setupRevocationEnv(t, "reactivate-unredeemed")

	userID, _, err := e.svc.InviteUser(e.wsCtx(e.admin), e.admin, InviteUserInput{
		Email: "revoked@acme.test", DisplayName: "Revoked Invite", Role: "rep",
	})
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	if err := e.svc.DeactivateUser(e.wsCtx(e.admin), e.admin, DeactivateUserInput{UserID: userID}); err != nil {
		t.Fatalf("deactivate an unredeemed invitation: %v", err)
	}
	if err := e.svc.ReactivateUser(e.wsCtx(e.admin), e.admin, userID); err != nil {
		t.Fatalf("reactivate: %v", err)
	}
	member, err := e.svc.GetUser(e.wsCtx(e.admin), userID)
	if err != nil {
		t.Fatal(err)
	}
	if member.Status != "invited" {
		t.Errorf("reactivated status = %q, want invited — this member has no password and can still sign in nowhere", member.Status)
	}
}

// An invited administrator cannot sign in, so they must not satisfy the guard
// that keeps an installation enterable. Before invitations carried their own
// status this was a real lockout: the row was written active, counted as
// another administrator, and let an operator deactivate the last one who could
// actually enter — with no way back in.
func TestAnInvitedAdminIsNotAnotherActiveAdmin(t *testing.T) {
	e := setupRevocationEnv(t, "invited-admin-not-a-guard")

	if _, _, err := e.svc.InviteUser(e.wsCtx(e.admin), e.admin, InviteUserInput{
		Email: "second-admin@acme.test", DisplayName: "Second Admin", Role: "admin",
	}); err != nil {
		t.Fatalf("invite an admin: %v", err)
	}

	err := e.svc.DeactivateUser(e.wsCtx(e.admin), e.admin, DeactivateUserInput{UserID: e.admin.UserID})
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("deactivating the only admin who can sign in: err = %v, want a conflict — "+
			"an unredeemed invitation is not somebody who can administer the installation", err)
	}

	// And once that invitation is redeemed, the guard is genuinely satisfied:
	// the refusal above must come from the status, not from a rule that never
	// lets the last admin step down.
	_, rawToken, err := e.svc.InviteUser(e.wsCtx(e.admin), e.admin, InviteUserInput{
		Email: "third-admin@acme.test", DisplayName: "Third Admin", Role: "admin",
	})
	if err != nil {
		t.Fatalf("invite a second admin: %v", err)
	}
	if err := e.svc.RedeemPasswordReset(
		principal.WithCorrelationID(e.wsCtx(e.admin), ids.NewV7()), rawToken, "an-admin-password-2"); err != nil {
		t.Fatalf("redeem: %v", err)
	}
	if err := e.svc.DeactivateUser(e.wsCtx(e.admin), e.admin, DeactivateUserInput{UserID: e.admin.UserID}); err != nil {
		t.Errorf("with a redeemed second admin the first may stand down: %v", err)
	}
}
