// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The sweep as it actually runs: the production worker, a real database, and a
// vault that records what it was asked to destroy.
//
// The store's own tests prove the predicate. What only this lane can prove is
// the ORDER the file's safety argument rests on — the vault entry is destroyed
// BEFORE the column naming it is cleared, so a failure between the two leaves a
// reference a later pass can still find rather than an orphan nothing can.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// recordingVault answers like the real one and remembers every destruction.
//
// failRef, when set, refuses exactly that reference — the case the sweep must
// survive without stranding the rest of the batch, and without clearing a
// column whose material is still there.
type recordingVault struct {
	deleted []string
	failRef string
}

func (v *recordingVault) Put(context.Context, ids.WorkspaceID, []byte) (keyvault.Ref, error) {
	return "", errors.New("the sweep never stores")
}

func (v *recordingVault) Get(context.Context, ids.WorkspaceID, keyvault.Ref) ([]byte, error) {
	return nil, errors.New("the sweep never reads material")
}

func (v *recordingVault) Delete(_ context.Context, _ ids.WorkspaceID, ref keyvault.Ref) error {
	if v.failRef != "" && string(ref) == v.failRef {
		return errors.New("this vault refuses that reference")
	}
	v.deleted = append(v.deleted, string(ref))
	return nil
}

func (v *recordingVault) GetOn(context.Context, keyvault.Querier, ids.WorkspaceID, keyvault.Ref) ([]byte, error) {
	return nil, errors.New("the sweep never reads material")
}

func (v *recordingVault) Health(context.Context) error { return nil }

// plantExpiredPayload writes one controller delivery holding material that
// expired before `expires`.
func plantExpiredPayload(t *testing.T, p *preflightEnv, ref string, expires time.Time) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := p.Pool.Exec(context.Background(), `
		INSERT INTO comms_outbound
		  (id, activity_id, provider, recipients, cc, references_chain,
		   message_id, subject, body, status, sender_kind,
		   template_key, template_version, payload_ref, payload_expires_at)
		VALUES ($1, $2, 'operator_relay', '["subject@example.test"]'::jsonb,
		        '[]'::jsonb, '[]'::jsonb, $3, 'Your details',
		        'body {{confirmation-link}}', 'pending', 'controller',
		        'record_confirmation', 1, $4, $5)`,
		id, p.activityID, "<sweep-"+id.String()+"@margince.test>", ref, expires); err != nil {
		t.Fatalf("planting expired link material: %v", err)
	}
	return id
}

func payloadRef(t *testing.T, p *preflightEnv, id ids.UUID) string {
	t.Helper()
	var ref *string
	if err := p.Pool.QueryRow(context.Background(),
		`SELECT payload_ref FROM comms_outbound WHERE id = $1`, id).Scan(&ref); err != nil {
		t.Fatalf("reading the payload reference: %v", err)
	}
	if ref == nil {
		return ""
	}
	return *ref
}

// TestTheSweepDestroysMaterialAndThenForgetsIt is the whole pass.
func TestTheSweepDestroysMaterialAndThenForgetsIt(t *testing.T) {
	p := setupPreflight(t)
	now := time.Now().UTC()
	id := plantExpiredPayload(t, p, "vault-ref-expired", now.Add(-time.Hour))

	vault := &recordingVault{}
	if err := compose.DriveControllerPayloadSweepForTest(
		context.Background(), p.Pool, vault, now); err != nil {
		t.Fatalf("the sweep failed: %v", err)
	}

	if len(vault.deleted) != 1 || vault.deleted[0] != "vault-ref-expired" {
		t.Errorf("vault was asked to destroy %v, want the expired material once", vault.deleted)
	}
	if ref := payloadRef(t, p, id); ref != "" {
		t.Errorf("payload_ref is still %q after the sweep — the column outlived the material "+
			"it names, so nothing can tell this row was already retired", ref)
	}
}

// TestAVaultRefusalLeavesTheReferenceStanding holds the order.
//
// The column must NOT be cleared when the destruction failed: clearing it would
// leave live material in the vault with nothing left pointing at it, which no
// later pass — and no erasure — can find by reference again.
func TestAVaultRefusalLeavesTheReferenceStanding(t *testing.T) {
	p := setupPreflight(t)
	now := time.Now().UTC()
	stubborn := plantExpiredPayload(t, p, "vault-ref-refused", now.Add(-2*time.Hour))
	ordinary := plantExpiredPayload(t, p, "vault-ref-ordinary", now.Add(-time.Hour))

	vault := &recordingVault{failRef: "vault-ref-refused"}
	if err := compose.DriveControllerPayloadSweepForTest(
		context.Background(), p.Pool, vault, now); err != nil {
		t.Fatalf("one refused reference failed the whole pass: %v", err)
	}

	if ref := payloadRef(t, p, stubborn); ref != "vault-ref-refused" {
		t.Errorf("payload_ref = %q after a REFUSED destruction, want it standing: "+
			"clearing it strands live material nothing can find", ref)
	}
	// And the rest of the batch still went: one row that will not destroy must
	// not strand the others.
	if ref := payloadRef(t, p, ordinary); ref != "" {
		t.Errorf("payload_ref = %q on the ordinary row — one refusal stranded the batch", ref)
	}
}
