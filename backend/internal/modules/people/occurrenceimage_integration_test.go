// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// The two writes that record an OCCURRENCE on a person rather than a change to
// its fields: a provider's claims arriving, and a human accepting research.
//
// Neither has a prior field state — both land beside the record, in tables the
// person row knows nothing about — so both write SQL NULL into `before` on
// purpose. What they must not do is leave the AFTER empty too: a row with
// neither image is byte-identical to the ones that simply did not look, and it
// records that something happened while being unable to say what.
//
// Neither image may quote a claim's VALUE. Those are the subject's own data,
// and audit_log is evidence rather than subject data — a bought mobile number
// copied here survives the erasure that removes it everywhere else. The KEYS and
// the FIELD names are closed vocabularies and name nobody.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

// occurrenceImages reads one person's newest update row whose after-image
// mentions key, and insists the before is SQL NULL — the occurrence door's own
// claim, which a test that only read the after would let slip.
func occurrenceImages(ctx context.Context, t *testing.T, s *Store, personID ids.PersonID, key string) (after map[string]any, rawAfter string) {
	t.Helper()
	var beforeJSON, afterJSON []byte
	if err := s.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT before, after FROM audit_log
			  WHERE entity_type = 'person' AND entity_id = $1 AND action = 'update' AND after ? $2
			  ORDER BY occurred_at DESC, id DESC LIMIT 1`, personID, key,
		).Scan(&beforeJSON, &afterJSON)
	}); err != nil {
		t.Fatalf("reading the person audit row holding %q: %v", key, err)
	}
	if beforeJSON != nil {
		t.Errorf("before = %s, want SQL NULL — no field of the person held anything before", beforeJSON)
	}
	if err := json.Unmarshal(afterJSON, &after); err != nil {
		t.Fatalf("after is not an object: %v", err)
	}
	return after, string(afterJSON)
}

// wantImageList fails unless image[key] is exactly the strings want, in order.
func wantImageList(t *testing.T, image map[string]any, key string, want ...string) {
	t.Helper()
	//craft:ignore naked-any an audit image is jsonb: a recorded list decodes to []any, which is the type under test
	recorded, ok := image[key].([]any)
	if !ok {
		t.Fatalf("after[%s] = %v, want the list of what happened", key, image[key])
	}
	if len(recorded) != len(want) {
		t.Fatalf("after[%s] = %v, want %v", key, recorded, want)
	}
	for i, name := range want {
		if recorded[i] != name {
			t.Errorf("after[%s][%d] = %v, want %q", key, i, recorded[i], name)
		}
	}
}

// A provider's claims land beside the record, so nothing of the person moved —
// but WHICH claims arrived is what the arrival has to record, and the keys are
// safe to name where the values never are.
func TestAProviderClaimArrivalRecordsWhichClaimsArrived(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	personID, _ := e.seedEmployedPerson(ctx, t,
		"Mira Halvorsen", "mira@voltaq.test", "Voltaq Systems GmbH", "voltaq.test")

	const bought = "bought@voltaq.test"
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return WriteProviderClaims(ctx, tx, e.seedProviderRun(ctx, t, tx, personID), personID.String(), "surfe",
			[]provider.Claim{{
				Key:   provider.ClaimProfessionalEmails,
				Value: []byte(`[{"value":"` + bought + `","validation_status":"valid"}]`),
			}}, time.Now().UTC())
	}); err != nil {
		t.Fatalf("WriteProviderClaims: %v", err)
	}

	after, raw := occurrenceImages(ctx, t, e.store, personID, "provider_claims_received")
	wantImageList(t, after, "provider_claims_received", string(provider.ClaimProfessionalEmails))
	if strings.Contains(raw, bought) {
		t.Errorf("the after image quotes a bought address: %s", raw)
	}
}

// seedProviderRun writes the run a claim hangs off. The run belongs to
// integrations, which people may not write, so the row is inserted here rather
// than through a store — the WRITE under test is the claim arrival, and that
// runs through its real writer.
func (e *dedupeEnv) seedProviderRun(ctx context.Context, t *testing.T, tx pgx.Tx, personID ids.PersonID) string {
	t.Helper()
	var runID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO provider_run
		  (subject_kind, person_id, provider, trigger, state, input_fingerprint,
		   external_correlation_id, connection_version, connection_epoch,
		   configuration_snapshot, requested_categories, completed_at)
		VALUES ('person', $1, 'surfe', 'manual', 'completed', $2,
		        gen_random_uuid(), 1, 1, '{}'::jsonb, ARRAY['professional_email'], now())
		RETURNING id::text`, personID, personID.String()).Scan(&runID); err != nil {
		t.Fatalf("seeding the provider run: %v", err)
	}
	return runID
}

// An acceptance lands in person_profile_field, so no column of the person
// moved — but WHICH fields a human accepted is what the acceptance has to
// record, and those names are the set the table's own constraint closes.
func TestAnAcceptedResearchClaimRecordsWhichFieldsItFilled(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	personID, _ := e.seedEmployedPerson(ctx, t,
		"Mira Halvorsen", "mira@voltaq.test", "Voltaq Systems GmbH", "voltaq.test")

	const title = "Head of Platform"
	saved, err := e.store.SaveResearchClaims(ctx, personID, []ResearchClaimInput{
		{
			Field: "title", Value: title,
			Quote: "Mira Halvorsen, Head of Platform at Voltaq.", SourceURL: "https://voltaq.test/team",
		},
		{
			Field: "linkedin", Value: "https://linkedin.test/in/mira-halvorsen",
			Quote: "Her profile is linked from the team page.", SourceURL: "https://voltaq.test/team",
		},
	})
	if err != nil {
		t.Fatalf("SaveResearchClaims: %v", err)
	}
	if saved != 2 {
		t.Fatalf("saved = %d, want both claims", saved)
	}

	after, raw := occurrenceImages(ctx, t, e.store, personID, "research_claims_accepted")
	wantImageList(t, after, "research_claims_accepted", "title", "linkedin")
	if strings.Contains(raw, title) {
		t.Errorf("the after image quotes an accepted value: %s", raw)
	}
}
