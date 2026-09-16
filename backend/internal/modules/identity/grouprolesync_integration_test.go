// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// The additive group→role grants at federated sign-in (grouprolesync.go),
// proven over a real migrated Postgres on seedSSOEnv's harness: a mapped group
// grants its role, a second sign-in grants nothing again, leaving the group
// revokes nothing, and the no-account-creation root rule survives a token
// carrying groups. The map reader is injected per test the way compose injects
// it, so each case states the policy it runs under.

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// groupSyncCtx carries the correlation scope the HTTP chassis stamps on every
// request: a grant stages a role.changed event, and storekit.Emit refuses to
// stage one with no scope bound.
func groupSyncCtx() context.Context {
	return principal.WithCorrelationID(context.Background(), ids.NewV7())
}

func heldRoleKeys(t *testing.T, conn *pgx.Conn, userID ids.UserID) []string {
	t.Helper()
	rows, err := conn.Query(context.Background(),
		`SELECT r.key FROM role_assignment ra JOIN role r ON r.id = ra.role_id WHERE ra.user_id = $1 ORDER BY r.key`,
		userID)
	if err != nil {
		t.Fatalf("reading held roles: %v", err)
	}
	keys, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("reading held roles: %v", err)
	}
	return keys
}

func roleAssignAuditCount(t *testing.T, conn *pgx.Conn, userID ids.UserID) int {
	t.Helper()
	var n int
	if err := conn.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE action = 'assign' AND entity_type = 'user' AND entity_id = $1`,
		userID.UUID).Scan(&n); err != nil {
		t.Fatalf("counting assign audit rows: %v", err)
	}
	return n
}

func TestFederatedSignInGrantsTheMappedRoleOnce(t *testing.T) {
	svc, conn, userID, email := seedSSOEnv(t, "sso-group-grant")
	svc.WithGroupRoleMap(func(context.Context, pgx.Tx) (map[string]string, error) {
		return map[string]string{"crm-users": "rep"}, nil
	})

	if _, err := svc.LoginViaFederatedIdentity(groupSyncCtx(), "google", "sub-1", email,
		[]string{"crm-users", "some-unmapped-group"}); err != nil {
		t.Fatalf("LoginViaFederatedIdentity: %v", err)
	}
	if got := heldRoleKeys(t, conn, userID); !slices.Equal(got, []string{"rep"}) {
		t.Fatalf("held roles = %v, want the mapped role and only it", got)
	}
	if n := roleAssignAuditCount(t, conn, userID); n != 1 {
		t.Fatalf("assign audit rows = %d, want exactly one for the grant", n)
	}
	var events int
	if err := conn.QueryRow(context.Background(),
		`SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'role.changed'
		 AND envelope->'entity'->>'id' = $1`, userID.UUID.String()).Scan(&events); err != nil {
		t.Fatalf("counting role.changed events: %v", err)
	}
	if events != 1 {
		t.Fatalf("role.changed events = %d, want one so permission caches learn of the grant", events)
	}

	// The SECOND sign-in changes nothing: the role is already held, so no new
	// assignment, no second audit row — an unchanged login stays a login.
	if _, err := svc.LoginViaFederatedIdentity(groupSyncCtx(), "google", "sub-1", email,
		[]string{"crm-users"}); err != nil {
		t.Fatalf("second LoginViaFederatedIdentity: %v", err)
	}
	if got := heldRoleKeys(t, conn, userID); !slices.Equal(got, []string{"rep"}) {
		t.Fatalf("held roles after re-login = %v, want unchanged", got)
	}
	if n := roleAssignAuditCount(t, conn, userID); n != 1 {
		t.Fatalf("assign audit rows after re-login = %d, want still one", n)
	}
}

// The decision's additive semantics, stated as the test name: a member whose
// group left the map KEEPS the role. Removing a member from an IdP group does
// not revoke anything here — revocation stays a deliberate admin action.
func TestAMemberKeepsARoleWhoseGroupLeftTheMap(t *testing.T) {
	svc, conn, userID, email := seedSSOEnv(t, "sso-group-keeps")
	roleMap := map[string]string{"crm-users": "rep"}
	svc.WithGroupRoleMap(func(context.Context, pgx.Tx) (map[string]string, error) { return roleMap, nil })

	if _, err := svc.LoginViaFederatedIdentity(groupSyncCtx(), "google", "sub-1", email,
		[]string{"crm-users"}); err != nil {
		t.Fatalf("first LoginViaFederatedIdentity: %v", err)
	}

	// The admin retires the mapping AND the directory drops the member from
	// the group — the strongest revocation the IdP side can express.
	roleMap = map[string]string{}
	if _, err := svc.LoginViaFederatedIdentity(groupSyncCtx(), "google", "sub-1", email, nil); err != nil {
		t.Fatalf("second LoginViaFederatedIdentity: %v", err)
	}
	if got := heldRoleKeys(t, conn, userID); !slices.Equal(got, []string{"rep"}) {
		t.Fatalf("held roles = %v, want the role KEPT — the map grants and never revokes", got)
	}
}

func TestAnUnmappedGroupGrantsNothing(t *testing.T) {
	svc, conn, userID, email := seedSSOEnv(t, "sso-group-unmapped")
	svc.WithGroupRoleMap(func(context.Context, pgx.Tx) (map[string]string, error) {
		return map[string]string{"crm-users": "rep"}, nil
	})

	if _, err := svc.LoginViaFederatedIdentity(groupSyncCtx(), "google", "sub-1", email,
		[]string{"another-group-entirely"}); err != nil {
		t.Fatalf("LoginViaFederatedIdentity: %v", err)
	}
	if got := heldRoleKeys(t, conn, userID); len(got) != 0 {
		t.Fatalf("held roles = %v, want none for a token naming no mapped group", got)
	}
	if n := roleAssignAuditCount(t, conn, userID); n != 0 {
		t.Fatalf("assign audit rows = %d, want none when nothing was granted", n)
	}
}

// A mapped role key the installation no longer defines must not fail the
// login: the member did nothing wrong, so the stale entry is skipped (and
// logged) while the sign-in completes. The validator refuses storing such a
// key, so this state only arises when a role is retired after the map was
// saved — which is why the map is injected raw here.
func TestAStaleMapEntryIsSkippedAndTheSignInSucceeds(t *testing.T) {
	svc, conn, userID, email := seedSSOEnv(t, "sso-group-stale")
	svc.WithGroupRoleMap(func(context.Context, pgx.Tx) (map[string]string, error) {
		return map[string]string{"crm-users": "a_role_since_retired"}, nil
	})

	if _, err := svc.LoginViaFederatedIdentity(groupSyncCtx(), "google", "sub-1", email,
		[]string{"crm-users"}); err != nil {
		t.Fatalf("a stale map entry failed the login: %v", err)
	}
	if got := heldRoleKeys(t, conn, userID); len(got) != 0 {
		t.Fatalf("held roles = %v, want none from a role key nothing defines", got)
	}
}

// The root no-JIT rule survives the feature it most tempts: a token carrying a
// mapped group for an email nobody invited is refused exactly as before, and
// no account exists afterwards.
func TestGroupsOnANeverInvitedEmailStillRefuse(t *testing.T) {
	svc, conn, _, _ := seedSSOEnv(t, "sso-group-no-jit")
	svc.WithGroupRoleMap(func(context.Context, pgx.Tx) (map[string]string, error) {
		return map[string]string{"crm-admins": "admin"}, nil
	})

	strangerEmail := "stranger@example.com"
	_, err := svc.LoginViaFederatedIdentity(groupSyncCtx(), "google", "sub-stranger", strangerEmail,
		[]string{"crm-admins"})
	if !errors.Is(err, ErrFederatedSignInRefused) {
		t.Fatalf("err = %v, want ErrFederatedSignInRefused — the map grants roles, never accounts", err)
	}
	var n int
	if err := conn.QueryRow(context.Background(),
		`SELECT count(*) FROM app_user WHERE lower(email) = lower($1)`, strangerEmail).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("app_user rows for the refused email = %d, want none", n)
	}
}

// A groupless token never reads the map at all — the common sign-in pays
// nothing for this feature, and even a broken map reader cannot touch it.
func TestAGrouplessTokenNeverReadsTheMap(t *testing.T) {
	svc, _, _, email := seedSSOEnv(t, "sso-group-none")
	svc.WithGroupRoleMap(func(context.Context, pgx.Tx) (map[string]string, error) {
		t.Error("the group-role map was read for a token that carried no groups")
		return map[string]string{}, nil
	})

	if _, err := svc.LoginViaFederatedIdentity(groupSyncCtx(), "google", "sub-1", email, nil); err != nil {
		t.Fatalf("LoginViaFederatedIdentity: %v", err)
	}
}
