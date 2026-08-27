// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"context"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// technicalValueKeyKey names the value_key half of one fact's line in the
// audit evidence — which of a multi-valued field's rows this is.
const technicalValueKeyKey = "value_key"

// technicalKeySeparator joins a field and a value_key into one comparable
// string, in Go and in SQL alike. A slug can contain neither a NUL nor this
// character, so the join is unambiguous in both places.
const technicalKeySeparator = "\x1f"

// heldFact is one technical row as the record holds it, with enough of its
// provenance to decide whether the lookup may touch it.
type heldFact struct {
	Field    string
	ValueKey string
	Value    string
	// HumanHeld marks a row a person has claimed. It is read from captured_by
	// rather than from source because that is the column the precedence guard
	// tests, and the two can disagree: a correction rewrites both, but only
	// captured_by decides.
	HumanHeld bool
}

// readTechnicalFacts reads what the record holds for the given fields.
//
// The read is the change detector's before-image: without it, an upsert can
// report that it wrote a row but not whether the row said something different
// beforehand, and "mail moved to Microsoft 365" is precisely that difference.
func readTechnicalFacts(
	ctx context.Context, tx pgx.Tx, orgID ids.OrganizationID, fields []string,
) ([]heldFact, error) {
	rows, err := tx.Query(ctx, `
		SELECT field, value_key, value, captured_by LIKE 'human:%'
		  FROM organization_fact
		 WHERE organization_id = $1 AND category = 'signal' AND field = ANY($2)`,
		orgID, fields)
	if err != nil {
		return nil, fmt.Errorf("read technical facts: %w", err)
	}
	defer rows.Close()

	var held []heldFact
	for rows.Next() {
		var fact heldFact
		if err := rows.Scan(&fact.Field, &fact.ValueKey, &fact.Value, &fact.HumanHeld); err != nil {
			return nil, fmt.Errorf("read technical facts: %w", err)
		}
		held = append(held, fact)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read technical facts: %w", err)
	}
	return held, nil
}

// writeTechnicalFacts lands the observations, refreshing an agent-captured row
// and never touching one a human has since claimed — the same precedence rule
// every other fact writer applies.
//
// It returns the rows actually written, so a human-held row upserts nothing and
// is honestly absent from the delta rather than reported as refreshed.
func writeTechnicalFacts(ctx context.Context, tx pgx.Tx, in TechnicalEnrichment) ([]map[string]any, error) {
	written := make([]map[string]any, 0, len(in.Observations))
	for _, observation := range in.Observations {
		tag, err := tx.Exec(ctx, `
			INSERT INTO organization_fact
			  (organization_id, category, field, value, value_key, evidence_snippet,
			   source_url, source, captured_by, retrieved_at)
			VALUES ($1, 'signal', $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (organization_id, category, field, value_key)
			DO UPDATE SET value = EXCLUDED.value, evidence_snippet = EXCLUDED.evidence_snippet,
			              source_url = EXCLUDED.source_url, source = EXCLUDED.source,
			              captured_by = EXCLUDED.captured_by, retrieved_at = EXCLUDED.retrieved_at,
			              captured_at = now()
			WHERE organization_fact.captured_by NOT LIKE 'human:%'`,
			in.OrganizationID, observation.Field, observation.Value, observation.ValueKey,
			observation.Evidence, observation.SourceURL, companySourceTechnical,
			technicalCapturedBy, in.ObservedAt)
		if err != nil {
			return nil, fmt.Errorf("write technical fact %s: %w", observation.Field, err)
		}
		if tag.RowsAffected() == 1 {
			written = append(written, technicalDelta(
				observation.Field, observation.ValueKey, observation.Value))
		}
	}
	return written, nil
}

// removeUnobservedTechnicalFacts deletes the machine-written rows a completed
// lane no longer observes.
//
// This is what makes a completed lane AUTHORITATIVE rather than additive. An
// upsert-only writer would leave a company that moved from Google Workspace to
// Microsoft 365 carrying both providers forever, and a careers page that came
// down would stay on the record as a reason to call.
//
// Two rows are never removed. One a human claimed is a decision that outranks
// the lookup. One belonging to a lane that did not complete is simply unknown
// this pass — deleting it would turn a certificate log outage into "this
// company operates no services".
func removeUnobservedTechnicalFacts(
	ctx context.Context, tx pgx.Tx, in TechnicalEnrichment, fields []string,
) ([]map[string]any, error) {
	// The pair is compared as one string because SQL has no tuple form of
	// <> ALL. The separator is a character neither half can contain: a fact
	// field and a value_key are both slugs.
	observed := make([]string, 0, len(in.Observations))
	for _, observation := range in.Observations {
		observed = append(observed, observation.Field+technicalKeySeparator+observation.ValueKey)
	}
	rows, err := tx.Query(ctx, `
		DELETE FROM organization_fact
		 WHERE organization_id = $1
		   AND category = 'signal'
		   AND field = ANY($2)
		   AND captured_by NOT LIKE 'human:%'
		   AND (field || $4::text || value_key) <> ALL($3)
		RETURNING field, value_key, value`,
		in.OrganizationID, fields, observed, technicalKeySeparator)
	if err != nil {
		return nil, fmt.Errorf("remove unobserved technical facts: %w", err)
	}
	defer rows.Close()

	removed := []map[string]any{}
	for rows.Next() {
		var field, valueKey, value string
		if err := rows.Scan(&field, &valueKey, &value); err != nil {
			return nil, fmt.Errorf("remove unobserved technical facts: %w", err)
		}
		removed = append(removed, technicalDelta(field, valueKey, value))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("remove unobserved technical facts: %w", err)
	}
	return removed, nil
}

