// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Folding a run's bought answers onto the contact's own record.
//
// Until this existed, a purchase sat beside the record and nothing read it but
// one tab: a rep who bought a contact's employer still saw an empty title on
// the page they work from, and the company roster showed nothing. What is
// bought is now what the record says, under three rules that make that safe.
//
// FILL ONLY WHAT IS EMPTY. Every write here carries its own emptiness
// predicate, so a value somebody typed is never replaced by one somebody sold
// us. The predicate is the compare-and-set, not a prior read: two writers
// racing means the second one lands nothing, which is the outcome we want.
//
// SAY WHAT WE FILLED, NEVER WHAT IT SAID. The audit image names fields and
// marks them filled. audit_log is append-only and outlives the erasure that
// clears the record, so a bought address written into an audit image would be
// exactly the residue the erasure exists to remove.
//
// RECORD ENOUGH TO TAKE IT BACK. Every field this fills gets a
// provider_applied_field row, so "delete the bought data" can clear what the
// provider put there and leave what a colleague wrote since.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

// appliedField is one record field this run filled, as the revert will need to
// find it again.
type appliedField struct {
	// table and field name what was written. rowID identifies a child row;
	// value carries what a plain column was set to. Exactly one is present —
	// a child row proves itself by still existing under this provider's
	// source, while a column has nothing to point at.
	table string
	field string
	rowID *ids.UUID
	value *string
}

// ApplyProviderClaims folds one completed run's claims onto the subject's
// record, inside the hand-off transaction that just wrote them.
//
// It runs in the SAME transaction as WriteProviderClaims on purpose. A second
// transaction would be a second thing that can fail, and the gap between them
// is a run that is paid, stored and invisible — the state claims_unwritten
// already exists to make legible, and one is enough.
//
// It probes and holds the subject itself. The hand-off already took the same
// lock through the holding fence, so this re-take costs nothing inside the same
// transaction — and it is what makes this writer's own correctness readable:
// every fill below puts PII on a shareable record, and a reader should not have
// to trace back through two packages to learn whether anything checked that the
// record is live and the caller may write it.
func ApplyProviderClaims(ctx context.Context, tx pgx.Tx, runID, personID, providerName string, claims []provider.Claim) error {
	subject, err := ids.Parse(personID)
	if err != nil {
		return fmt.Errorf("people: the subject a purchase applies to: %w", err)
	}
	run, err := ids.Parse(runID)
	if err != nil {
		return fmt.Errorf("people: the run a purchase came from: %w", err)
	}
	if err := auth.HoldWritableLive(ctx, tx, entityPerson, subject); err != nil {
		return err
	}

	values, err := decodeApplicableClaims(claims)
	if err != nil {
		return err
	}
	applied, err := applyEach(ctx, tx, subject, providerName, values)
	if err != nil {
		return err
	}
	if len(applied) == 0 {
		return nil
	}
	if err := recordApplied(ctx, tx, subject, run, providerName, applied); err != nil {
		return err
	}
	return auditApplied(ctx, tx, subject, run, providerName, applied)
}

// applyEach runs every fill this run's answers support, collecting what landed.
//
// A field that lands nothing is not an error and not recorded: the record
// already held something, which is the rule working rather than a failure.
func applyEach(ctx context.Context, tx pgx.Tx, subject ids.UUID, providerName string, v applicableClaims) ([]appliedField, error) {
	var applied []appliedField
	fills := []func(context.Context, pgx.Tx, ids.UUID, string, applicableClaims) (filled, error){
		fillTitle,
		fillLinkedIn,
		fillEmployment,
		fillEmail,
		fillPhone,
	}
	for _, fill := range fills {
		landed, err := fill(ctx, tx, subject, providerName, v)
		if err != nil {
			return nil, err
		}
		if landed.ok {
			applied = append(applied, landed.field)
		}
	}
	return applied, nil
}

