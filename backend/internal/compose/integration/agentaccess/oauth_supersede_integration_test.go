// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package agentaccess

// Re-consent from one client registration supersedes that client's earlier
// grant rather than accumulating beside it (identity.supersedePriorGrants),
// and the two properties that matter are both end-to-end: the superseded
// connection's access token must stop working immediately, not merely the
// grant row it hangs from, and two DCR registrations of one product must
// survive each other rather than folding into a single connection.

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
)

// registerSecondClient performs a second DCR registration on the same
// harness, the way a second install of one product would: it goes through
// POST /oauth/register directly rather than a fresh setupOAuth, so the new
// client shares the workspace and human that o's own client_id was
// registered under.
func registerSecondClient(t *testing.T, o *oauthEnv) string {
	t.Helper()
	var registered struct {
		ClientID string `json:"client_id"`
	}
	if status := o.Call(t, "POST", "/oauth/register", integration.AnyMap{
		"client_name": "night agent (second install)", "redirect_uris": []string{oauthRedirect},
	}, nil, &registered); status != http.StatusCreated || registered.ClientID == "" {
		t.Fatalf("second DCR → %d %+v", status, registered)
	}
	return registered.ClientID
}

// grantIDForLiveGrant reads the id of the one LIVE grant a client_id
// currently owns. Called immediately after a connect for that client_id and
// before any later consent can touch it, so "the live one" is unambiguous
// even once the test goes on to mint further grants for other clients.
func grantIDForLiveGrant(t *testing.T, o *oauthEnv, clientID string) string {
	t.Helper()
	var grantID string
	if err := o.Owner.QueryRow(context.Background(),
		`SELECT id FROM oauth_grant WHERE client_id = $1 AND revoked_at IS NULL`,
		clientID).Scan(&grantID); err != nil {
		t.Fatalf("reading the live grant for client %q: %v", clientID, err)
	}
	return grantID
}

// grantLiveByID reports whether one grant, named by id rather than by
// client_id, is still live — the by-id counterpart of grantRevoked, needed
// once a test holds more than one grant row and grantRevoked's single-row
// assumption no longer fits.
func grantLiveByID(t *testing.T, o *oauthEnv, grantID string) bool {
	t.Helper()
	var revokedAt *time.Time
	if err := o.Owner.QueryRow(context.Background(),
		`SELECT revoked_at FROM oauth_grant WHERE id = $1`, grantID).Scan(&revokedAt); err != nil {
		t.Fatalf("reading grant %q: %v", grantID, err)
	}
	return revokedAt == nil
}

// A human reconnecting one client should end with ONE live connection, not a
// Settings list that grows a row every time a refresh chain expires. The
// cascade must reach the credential, not only the grant row: a live access
// token under a revoked grant is authority nothing can end.
func TestReconsentSupersedesTheSameClientsGrant(t *testing.T) {
	o := setupOAuth(t)

	firstAccess, _ := o.connect(t)
	firstGrantID := grantIDForLiveGrant(t, o, o.clientID)

	secondAccess, _ := o.connect(t)
	secondGrantID := grantIDForLiveGrant(t, o, o.clientID)

	if grantLiveByID(t, o, firstGrantID) {
		t.Fatal("the first grant must be revoked by the second consent from the same client")
	}
	if !grantLiveByID(t, o, secondGrantID) {
		t.Fatal("the second grant is the live connection")
	}
	if o.accessTokenWorks(t, firstAccess) {
		t.Fatal("the superseded connection's access token must stop working immediately")
	}
	if !o.accessTokenWorks(t, secondAccess) {
		t.Fatal("the surviving connection's access token must still work")
	}
}

// Clients register themselves by DCR, so two installs are two client_ids.
// Supersede is keyed on client_id and must not fold them: connecting a
// laptop cannot silently kill the desktop.
func TestTwoRegistrationsOfOneProductSurviveEachOther(t *testing.T) {
	o := setupOAuth(t)

	laptopAccess, _ := o.connect(t)
	laptopGrantID := grantIDForLiveGrant(t, o, o.clientID)

	desktopClientID := registerSecondClient(t, o)
	o.clientID = desktopClientID
	desktopAccess, _ := o.connect(t)
	desktopGrantID := grantIDForLiveGrant(t, o, o.clientID)

	if !grantLiveByID(t, o, laptopGrantID) || !grantLiveByID(t, o, desktopGrantID) {
		t.Fatal("two client registrations are two connections; neither supersedes the other")
	}
	if !o.accessTokenWorks(t, laptopAccess) || !o.accessTokenWorks(t, desktopAccess) {
		t.Fatal("neither connection's access token should have been touched by the other's consent")
	}
}
