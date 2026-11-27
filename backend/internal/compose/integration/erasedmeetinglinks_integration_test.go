// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The link secrets of erased meetings are destroyed by the controller payload
// sweep, which a worker registers with a vault and no Gmail registry. The
// sweep is driven through the same wiring a worker gets, never by a direct call.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/erasedmark"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheWiredSweepDestroysTheLinkSecretsOfErasedMeetings(t *testing.T) {
	t.Parallel()
	e := Setup(t)
	vault := keyvault.NewMemory()
	seal := func(secret string) string {
		ref, err := vault.Put(context.Background(), ids.From[ids.WorkspaceKind](e.WS), []byte(secret))
		if err != nil {
			t.Fatal(err)
		}
		return string(ref)
	}
	proposalRef, invitationRef, liveRef := seal("proposal link"), seal("management link"), seal("live link")
	seed := func(managementRef, status, linkRef string) ids.UUID {
		activity := ids.NewV7()
		token := storekit.TokenHash(activity.String())
		if status == "erased" {
			token = erasedmark.Prefix + activity.String()
		}
		e.WsExec(t, `INSERT INTO activity (id, kind, occurred_at, source, captured_by)
			VALUES ($1, 'meeting', now(), 'manual', 'human:test')`, activity)
		e.WsExec(t, `INSERT INTO meeting_proposal (activity_id, host_user_id, request, token_hash, expires_at, link_ref, created_at)
			VALUES ($1, $2, '{}'::jsonb, $3, now() + interval '1 day', $4, now())`, activity, e.AdminUser, token, linkRef)
		e.WsExec(t, `INSERT INTO meeting_invitation (activity_id, host_user_id, provider, calendar_id, appointment, status,
				management_hash, management_ref, next_attempt_at, cleanup_complete)
			VALUES ($1, $2, 'gcal', 'primary', '{}'::jsonb, $3, $4, $5, now(), true)`,
			activity, e.AdminUser, status, "hash-"+activity.String(), managementRef)
		return activity
	}
	erased := seed(invitationRef, "erased", proposalRef)
	live := seed(liveRef, "confirmed", liveRef)

	if err := compose.DriveControllerPayloadSweepForTest(context.Background(), e.Pool, vault, clocktest.Now(t)); err != nil {
		t.Fatal(err)
	}

	if e.VaultHolds(vault, proposalRef) || e.VaultHolds(vault, invitationRef) {
		t.Error("a link secret of an erased meeting survived the sweep")
	}
	if !e.VaultHolds(vault, liveRef) {
		t.Error("the sweep destroyed the link secret of a meeting that was not erased")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM meeting_proposal WHERE activity_id = $1`, erased); n != 0 {
		t.Errorf("the erased proposal row is still there")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM meeting_invitation WHERE activity_id = $1 AND management_ref = ''`, erased); n != 1 {
		t.Errorf("the erased invitation still names its secret")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM meeting_proposal p JOIN meeting_invitation i USING (activity_id)
		WHERE activity_id = $1 AND p.link_ref = $2 AND i.management_ref = $2`, live, liveRef); n != 1 {
		t.Error("the live meeting rows were changed")
	}
}
