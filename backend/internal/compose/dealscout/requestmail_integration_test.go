// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package dealscout_test

// Which findings the signal pass keeps from private mail, through the pass
// itself with a fixed model reply.

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/dealscout"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// readMixedThread captures a thread whose first message is shared and whose
// second is private for reason, held by heldBy's seat when set. The model finds
// a need in the first and an explicit request in the second. It answers how
// many readings were asked.
func readMixedThread(t *testing.T, e *requestEnv, reason string, heldBy *ids.UUID) int {
	t.Helper()
	company := e.SeedCompany(t, "Acme", &e.AdminUser)
	first, buyer := e.capture(t, company, "We need a platform. Please send a quotation.", mail{role: "from", authenticated: true, thread: "mixed"})
	second, asker := e.capture(t, company, "We need a platform. Please send a quotation.", mail{role: "from", authenticated: true, thread: "mixed", reason: reason})
	if heldBy != nil {
		holder := e.As(*heldBy, nil, integration.AdminPerms)
		if err := database.WithWorkspaceTx(holder, e.Pool, func(tx pgx.Tx) error {
			return capture.NewThreadVerdictStore(compose.InstallationDB(e.Pool)).DecideAsOwner(holder, tx, "mixed", false)
		}); err != nil {
			t.Fatalf("holding the thread: %v", err)
		}
	}
	need := e.opportunity(first, buyer, dealscout.RequestKindNeed, "")
	request := e.opportunity(second, asker, dealscout.RequestKindExplicit, "")
	return e.read(t, need, request)
}

// Mail private only by a mailbox setting is read while both switches are on,
// and only its explicit request is filed, shared. Mail private for any other
// reason, or any private mail with a switch off, is never read.
func TestOnlyAnExplicitRequestIsKeptFromRoutinePrivateMail(t *testing.T) {
	e := setupRequests(t)
	if asked := readMixedThread(t, e, activities.ReasonPosture, nil); asked == 0 {
		t.Fatal("the routine-private thread was not read")
	}
	kinds := e.WsScalar(t, `SELECT coalesce(string_agg(evidence->0->'fields'->>'request_kind' || ':' || visibility, ','), '')
		FROM signal WHERE kind = 'new_opportunity'`)
	if kinds != dealscout.RequestKindExplicit+":workspace" {
		t.Errorf("the pass filed %q, want only the explicit request, shared", kinds)
	}

	for _, refused := range []struct {
		name, reason, off string
		held              bool
	}{
		{"by hand", activities.ReasonManual, "", false},
		{"workspace floor", activities.ReasonWorkspaceFloor, "", false},
		{"no record", activities.ReasonNoRecord, "", false},
		{"held shared mailbox", activities.ReasonSharedMailbox, "", false},
		{"counterparty", activities.ReasonCounterparty, "", false},
		{"reading switched off", activities.ReasonPosture, activities.PrivateMailRequests.Key(), false},
		{"sharing switched off", activities.ReasonPosture, shareKey, false},
		{"held by another seat", activities.ReasonPosture, "", true},
	} {
		t.Run(refused.name, func(t *testing.T) {
			e := setupRequests(t)
			if refused.off != "" {
				switchOff(t, refused.off)
			}
			var heldBy *ids.UUID
			if refused.held {
				heldBy = &e.Rep2
			}
			if asked := readMixedThread(t, e, refused.reason, heldBy); asked != 0 {
				t.Errorf("the model was asked %d times about mail whose content may not be shared", asked)
			}
			if n := e.WsCount(t, `SELECT count(*) FROM signal`); n != 0 {
				t.Errorf("%d signals filed from mail whose content may not be shared", n)
			}
		})
	}
}