// technicalDelta is one row's line in the audit evidence.
//
// Shared by the written and the removed halves because a reader comparing them
// is comparing the same three keys — a delta whose two halves named their
// columns differently would make "what this write replaced" harder to read
// than it needs to be.
func technicalDelta(field, valueKey, value string) map[string]any {
	return map[string]any{
		evidenceFieldKey: field, technicalValueKeyKey: valueKey, auditKeyValue: value,
	}
}

// withoutHumanSettledFields drops every observation for a single-valued field
// a human already answered.
//
// Only single-valued fields: a person confirming one operated service says
// nothing about whether the company also runs another, so a multi-valued
// field keeps taking new rows beside the one they hold.
func withoutHumanSettledFields(
	in TechnicalEnrichment, held []heldFact,
) TechnicalEnrichment {
	settled := map[string]bool{}
	for _, fact := range held {
		if fact.HumanHeld && singleValuedTechnicalFields[fact.Field] {
			settled[fact.Field] = true
		}
	}
	if len(settled) == 0 {
		return in
	}
	kept := make([]TechnicalObservation, 0, len(in.Observations))
	for _, observation := range in.Observations {
		if !settled[observation.Field] {
			kept = append(kept, observation)
		}
	}
	in.Observations = kept
	return in
}

// technicalChanges is the difference between what the record held and what the
// lookup read, in the words a company event will use.
//
// A human-held row produces no change however the lookup differs from it: the
// record already says what a person decided it says, and announcing a change
// the record did not make would send a rep to look at something that is not
// there.
func technicalChanges(in TechnicalEnrichment, held []heldFact, removed []map[string]any) []TechnicalChange {
	heldByKey := map[string]heldFact{}
	heldByField := map[string][]heldFact{}
	for _, fact := range held {
		heldByKey[fact.Field+technicalKeySeparator+fact.ValueKey] = fact
		heldByField[fact.Field] = append(heldByField[fact.Field], fact)
	}

	var changes []TechnicalChange
	for _, observation := range in.Observations {
		if _, unchanged := heldByKey[observation.Field+technicalKeySeparator+observation.ValueKey]; unchanged {
			continue
		}
		change := TechnicalChange{
			OrganizationID: in.OrganizationID,
			Field:          observation.Field,
			ValueKey:       observation.ValueKey,
			Value:          observation.Value,
			Kind:           TechnicalAppeared,
			Evidence:       observation.Evidence,
		}
		// A single-valued field that already held a different value MOVED
		// rather than appeared — "mail moved to Microsoft 365" is the sentence
		// a rep acts on, and it needs the value that was replaced.
		if previous, ok := replacedValue(heldByField[observation.Field], observation.Field); ok {
			change.Kind = TechnicalMoved
			change.Previous = previous
		}
		changes = append(changes, change)
	}
	changes = append(changes, goneChanges(in, heldByKey, removed)...)
	sort.SliceStable(changes, func(i, j int) bool {
		if changes[i].Field != changes[j].Field {
			return changes[i].Field < changes[j].Field
		}
		return changes[i].ValueKey < changes[j].ValueKey
	})
	return changes
}

// goneChanges reports the rows the reconciliation removed, skipping any whose
// field is single-valued and was reported as a move — a mail provider that
// moved is one event, not an arrival and a departure.
func goneChanges(
	in TechnicalEnrichment, heldByKey map[string]heldFact, removed []map[string]any,
) []TechnicalChange {
	movedFields := map[string]bool{}
	for _, observation := range in.Observations {
		if singleValuedTechnicalFields[observation.Field] {
			movedFields[observation.Field] = true
		}
	}
	var changes []TechnicalChange
	for _, row := range removed {
		field, _ := row[evidenceFieldKey].(string)
		valueKey, _ := row[technicalValueKeyKey].(string)
		value, _ := row[auditKeyValue].(string)
		if movedFields[field] {
			continue
		}
		changes = append(changes, TechnicalChange{
			OrganizationID: in.OrganizationID,
			Field:          field, ValueKey: valueKey, Value: value,
			Previous: value, Kind: TechnicalGone,
			Evidence: heldByKey[field+technicalKeySeparator+valueKey].Value,
		})
	}
	return changes
}

// singleValuedTechnicalFields names the fields a company has exactly one of.
//
// The DDL cannot express this: every `signal` fact is multi-value there,
// because value_key must be non-empty for the whole category. So the rule
// lives here, where the reconciliation can act on it — a company has one mail
// system and one hosting provider, and reading a second is a move rather than
// an addition.
var singleValuedTechnicalFields = map[string]bool{
	FactMailProvider:    true,
	FactHostingProvider: true,
}

// replacedValue reports the value a single-valued field held before, when the
// row that held it was not a human's.
func replacedValue(held []heldFact, field string) (string, bool) {
	if !singleValuedTechnicalFields[field] {
		return "", false
	}
	for _, fact := range held {
		if !fact.HumanHeld {
			return fact.Value, true
		}
	}
	return "", false
}
