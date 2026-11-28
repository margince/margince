// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// The Google directory connection over a real migrated Postgres, with a fake
// Directory API. The groups it answers feed the grant path the claim uses. A
// failed lookup signs the member in with nothing new. The credential stays
// sealed and admin-only.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

const (
	directoryGroup  = "crm-users@example.test"
	workspaceDomain = "example.test"
)

// googleClaims is a Google token for a Workspace member: the domain that
// vouches for it, and the subject and address signIn fills in.
func googleClaims() OIDCClaims { return OIDCClaims{HostedDomain: workspaceDomain} }

// memorySealer stands in for the key vault. It keeps what was sealed under a
// ref and forgets it when the setting retires the ref.
type memorySealer struct {
	mu     sync.Mutex
	next   int
	sealed map[string]string
}

func (m *memorySealer) sealer() config.Sealer {
	return config.Sealer{
		Seal: func(_ context.Context, plaintext string) (string, error) {
			m.mu.Lock()
			defer m.mu.Unlock()
			m.next++
			ref := fmt.Sprintf("ref-%d", m.next)
			m.sealed[ref] = plaintext
			return ref, nil
		},
		Resolve: func(_ context.Context, ref string) (string, error) {
			m.mu.Lock()
			defer m.mu.Unlock()
			if v, ok := m.sealed[ref]; ok {
				return v, nil
			}
			return "", errors.New("no such ref")
		},
		Retire: func(_ context.Context, ref string) {
			m.mu.Lock()
			defer m.mu.Unlock()
			delete(m.sealed, ref)
		},
	}
}

func (m *memorySealer) held() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sealed)
}

type directoryEnv struct {
	*revocationEnv
	google *fakeGoogle
	vault  *memorySealer
	cred   directoryCredential
}

// setupDirectoryEnv wires the lookup the way compose does, with the map
// granting `rep` to the group the fake Directory API answers with.
func setupDirectoryEnv(t *testing.T, slug string) *directoryEnv {
	t.Helper()
	e := &directoryEnv{revocationEnv: setupRevocationEnv(t, slug)}
	e.google = newFakeGoogle(t, []string{directoryGroup})
	e.vault = &memorySealer{sealed: map[string]string{}}
	e.cred = testDirectoryCredential(t)
	e.google.known = []string{e.cred.AdminSubject, "sub-" + e.slug}
	e.svc.WithGoogleDirectory(unlockedConfig(t, e.pool).WithSealer(e.vault.sealer()))
	e.svc.directory.reader = e.google.reader(t)
	e.svc.WithGroupRoleMap(func(context.Context, pgx.Tx) (map[string]string, error) {
		return map[string]string{directoryGroup: "rep"}, nil
	})
	return e
}

func (e *directoryEnv) storeCredential(t *testing.T) {
	t.Helper()
	if _, err := e.svc.SetGoogleDirectory(e.wsCtx(e.admin), DirectoryInput{
		ServiceAccountJSON: e.cred.ServiceAccountKey, AdminSubject: e.cred.AdminSubject,
	}); err != nil {
		t.Fatalf("storing the credential: %v", err)
	}
}

func (e *directoryEnv) signIn(t *testing.T, provider string, claims OIDCClaims) (string, error) {
	t.Helper()
	if claims.Email == "" {
		claims.Email = e.member.Email
	}
	if claims.Subject == "" {
		claims.Subject = "sub-" + e.slug
	}
	return e.svc.LoginViaFederatedIdentity(groupSyncCtx(), provider, claims)
}

