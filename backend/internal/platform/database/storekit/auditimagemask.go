// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/credentialmask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Audit images may last until privacy erasure, so conversation credentials must
// be masked before they enter the log.
var textMaskedAuditTypes = map[string]bool{"activity": true, "scheduled_send": true, "mail_draft": true}

// MaskedAuditEntityTypes are the record types whose audit images the masking
// rules reach, sorted so a caller's query plan does not change between runs.
//
// A repair of images written before those rules derives its own corpus from
// here. A second list would read a smaller tree than the rules do, and the rows
// it missed would keep their credentials with nothing failing.
func MaskedAuditEntityTypes() []string {
	types := make([]string, 0, len(textMaskedAuditTypes))
	for entityType := range textMaskedAuditTypes {
		types = append(types, entityType)
	}
	slices.Sort(types)
	return types
}

// AuditedText is text in the form the audit trail of entityType stores it. A
// reader that compares a live column with an image compares this form.
func AuditedText(entityType, text string) string {
	if !textMaskedAuditTypes[entityType] {
		return text
	}
	return credentialmask.MaskText(text)
}

// HoldsMaskedText reports whether an audit value of entityType is text the
// trail may store with a credential masked out. Text a user typed with the
// marker in it reads the same.
func HoldsMaskedText(entityType, text string) bool {
	return textMaskedAuditTypes[entityType] && strings.Contains(text, credentialmask.Marker)
}

// maskAuditColumns masks the encoded before, after and evidence of one audit
// row in place.
func maskAuditColumns(entityType string, columns ...*[]byte) error {
	for _, column := range columns {
		masked, err := maskAuditImage(entityType, *column)
		if err != nil {
			return err
		}
		*column = masked
	}
	return nil
}

// maskAuditImage masks every string in one encoded column. Numbers decode as
// json.Number so an int64 keeps every digit. A column with nothing to mask keeps
// the bytes it was encoded with.
func maskAuditImage(entityType string, image []byte) ([]byte, error) {
	if image == nil || !textMaskedAuditTypes[entityType] {
		return image, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(image))
	decoder.UseNumber()
	var document any
	if err := decoder.Decode(&document); err != nil {
		return nil, err
	}
	masked, changed := maskImageStrings(entityType, document)
	if !changed {
		return image, nil
	}
	return json.Marshal(masked)
}

//craft:ignore naked-any one node of a decoded audit image, whichever JSON type it holds
func maskImageStrings(entityType string, node any) (any, bool) {
	switch node := node.(type) {
	case string:
		masked := AuditedText(entityType, node)
		return masked, masked != node
	case map[string]any:
		changed := false
		for key, value := range node {
			masked, maskedOne := maskImageStrings(entityType, value)
			node[key], changed = masked, changed || maskedOne
		}
		return node, changed
	case []any:
		changed := false
		for i, value := range node {
			masked, maskedOne := maskImageStrings(entityType, value)
			node[i], changed = masked, changed || maskedOne
		}
		return node, changed
	default:
		return node, false
	}
}

// AuditImageRow is one audit row's images, as a repair reads them before asking
// for a rewrite.
type AuditImageRow struct {
	ID            ids.UUID
	Before, After []byte
}

// AuditRepairSetting is the GUC the append-only trigger reads to admit a
// rewrite of the images, and the one this package sets.
const AuditRepairSetting = "margince.audit_credential_mask"

// EvidenceKeyMaskedAuditImages names the evidence flag on the row recording that
// a record's audit images were masked. The trigger reads that row, written in
// the same transaction, as the certificate for the rewrite.
const EvidenceKeyMaskedAuditImages = "audit_credentials_masked"

// AuditTrailAdmitsMasking answers whether this installation's append-only
// function knows about the masking exception.
//
// Asked rather than inferred from a refusal. A restored strict trigger is a
// supported state, and a caller that forgot the setting is a defect, yet both
// raise the same check violation.
func AuditTrailAdmitsMasking(ctx context.Context, tx pgx.Tx) (bool, error) {
	var admits bool
	err := tx.QueryRow(ctx, `
		SELECT pg_get_functiondef(p.oid) LIKE '%' || @setting || '%'
		  FROM pg_proc p
		  JOIN pg_namespace n ON n.oid = p.pronamespace
		 WHERE p.proname = 'audit_log_immutable' AND n.nspname = 'public'`,
		pgx.NamedArgs{"setting": AuditRepairSetting}).Scan(&admits)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return admits, err
}

// MaskRecordAuditImagesTx masks the credentials in the audit images of one
// record and answers how many rows it rewrote. Before the first rewrite it turns
// the setting on. It also writes the record's certificate row, with no images
// and the evidence flag. The trigger admits a rewrite only with both in this
// transaction.
func MaskRecordAuditImagesTx(
	ctx context.Context, tx pgx.Tx, entityType string, entityID ids.UUID, rows []AuditImageRow,
) (int, error) {
	type rewrite struct{ stored, masked AuditImageRow }
	rewrites := make([]rewrite, 0, len(rows))
	for _, row := range rows {
		before, err := maskAuditImage(entityType, row.Before)
		if err != nil {
			return 0, fmt.Errorf("storekit: masking the before image of audit row %s: %w", row.ID, err)
		}
		after, err := maskAuditImage(entityType, row.After)
		if err != nil {
			return 0, fmt.Errorf("storekit: masking the after image of audit row %s: %w", row.ID, err)
		}
		if !bytes.Equal(before, row.Before) || !bytes.Equal(after, row.After) {
			rewrites = append(rewrites, rewrite{row, AuditImageRow{ID: row.ID, Before: before, After: after}})
		}
	}
	if len(rewrites) == 0 {
		return 0, nil
	}
	if _, err := tx.Exec(ctx, `SELECT set_config($1, 'on', true)`, AuditRepairSetting); err != nil {
		return 0, fmt.Errorf("storekit: admitting the audit image rewrite: %w", err)
	}
	if _, err := AuditEventWithEvidence(ctx, tx, "update", entityType, entityID,
		nil, map[string]any{EvidenceKeyMaskedAuditImages: true}); err != nil {
		return 0, err
	}
	wrote := 0
	for _, r := range rewrites {
		// The images this read are the guard: audit_log carries no version.
		tag, err := tx.Exec(ctx, `
			UPDATE audit_log SET before = $2, after = $3
			 WHERE id = $1 AND before IS NOT DISTINCT FROM $4 AND after IS NOT DISTINCT FROM $5`,
			r.stored.ID, r.masked.Before, r.masked.After, r.stored.Before, r.stored.After)
		if err != nil {
			return wrote, fmt.Errorf("storekit: rewriting the images of audit row %s: %w", r.stored.ID, err)
		}
		wrote += int(tag.RowsAffected())
	}
	return wrote, nil
}
