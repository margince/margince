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

func tombstoneLeadsAndSignals(ctx context.Context, tx pgx.Tx, subject ids.ContactID, leads []ids.UUID, reason string) error {
	if err := tombstoneCollateralScrubs(ctx, tx, "lead", leads, reason, causeContactErasure); err != nil {
		return err
	}
	return scrubSubjectSignals(ctx, tx, subject, reason, causeContactErasure)
}

// scrubSubjectSignals is what an erasure and a retention anonymize both owe the
// signals and link suggestions about a contact: the audit images, the personal
// evidence and the dedupe key, and the suggestion reasons.
func scrubSubjectSignals(ctx context.Context, tx pgx.Tx, subject ids.ContactID, reason, cause string) error {
	if err := tombstoneSubjectSignals(ctx, tx, subject, reason, cause); err != nil {
		return err
	}
	if err := redactSubjectExtensionSignals(ctx, tx, subject); err != nil {
		return err
	}
	return redactLinkSuggestionReasons(ctx, tx, []ids.UUID{subject.UUID}, nil)
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

// redactSubjectExtensionSignals removes what an erasure or anonymize of a
// contact must reach from the extension signals about them. The summary and
// each claim's snippet are free text a unit wrote, so they always go. The
// fingerprint goes too: a unit builds it from the event's own key, which can be
// an address or a name, and the row id keeps the unique index satisfied. A layout
// member goes when its declaration marks it personal. Every member goes when no
// declaration is registered for the kind, because nothing says what is safe.
// Provenance names a unit and a seat, never the subject, and stays.
func redactSubjectExtensionSignals(ctx context.Context, tx pgx.Tx, subject ids.ContactID) error {
	type held struct {
		ID       ids.UUID
		Kind     string
		Evidence []byte
	}
	rows, err := tx.Query(ctx,
		`SELECT id, kind, evidence FROM signal
		  WHERE kind LIKE 'ext:%'
		    AND ((entity_type = 'contact' AND entity_id = $1) OR resolved_contact_id = $1)`, subject)
	if err != nil {
		return fmt.Errorf("privacy: reading the subject's extension signals: %w", err)
	}
	signals, err := pgx.CollectRows(rows, pgx.RowToStructByPos[held])
	if err != nil {
		return fmt.Errorf("privacy: collecting the subject's extension signals: %w", err)
	}
	for _, s := range signals {
		var claims []crmcontracts.SignalEvidence
		if err := json.Unmarshal(s.Evidence, &claims); err != nil {
			return fmt.Errorf("privacy: signal evidence is not the contract shape: %w", err)
		}
		declared, known := signaltypes.Lookup(s.Kind)
		for i := range claims {
			claims[i].Snippet = erasedText
			claims[i].Fields = keepNonPersonal(claims[i].Fields, declared, known)
		}
		redacted, err := json.Marshal(claims)
		if err != nil {
			return fmt.Errorf("privacy: encoding redacted signal evidence: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE signal SET summary = $2, evidence = $3, fingerprint = $4 WHERE id = $1`,
			s.ID, erasedText, redacted, erasedmark.Prefix+s.ID.String()); err != nil {
			return fmt.Errorf("privacy: redacting an extension signal: %w", err)
		}
	}
	return nil
}

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

// tombstoneSubjectSignals cuts the audit history of every signal about the
// subject. A corrected signal keeps its old kind and evidence in the before
// image of an update row, and evidence quotes the source. The audit spine is
// append-only, so the history reads withhold those images by this tombstone.
func tombstoneSubjectSignals(ctx context.Context, tx pgx.Tx, subject ids.ContactID, reason, cause string) error {
	rows, err := tx.Query(ctx,
		`SELECT id FROM signal
		  WHERE (entity_type = 'contact' AND entity_id = $1) OR resolved_contact_id = $1`, subject)
	if err != nil {
		return fmt.Errorf("privacy: reading the subject's signals: %w", err)
	}
	signals, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return fmt.Errorf("privacy: collecting the subject's signals: %w", err)
	}
	return tombstoneCollateralScrubs(ctx, tx, "signal", signals, reason, cause)
}
