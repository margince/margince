// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/erasedmark"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/signaltypes"
)

// scrubSubjectSignals is the scrub an erasure and a retention anonymize share.
// It covers the audit images, the personal evidence and the dedupe key of every
// signal about the contact or citing them. It also covers the reasons of link
// suggestions. A retention anonymize keeps the message text, so it passes no
// activities.
func scrubSubjectSignals(ctx context.Context, tx pgx.Tx, subject ids.ContactID, activities []ids.UUID, reason, cause string) error {
	signals, err := subjectSignals(ctx, tx, subject, activities)
	if err != nil {
		return err
	}
	signalIDs := make([]ids.UUID, len(signals))
	for i, s := range signals {
		signalIDs[i] = s.ID
	}
	if err := tombstoneCollateralScrubs(ctx, tx, "signal", signalIDs, reason, cause); err != nil {
		return err
	}
	if err := redactSignals(ctx, tx, signals); err != nil {
		return err
	}
	return redactLinkSuggestionReasons(ctx, tx, []ids.UUID{subject.UUID}, nil)
}

type heldSignal struct {
	ID       ids.UUID
	Kind     string
	Evidence []byte
}

// subjectSignals reads the signals that name the contact as their entity, that
// resolve to them, or that cite them in a claim. A derived signal on a company
// cites its source messages and the buyer it read. So the claims are searched
// as well as the columns.
func subjectSignals(ctx context.Context, tx pgx.Tx, subject ids.ContactID, activities []ids.UUID) ([]heldSignal, error) {
	cited := make([]string, len(activities))
	for i, a := range activities {
		cited[i] = a.String()
	}
	rows, err := tx.Query(ctx, `
		SELECT id, kind, evidence FROM signal
		 WHERE (entity_type = 'contact' AND entity_id = $1) OR resolved_contact_id = $1
		    OR EXISTS (SELECT 1 FROM jsonb_array_elements(
		         CASE WHEN jsonb_typeof(evidence) = 'array' THEN evidence ELSE '[]'::jsonb END) claim
		       WHERE claim->>'buyer_contact_id' = $1::uuid::text
		          OR (claim->>'source_type' = 'activity' AND claim->>'source_id' = ANY($2)))`, subject, cited)
	if err != nil {
		return nil, fmt.Errorf("privacy: reading the subject's signals: %w", err)
	}
	signals, err := pgx.CollectRows(rows, pgx.RowToStructByPos[heldSignal])
	if err != nil {
		return nil, fmt.Errorf("privacy: collecting the subject's signals: %w", err)
	}
	return signals, nil
}

// redactSignals removes what a contact's erasure or anonymize must reach from
// their signals. The summary and each claim's snippet are free text, so they
// always go, with the quote proof and the buyer id. A layout member of an
// extension signal goes when its declaration marks it personal. Every member
// goes when no declaration is registered. Provenance names a unit and a seat,
// never the subject, and stays. An extension signal's fingerprint goes too. A
// unit builds it from the event's key, which can be an address or a name.
func redactSignals(ctx context.Context, tx pgx.Tx, signals []heldSignal) error {
	for _, s := range signals {
		var claims []crmcontracts.SignalEvidence
		if err := json.Unmarshal(s.Evidence, &claims); err != nil {
			return fmt.Errorf("privacy: signal evidence is not the contract shape: %w", err)
		}
		declared, known := signaltypes.Lookup(s.Kind)
		for i := range claims {
			claims[i].Snippet = erasedText
			claims[i].SourceQuote = nil
			claims[i].BuyerContactId = nil
			claims[i].Fields = keepNonPersonal(claims[i].Fields, declared, known)
		}
		redacted, err := json.Marshal(claims)
		if err != nil {
			return fmt.Errorf("privacy: encoding redacted signal evidence: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE signal SET summary = $2, evidence = $3,
			       fingerprint = CASE WHEN kind LIKE 'ext:%' THEN $4 ELSE fingerprint END
			 WHERE id = $1`, s.ID, erasedText, redacted, erasedmark.Prefix+s.ID.String()); err != nil {
			return fmt.Errorf("privacy: redacting a signal: %w", err)
		}
	}
	return nil
}

// redactLinkSuggestionReasons replaces the free text a unit wrote to justify a
// suggested link. It reaches the suggestions that name a contact as the target
// and those on messages an erasure emptied; either list may be empty.
func redactLinkSuggestionReasons(ctx context.Context, tx pgx.Tx, contacts, activities []ids.UUID) error {
	if _, err := tx.Exec(ctx, `
		UPDATE activity_link_suggestion SET reason = $1
		 WHERE reason <> $1
		   AND ((entity_type = 'contact' AND entity_id = ANY($2)) OR activity_id = ANY($3))`,
		erasedText, contacts, activities); err != nil {
		return fmt.Errorf("privacy: redacting the reasons of link suggestions: %w", err)
	}
	return nil
}

// erasedText replaces the free text of a signal about an erased subject.
const erasedText = "[erased]"

// keepNonPersonal drops the members a declaration marks personal, or all of
// them when the kind has no declaration.
func keepNonPersonal(fields *map[string]any, declared signaltypes.Declared, known bool) *map[string]any {
	if fields == nil {
		return nil
	}
	kept := map[string]any{}
	if known {
		personal := declared.PersonalFields()
		for name, v := range *fields {
			if !slices.Contains(personal, name) {
				kept[name] = v
			}
		}
	}
	return &kept
}