func (e *directoryEnv) directoryLog(t *testing.T) []string {
	t.Helper()
	rows, err := e.owner.Query(context.Background(),
		`SELECT detail->>'outcome' || ':' || (detail->>'reason') FROM system_log WHERE action = $1 ORDER BY occurred_at, id`,
		directoryLogAction)
	if err != nil {
		t.Fatal(err)
	}
	out, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestAGoogleMemberInAMappedGroupHoldsTheMappedRole(t *testing.T) {
	e := setupDirectoryEnv(t, "gdir-grant")
	e.storeCredential(t)
	e.google.userKeys = nil

	token, err := e.signIn(t, "google", googleClaims())
	if err != nil || token == "" {
		t.Fatalf("sign-in: token=%q err=%v", token, err)
	}
	if got := heldRoleKeys(t, e.owner, e.member.UserID); len(got) != 1 || got[0] != "rep" {
		t.Fatalf("held roles = %v, want the mapped role", got)
	}
	if n := roleAssignAuditCount(t, e.owner, e.member.UserID); n != 1 {
		t.Fatalf("assign audit rows = %d, want the one row the claim path writes", n)
	}
	if len(e.google.userKeys) != 1 || e.google.userKeys[0] != "sub-"+e.slug {
		t.Fatalf("directory asked for %v, want the signed-in member's subject once", e.google.userKeys)
	}
}

func TestAGoogleSignInWithoutACredentialMakesNoLookupAndGrantsNothing(t *testing.T) {
	e := setupDirectoryEnv(t, "gdir-nocred")

	if _, err := e.signIn(t, "google", googleClaims()); err != nil {
		t.Fatalf("sign-in: %v", err)
	}
	if len(e.google.userKeys) != 0 || len(e.google.assertions) != 0 {
		t.Fatalf("google was called without a credential: %v", e.google.userKeys)
	}
	if got := heldRoleKeys(t, e.owner, e.member.UserID); len(got) != 0 {
		t.Fatalf("held roles = %v, want none", got)
	}
}

func TestAGoogleSignInWithAnEmptyMapMakesNoLookup(t *testing.T) {
	e := setupDirectoryEnv(t, "gdir-nomap")
	e.storeCredential(t)
	e.google.userKeys = nil
	e.svc.WithGroupRoleMap(func(context.Context, pgx.Tx) (map[string]string, error) { return map[string]string{}, nil })

	if _, err := e.signIn(t, "google", googleClaims()); err != nil {
		t.Fatalf("sign-in: %v", err)
	}
	if len(e.google.userKeys) != 0 {
		t.Fatalf("a lookup nothing could use was made: %v", e.google.userKeys)
	}
}

func TestAFailedLookupSignsTheMemberInWithNothingNewAndTakesNothingAway(t *testing.T) {
	e := setupDirectoryEnv(t, "gdir-fail")
	e.storeCredential(t)
	// A role the member already holds from an earlier sign-in whose token carried it.
	if _, err := e.signIn(t, "google", OIDCClaims{HostedDomain: workspaceDomain, Groups: []string{directoryGroup}}); err != nil {
		t.Fatalf("first sign-in: %v", err)
	}
	if _, err := e.owner.Exec(context.Background(),
		`DELETE FROM role_assignment WHERE user_id = $1`, e.member.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO role_assignment (role_id, user_id) SELECT id, $1 FROM role WHERE key = 'ops'`, e.member.UserID); err != nil {
		t.Fatal(err)
	}
	e.google.groupsStatus = http.StatusServiceUnavailable

	token, err := e.signIn(t, "google", googleClaims())
	if err != nil || token == "" {
		t.Fatalf("a failed lookup must not fail the sign-in: token=%q err=%v", token, err)
	}
	if got := heldRoleKeys(t, e.owner, e.member.UserID); len(got) != 1 || got[0] != "ops" {
		t.Fatalf("held roles = %v, want only what the member held before", got)
	}
	if rows := e.directoryLog(t); len(rows) != 2 || rows[1] != "failed:unavailable" {
		t.Fatalf("directory log = %v, want the stored-credential check then one failure with its reason class", rows)
	}
	status, err := e.svc.GoogleDirectoryStatus(e.wsCtx(e.admin))
	if err != nil || !status.Configured || status.LastResult != directoryFailed || status.LastReason != "unavailable" {
		t.Fatalf("status = %+v err=%v, want the failure and its reason", status, err)
	}
}

func TestARecoveredLookupWritesOneRowAndABusyDirectoryWritesNoMore(t *testing.T) {
	e := setupDirectoryEnv(t, "gdir-recover")
	e.storeCredential(t)
	e.google.groupsStatus = http.StatusForbidden
	if _, err := e.signIn(t, "google", googleClaims()); err != nil {
		t.Fatal(err)
	}
	e.google.groupsStatus = http.StatusOK
	for range 3 {
		if _, err := e.signIn(t, "google", googleClaims()); err != nil {
			t.Fatal(err)
		}
	}
	rows := e.directoryLog(t)
	if want := []string{"ok:", "failed:permission_denied", "ok:"}; strings.Join(rows, ",") != strings.Join(want, ",") {
		t.Fatalf("directory log = %v, want %v", rows, want)
	}
}

func TestAMicrosoftSignInNeverCallsTheGoogleDirectory(t *testing.T) {
	e := setupDirectoryEnv(t, "gdir-microsoft")
	e.storeCredential(t)
	e.google.userKeys = nil

	if _, err := e.signIn(t, "microsoft", OIDCClaims{}); err != nil {
		t.Fatalf("sign-in: %v", err)
	}
	if len(e.google.userKeys) != 0 {
		t.Fatalf("a Microsoft sign-in asked the Google directory: %v", e.google.userKeys)
	}
}

func TestATokenThatCarriesGroupsWinsAndNoLookupRuns(t *testing.T) {
	e := setupDirectoryEnv(t, "gdir-claim")
	e.storeCredential(t)
	e.google.userKeys = nil

	if _, err := e.signIn(t, "google", OIDCClaims{HostedDomain: workspaceDomain, Groups: []string{directoryGroup}}); err != nil {
		t.Fatalf("sign-in: %v", err)
	}
	if len(e.google.userKeys) != 0 {
		t.Fatalf("a lookup ran although the token carried groups: %v", e.google.userKeys)
	}
	if got := heldRoleKeys(t, e.owner, e.member.UserID); len(got) != 1 || got[0] != "rep" {
		t.Fatalf("held roles = %v, want the claim's grant", got)
	}
}

func TestAnUninvitedGoogleAddressIsRefusedBeforeAnyLookup(t *testing.T) {
	e := setupDirectoryEnv(t, "gdir-uninvited")
	e.storeCredential(t)
	e.google.userKeys = nil

	_, err := e.svc.LoginViaFederatedIdentity(groupSyncCtx(), "google",
		OIDCClaims{Subject: "stranger", Email: "stranger@" + e.slug + ".test"})
	if !errors.Is(err, ErrFederatedSignInRefused) {
		t.Fatalf("err = %v, want the neutral refusal", err)
	}
	if len(e.google.userKeys) != 0 {
		t.Fatalf("an address nobody invited cost a directory lookup: %v", e.google.userKeys)
	}
}

func TestOnlyAnAdminReadsStoresOrRemovesTheCredential(t *testing.T) {
	e := setupDirectoryEnv(t, "gdir-gate")
	ctx := e.wsCtx(e.member)

	if _, err := e.svc.GoogleDirectoryStatus(ctx); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("status err = %v, want permission denied", err)
	}
	_, err := e.svc.SetGoogleDirectory(ctx, DirectoryInput{ServiceAccountJSON: e.cred.ServiceAccountKey, AdminSubject: e.cred.AdminSubject})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("set err = %v, want permission denied", err)
	}
	if err := e.svc.RemoveGoogleDirectory(ctx); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("remove err = %v, want permission denied", err)
	}
	if e.vault.held() != 0 || len(e.google.assertions) != 0 {
		t.Fatal("a refused caller reached the vault or Google")
	}
}

func TestReplacingTheKeyDestroysTheOldSealedCopyAndTheStatusNeverHoldsTheKey(t *testing.T) {
	e := setupDirectoryEnv(t, "gdir-rotate")
	e.storeCredential(t)
	e.cred = testDirectoryCredential(t)
	e.storeCredential(t)

	if e.vault.held() != 1 {
		t.Fatalf("sealed copies = %d, want exactly the current one", e.vault.held())
	}
	status, err := e.svc.GoogleDirectoryStatus(e.wsCtx(e.admin))
	if err != nil || !status.Configured || status.LastResult != directoryOK {
		t.Fatalf("status = %+v err=%v", status, err)
	}
	wire, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(wire), "PRIVATE KEY") || strings.Contains(string(wire), e.cred.AdminSubject) {
		t.Fatalf("the status carries credential material: %s", wire)
	}

	if err := e.svc.RemoveGoogleDirectory(e.wsCtx(e.admin)); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if e.vault.held() != 0 {
		t.Fatalf("sealed copies after removal = %d, want none", e.vault.held())
	}
	if status, _ := e.svc.GoogleDirectoryStatus(e.wsCtx(e.admin)); status.Configured {
		t.Fatal("the credential still reads as configured after removal")
	}
}

func TestACredentialGoogleRefusesIsNeverSealed(t *testing.T) {
	e := setupDirectoryEnv(t, "gdir-refused")
	e.google.tokenStatus = http.StatusBadRequest

	_, err := e.svc.SetGoogleDirectory(e.wsCtx(e.admin), DirectoryInput{
		ServiceAccountJSON: e.cred.ServiceAccountKey, AdminSubject: e.cred.AdminSubject,
	})
	var invalid settings.InvalidValue
	if !errors.As(err, &invalid) || !strings.Contains(invalid.Reason, "domain-wide delegation") {
		t.Fatalf("err = %v, want a refusal that says what to grant", err)
	}
	if strings.Contains(invalid.Reason, "PRIVATE KEY") {
		t.Fatal("the refusal repeats the key")
	}
	if e.vault.held() != 0 {
		t.Fatal("a credential Google refused was sealed")
	}
	if rows := e.directoryLog(t); len(rows) != 0 {
		t.Fatalf("directory log = %v, want no row: a refused candidate leaves the status alone", rows)
	}
}

func TestAGoogleTokenTheDirectoryDoesNotVouchForGetsNoGroupsAndNoRoles(t *testing.T) {
	cases := []struct {
		name   string
		claims OIDCClaims
		asked  bool
	}{
		{"no hosted domain", OIDCClaims{}, false},
		{"a foreign hosted domain", OIDCClaims{HostedDomain: "other.test"}, false},
		{"a subject the directory does not know", OIDCClaims{HostedDomain: workspaceDomain, Subject: "someone-else"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := setupDirectoryEnv(t, "gdir-vouch")
			e.storeCredential(t)
			e.google.userKeys = nil

			if token, err := e.signIn(t, "google", tc.claims); err != nil || token == "" {
				t.Fatalf("sign-in: token=%q err=%v", token, err)
			}
			if got := heldRoleKeys(t, e.owner, e.member.UserID); len(got) != 0 {
				t.Fatalf("held roles = %v, want none", got)
			}
			if asked := len(e.google.userKeys) > 0; asked != tc.asked {
				t.Fatalf("directory asked = %v (%v), want %v", asked, e.google.userKeys, tc.asked)
			}
		})
	}
}

func TestAStatusBelongsToTheCredentialThatEarnedIt(t *testing.T) {
	e := setupDirectoryEnv(t, "gdir-fingerprint")
	e.storeCredential(t)
	ctx := e.wsCtx(e.admin)

	// A refused replacement answers by its error alone and leaves the working
	// credential's status as it was.
	e.google.groupsStatus = http.StatusForbidden
	rotated := testDirectoryCredential(t)
	if _, err := e.svc.SetGoogleDirectory(ctx, DirectoryInput{
		ServiceAccountJSON: rotated.ServiceAccountKey, AdminSubject: rotated.AdminSubject,
	}); err == nil {
		t.Fatal("a refused replacement was accepted")
	}
	status, err := e.svc.GoogleDirectoryStatus(ctx)
	if err != nil || !status.Configured || status.LastResult != directoryOK {
		t.Fatalf("status after a refused replacement = %+v err=%v, want the working credential's ok", status, err)
	}

	// A credential nobody has checked yet is not the old one's ok.
	e.google.groupsStatus = http.StatusOK
	e.svc.directory.cfg = identityConfig(t, environConfig(t,
		"MARGINCE_IDENTITY_GOOGLE_DIRECTORY_CREDENTIAL="+bootBundle(t, rotated)), e.pool).WithSealer(e.vault.sealer())
	if status, err = e.svc.GoogleDirectoryStatus(ctx); err != nil || !status.Configured || status.LastResult != "" {
		t.Fatalf("status of an unchecked boot credential = %+v err=%v, want configured and not checked", status, err)
	}
}

func bootBundle(t *testing.T, cred directoryCredential) string {
	t.Helper()
	raw, err := json.Marshal(cred)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
