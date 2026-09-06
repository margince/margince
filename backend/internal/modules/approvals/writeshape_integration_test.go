// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package approvals

// The rows an approval leaves behind are the same shape every other module's are.
//
// This module wrote its own audit and outbox rows, and they carried less: nine
// columns where storekit writes twelve. The missing one that matters is
// causation_id — without it an approval event cannot be traced to the audit row
// that caused it, which is the link the write shape exists to guarantee. The
// before/after images were absent too, and for an approval those are not
// decoration: its whole content is a state transition, so a row without them
// says a decision happened and not what it decided.
//
// Nothing failed when they were thin. That is what this test is for.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// auditRow is what an approval's audit row carries.
type auditRow struct {
	ID              ids.UUID
	Before, After   []byte
	AuthorizationOK bool
}

// approvalAudit reads the audit row for one approval and one action.
func (e *stagingEnv) approvalAudit(t *testing.T, id ids.ApprovalID, action string) auditRow {
	t.Helper()
	var row auditRow
	if err := e.owner.QueryRow(context.Background(), `
		SELECT id, before, after, authorization_rule IS NOT NULL
		  FROM audit_log
		 WHERE entity_type = 'approval' AND entity_id = $1 AND action = $2
		 ORDER BY id DESC LIMIT 1`, id, action).Scan(
		&row.ID, &row.Before, &row.After, &row.AuthorizationOK); err != nil {
		t.Fatalf("reading the %s audit row: %v", action, err)
	}
	return row
}

// decidedEvent reads the envelope this approval's decision staged.
func (e *stagingEnv) decidedEvent(t *testing.T, id ids.ApprovalID) map[string]any {
	t.Helper()
	var raw []byte
	if err := e.owner.QueryRow(context.Background(), `
		SELECT envelope FROM event_outbox
		 WHERE envelope #>> '{entity,id}' = $1::text
		   AND envelope #>> '{type}' = 'approval.decided'
		 ORDER BY id DESC LIMIT 1`, id).Scan(&raw); err != nil {
		t.Fatalf("reading the decided event: %v", err)
	}
	var env map[string]any
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decoding the envelope: %v", err)
	}
	return env
}

func TestADecidedApprovalWritesTheSameRowShapeEveryModuleDoes(t *testing.T) {
	e := setupStaging(t)
	ctx := e.asHumanWith(decidesEverything())
	org := e.organization(t)
	id := e.stageInto(ctx, t, ids.NewV7(), org, kindDeepRead, "shape-hash")

	if _, err := e.svc.Decide(ctx, id, true, nil); err != nil {
		t.Fatalf("deciding: %v", err)
	}

	row := e.approvalAudit(t, id, "approve")

	// The transition, which is the approval's whole content.
	var before, after map[string]any
	if err := json.Unmarshal(row.Before, &before); err != nil {
		t.Fatalf("the audit row carries no before image (%q): a row saying a decision happened "+
			"without saying what it decided is the half a reader came for", row.Before)
	}
	if err := json.Unmarshal(row.After, &after); err != nil {
		t.Fatalf("the audit row carries no after image (%q)", row.After)
	}
	if before["status"] != StatusPending || after["status"] != StatusApproved {
		t.Errorf("the images say %v → %v, want pending → approved", before["status"], after["status"])
	}

	// The rule the write was admitted under. The private writer left this
	// column null on every approval row in the product.
	if !row.AuthorizationOK {
		t.Error("the audit row records no authorization rule: every other module's rows say which " +
			"rule admitted the write, and an approval's is the one a reader most wants")
	}

	// And the link the whole write shape exists for.
	env := e.decidedEvent(t, id)
	trace, _ := env["trace"].(map[string]any)
	if trace["audit_log_id"] != row.ID.String() {
		t.Errorf("the event's audit_log_id is %v, want %s", trace["audit_log_id"], row.ID)
	}
}

// An approval decided BECAUSE of another event carries that event as its
// causation, so the chain is followable.
//
// A human pressing approve is a chain ROOT and correctly carries none — that is
// what principal.WithCausationEvent's own doc says, and asserting otherwise
// would be asserting the opposite of the design. What the private writer could
// never do is carry one when there IS one: it read only the correlation id and
// the audit id, so every approval event in the product started a new chain
// whatever caused it.
func TestAnApprovalDecidedByAnotherEventNamesItAsTheCause(t *testing.T) {
	e := setupStaging(t)
	ctx := e.asHumanWith(decidesEverything())
	org := e.organization(t)
	id := e.stageInto(ctx, t, ids.NewV7(), org, kindDeepRead, "causation-hash")

	cause := ids.NewV7()
	if _, err := e.svc.Decide(principal.WithCausationEvent(ctx, cause), id, true, nil); err != nil {
		t.Fatalf("deciding: %v", err)
	}

	trace, _ := e.decidedEvent(t, id)["trace"].(map[string]any)
	if trace["causation_id"] != cause.String() {
		t.Errorf("the event's causation_id is %v, want %s — an approval event that cannot be "+
			"traced to what caused it breaks the chain the write shape exists to guarantee",
			trace["causation_id"], cause)
	}
}

// The key every approval token is signed with is minted lazily, on the first
// token this installation issues — and it came into existence with nothing
// recording that it had. A private key appearing is exactly the kind of event
// an audit log is for: the row itself carries a creation timestamp and no
// answer to who was in the building when it was made.
//
// Written as a census over the keys rather than around one mint, because the
// mint is lazy: whichever case in this package caused it, every key there is
// must be accounted for.
func TestNoApprovalSigningKeyComesIntoExistenceUnrecorded(t *testing.T) {
	e := setupStaging(t)
	ctx := e.asHumanWith(decidesEverything())
	org := e.organization(t)
	id := e.stageInto(ctx, t, ids.NewV7(), org, kindDeepRead, "signing-hash")
	if _, err := e.svc.Decide(ctx, id, true, nil); err != nil {
		t.Fatalf("deciding: %v", err)
	}
	if _, err := e.svc.MintApprovalToken(ctx, id); err != nil {
		t.Fatalf("minting the token: %v", err)
	}

	// The census reads a smaller tree than it should if no key was ever minted,
	// and would then report success over nothing.
	if keys := e.count(t, `SELECT count(*) FROM signing_key`); keys == 0 {
		t.Fatal("no signing key exists after a token was minted, so the census below proves nothing")
	}
	if n := e.count(t, `SELECT count(*) FROM signing_key k
		 WHERE NOT EXISTS (SELECT 1 FROM audit_log a
		                    WHERE a.entity_type = 'signing_key' AND a.after->>'kid' = k.kid)`); n != 0 {
		t.Errorf("%d signing key(s) exist with no audit row naming them — a private key came into "+
			"existence and the only record of it is the row it signs with", n)
	}
	// And the record carries the identifier alone. audit_log is append-only and
	// every compliance export reads it, so key material landing there would be
	// key material nobody can take back out.
	if n := e.count(t, `SELECT count(*) FROM audit_log
		 WHERE entity_type = 'signing_key' AND (after - 'kid') <> '{}'::jsonb`); n != 0 {
		t.Errorf("%d signing-key audit row(s) carry more than the kid", n)
	}
}
