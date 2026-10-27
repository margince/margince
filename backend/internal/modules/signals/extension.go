// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package signals

// Filing a signal of a kind an extension declared. The row, its audit entry and
// its outbox event are written in the caller's transaction, as CreateSignal
// does. What differs is the kind's evidence layout, the provenance core stamps,
// and the fingerprint that makes a repeat report one signal.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/signaltypes"
	"github.com/margince/margince/backend/pkg/extension"
)

const (
	signalEvidenceArg = "evidence"
	channelDerived    = "derived"
)

// ExtensionSignal is one report of an extension's signal type.
type ExtensionSignal struct {
	// Unit is the extension filing the signal. It must own Kind.
	Unit extension.Name
	// Kind is the full `ext:<unit>:<kind>` name.
	Kind        string
	EntityType  string
	EntityID    ids.UUID
	Summary     string
	Fields      map[string]any
	Fingerprint string
}

// InvalidExtensionSignalError answers 422 on the field that broke the layout or
// the ownership of the kind. The message names the member and never a value.
type InvalidExtensionSignalError struct{ Field, Reason string }

func (e *InvalidExtensionSignalError) Error() string { return e.Field + ": " + e.Reason }

// FieldFault carries the verdict so httperr answers it without a per-type list.
func (e *InvalidExtensionSignalError) FieldFault() (field, code, message string) {
	return e.Field, "invalid_evidence", e.Error()
}

// refuseReservedEvidence refuses the members only core writes, when a request
// carries them: a human or an agent could otherwise forge a provenance.
func refuseReservedEvidence(evidence []crmcontracts.SignalEvidence) error {
	for _, claim := range evidence {
		if claim.Fields != nil || claim.Provenance != nil {
			return &InvalidExtensionSignalError{Field: signalEvidenceArg, Reason: "fields and provenance are written by core for extension signals, and a request may not carry them"}
		}
	}
	return nil
}

// extensionFingerprint scopes a unit's fingerprint to its kind. The unique index
// is shared, so without the scope two kinds, or a unit and a core producer,
// could swallow each other's signals.
func extensionFingerprint(kind, fingerprint string) string { return kind + "|" + fingerprint }

// RecordExtensionTx files the signal, or answers the live signal that carries
// the same fingerprint with created false. Either answer is a read, so the
// caller's row scope applies to a replay as it does to a first write.
func RecordExtensionTx(ctx context.Context, tx pgx.Tx, in ExtensionSignal) (sig crmcontracts.Signal, created bool, err error) {
	if err := auth.Require(ctx, "signal", principal.ActionCreate); err != nil {
		return crmcontracts.Signal{}, false, err
	}
	declared, err := declaredType(in)
	if err != nil {
		return crmcontracts.Signal{}, false, err
	}
	if !signalEntityTables[in.EntityType] {
		return crmcontracts.Signal{}, false, &InvalidSignalEntityTypeError{EntityType: in.EntityType}
	}
	if err := auth.EnsureReferenceTarget(ctx, tx, "entity_id", in.EntityType, in.EntityID); err != nil {
		return crmcontracts.Signal{}, false, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return crmcontracts.Signal{}, false, err
	}
	summary := strings.TrimSpace(in.Summary)
	if summary == "" {
		summary = declared.Label
	}
	evidence, err := extensionEvidence(summary, in, by)
	if err != nil {
		return crmcontracts.Signal{}, false, err
	}
	stored := extensionFingerprint(in.Kind, in.Fingerprint)
	var id ids.UUID
	err = tx.QueryRow(ctx,
		`INSERT INTO signal (kind, source_channel, entity_type, entity_id, resolution_state, severity,
		                     summary, evidence, fingerprint, detected_at, source, captured_by)
		 VALUES ($1, 'derived', $2, $3, 'resolved', 'info', $4, $5, $6, $7, $8, $9)
		 ON CONFLICT DO NOTHING
		 RETURNING id`,
		in.Kind, in.EntityType, in.EntityID, summary, evidence, stored,
		time.Now().UTC(), string(in.Unit), by).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		sig, err = readByFingerprint(ctx, tx, stored)
		return sig, false, err
	}
	if err != nil {
		return crmcontracts.Signal{}, false, fmt.Errorf("insert extension signal: %w", err)
	}
	auditID, err := storekit.Audit(ctx, tx, "create", "signal", id, nil,
		map[string]any{signalKindArg: in.Kind, "source_channel": channelDerived, "resolution_state": "resolved"})
	if err != nil {
		return crmcontracts.Signal{}, false, fmt.Errorf("audit extension signal: %w", err)
	}
	if sig, err = readSignal(ctx, tx, ids.From[ids.SignalKind](id), storekit.LiveOnly); err != nil {
		return crmcontracts.Signal{}, false, fmt.Errorf("read extension signal: %w", err)
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, id, detectedPayload(sig)); err != nil {
		return crmcontracts.Signal{}, false, fmt.Errorf("emit signal.detected: %w", err)
	}
	return sig, true, nil
}

// declaredType answers the layout of the kind, and refuses a kind the unit does
// not own or evidence that does not fit the layout.
func declaredType(in ExtensionSignal) (signaltypes.Declared, error) {
	declared, ok := signaltypes.Lookup(in.Kind)
	if !ok || declared.Unit != in.Unit {
		return signaltypes.Declared{}, &InvalidExtensionSignalError{Field: signalKindArg, Reason: "kind is not a signal type this extension declares"}
	}
	if strings.TrimSpace(in.Fingerprint) == "" {
		return signaltypes.Declared{}, &InvalidExtensionSignalError{Field: "fingerprint", Reason: "a fingerprint is required, so the same event is recorded once"}
	}
	if err := declared.CheckFields(in.Fields); err != nil {
		return signaltypes.Declared{}, &InvalidExtensionSignalError{Field: signalEvidenceArg, Reason: err.Error()}
	}
	return declared, nil
}

// extensionEvidence builds the one evidence claim an extension signal carries.
// The provenance member is written here from the principal and the unit, never
// from the caller's fields.
func extensionEvidence(summary string, in ExtensionSignal, capturedBy string) ([]byte, error) {
	claim := crmcontracts.SignalEvidence{
		Snippet: summary,
		Fields:  &in.Fields,
		Provenance: &crmcontracts.SignalProvenance{
			Unit: string(in.Unit), CapturedBy: capturedBy,
		},
	}
	out, err := json.Marshal([]crmcontracts.SignalEvidence{claim})
	if err != nil {
		return nil, fmt.Errorf("marshal extension signal evidence: %w", err)
	}
	return out, nil
}

// readByFingerprint answers the live signal a fingerprint names, behind the
// caller's visibility check. A signal the caller cannot see is not found, so a
// fingerprint cannot be used to learn that one exists.
func readByFingerprint(ctx context.Context, tx pgx.Tx, fingerprint string) (crmcontracts.Signal, error) {
	var id ids.UUID
	err := tx.QueryRow(ctx,
		`SELECT id FROM signal WHERE fingerprint = $1 AND archived_at IS NULL`, fingerprint).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return crmcontracts.Signal{}, apperrors.ErrNotFound
	}
	if err != nil {
		return crmcontracts.Signal{}, fmt.Errorf("read signal by fingerprint: %w", err)
	}
	return ReadSignalTx(ctx, tx, ids.From[ids.SignalKind](id), storekit.LiveOnly)
}