// recordApplied writes the rows a later revert reads.
func recordApplied(ctx context.Context, tx pgx.Tx, subject, run ids.UUID, providerName string, applied []appliedField) error {
	for _, f := range applied {
		if _, err := tx.Exec(ctx, `
			INSERT INTO provider_applied_field
			       (person_id, run_id, provider, target_table, target_row_id,
			        target_field, applied_value, captured_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT DO NOTHING`,
			subject, run, providerName, f.table, f.rowID, f.field, f.value,
			connectorCapturedBy(providerName)); err != nil {
			return fmt.Errorf("people: recording the %s a purchase filled: %w", f.field, err)
		}
	}
	return nil
}

// auditApplied names the fields and says they were filled — never what they
// say. See the file header: audit_log outlives the erasure.
func auditApplied(ctx context.Context, tx pgx.Tx, subject, run ids.UUID, providerName string, applied []appliedField) error {
	before := make(map[string]any, len(applied))
	after := make(map[string]any, len(applied))
	names := make([]string, 0, len(applied))
	for _, f := range applied {
		// Empty is what every one of these fields held: each fill carries its
		// own emptiness predicate, so a field that landed was empty before.
		before[f.field] = nil
		after[f.field] = "filled"
		names = append(names, f.field)
	}
	auditID, err := storekit.AuditWithEvidence(ctx, tx, "update", entityPerson, subject, before, after,
		map[string]any{auditKeyProvider: providerName, "run_id": run.String(), "fields": names})
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, auditID, subject,
		crmcontracts.PublicEventPersonUpdated{ChangedFields: after})
}

// connectorCapturedBy is who a bought value is attributed to. One spelling,
// shared with the claim writer, so a reader filtering on either finds both.
func connectorCapturedBy(providerName string) string { return "connector:" + providerName }

// ApplyStoredProviderClaims folds a purchase already in this module's table
// onto the subject's record.
//
// The catch-up sweep's half of the applier. Its runs completed before a record
// could hold their values, so there is no payload to hand over — the claims are
// already stored, and the run id is what finds them.
//
// One statement's difference from the hand-off path, and everything after it is
// shared: a purchase applied late must land exactly as one applied on arrival,
// or the record would depend on when the sweep happened to reach it.
func ApplyStoredProviderClaims(ctx context.Context, tx pgx.Tx, personID, runID string) error {
	claims, err := storedClaimsOfRun(ctx, tx, runID)
	if err != nil {
		return err
	}
	if len(claims) == 0 {
		return nil
	}
	providerName, err := claimProviderOfRun(ctx, tx, runID)
	if err != nil {
		return err
	}
	return ApplyProviderClaims(ctx, tx, runID, personID, providerName, claims)
}

// storedClaimsOfRun reads back what one run bought, in the shape the applier
// takes.
func storedClaimsOfRun(ctx context.Context, tx pgx.Tx, runID string) ([]provider.Claim, error) {
	rows, err := tx.Query(ctx, `
		SELECT claim_key, value_json, confidence
		  FROM person_provider_claim
		 WHERE run_id = $1
		 ORDER BY claim_key`, runID)
	if err != nil {
		return nil, fmt.Errorf("people: reading a stored purchase: %w", err)
	}
	defer rows.Close()
	var claims []provider.Claim
	for rows.Next() {
		var c provider.Claim
		var key string
		if err := rows.Scan(&key, &c.Value, &c.Confidence); err != nil {
			return nil, err
		}
		c.Key = provider.ClaimKey(key)
		claims = append(claims, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("people: reading a stored purchase: %w", err)
	}
	return claims, nil
}

// claimProviderOfRun names who sold the claims, read from the claims rather
// than from the run: the run belongs to another module's table, and the claim
// rows carry the same name.
func claimProviderOfRun(ctx context.Context, tx pgx.Tx, runID string) (string, error) {
	var name string
	if err := tx.QueryRow(ctx,
		`SELECT provider FROM person_provider_claim WHERE run_id = $1 LIMIT 1`, runID).Scan(&name); err != nil {
		return "", fmt.Errorf("people: reading which provider sold a stored purchase: %w", err)
	}
	return name, nil
}
