// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/credentialmask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// textMaskedAuditTypes are the record types whose audit rows carry mail text or
// text a user typed about a conversation. audit_log is append-only, so a
// credential written into it in clear could never be taken out again.
var textMaskedAuditTypes = map[string]bool{"activity": true, "scheduled_send": true, "mail_draft": true}

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
	ID         ids.UUID
	EntityType string
	Before     []byte
	After      []byte
}

// AuditRepairSetting is the GUC the append-only trigger reads to admit a
// rewrite of the images, and the one this package sets.
const AuditRepairSetting = "margince.audit_credential_mask"

// EvidenceKeyMaskedAuditImages names the evidence flag on a row recording that
// an entity's audit images were masked.
//
// Evidence and not an image, because an image is projected as a field change
// and none of the entity's fields changed.
const EvidenceKeyMaskedAuditImages = "audit_credentials_masked"

// AuditTrailAdmitsMasking answers whether this installation's append-only
// function knows about the masking exception.
//
// Asked rather than inferred from a refusal. A restored strict trigger and a
// caller that forgot the setting raise the same check violation.
//
// One is a supported state and the other a defect. A caller that treated them
// alike would go quiet when it had stopped working.
func AuditTrailAdmitsMasking(ctx context.Context, tx pgx.Tx) (bool, error) {
	var admits bool
	err := tx.QueryRow(ctx, `
		SELECT pg_get_functiondef(p.oid) LIKE '%' || @setting || '%'
		  FROM pg_proc p
		  JOIN pg_namespace n ON n.oid = p.pronamespace
		 WHERE p.proname = 'audit_log_immutable' AND n.nspname = 'public'`,
		pgx.NamedArgs{"setting": AuditRepairSetting}).Scan(&admits)
	if errors.Is(err, pgx.ErrNoRows) {
		// No append-only trigger means no exception to make, which no supported
		// installation has. Said as a refusal rather than a failure: the trail
		// is not a caller's to install.
		return false, nil
	}
	return admits, err
}

// MaskAuditImagesTx masks the credentials in one audit row's images and reports
// whether it wrote.
//
// The one rewrite of an audit image in this tree, and it lives here because
// this package owns audit_log.
//
// The trigger admits it only while the setting is on, and only when nothing but
// the two image columns moves.
//
// Held by: TestEveryPackageOnlyWritesTablesItOwns (backend/gates/tableownership_test.go),
// which fails any other package that writes audit_log.
func MaskAuditImagesTx(ctx context.Context, tx pgx.Tx, row AuditImageRow) (bool, error) {
	before, err := maskAuditImage(row.EntityType, row.Before)
	if err != nil {
		return false, fmt.Errorf("storekit: masking the before image of audit row %s: %w", row.ID, err)
	}
	after, err := maskAuditImage(row.EntityType, row.After)
	if err != nil {
		return false, fmt.Errorf("storekit: masking the after image of audit row %s: %w", row.ID, err)
	}
	if bytes.Equal(before, row.Before) && bytes.Equal(after, row.After) {
		// Nothing to mask, so nothing is written: the trail's one permitted
		// rewrite is not spent on a row that holds no credential.
		return false, nil
	}
	if _, err := tx.Exec(ctx,
		`SELECT set_config($1, 'on', true)`, AuditRepairSetting); err != nil {
		return false, fmt.Errorf("storekit: admitting the audit image rewrite: %w", err)
	}
	// The images this read are the guard. audit_log carries no version, and a
	// row whose images moved under the read is one this pass has not seen.
	tag, err := tx.Exec(ctx, `
		UPDATE audit_log SET before = $2, after = $3
		 WHERE id = $1 AND before IS NOT DISTINCT FROM $4 AND after IS NOT DISTINCT FROM $5`,
		row.ID, before, after, row.Before, row.After)
	if err != nil {
		return false, fmt.Errorf("storekit: rewriting the images of audit row %s: %w", row.ID, err)
	}
	return tag.RowsAffected() == 1, nil
}
