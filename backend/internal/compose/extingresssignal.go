// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The signal arm of the ingress port.
//
// It lands through the signals module's own extension writer, under the same
// live authority an activity runs as. That writer checks the member's grant to
// create a signal and their row scope on the subject. It answers a repeat with
// the signal already held. Nothing here is a second write path.

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension"
)

// landSignal files one signal for the member runCtx acts as.
//
// The subject's type must be a record kind the source declared. A unit that
// lands only companies cannot raise a signal on a deal, whatever the member may
// see. The kind is built from the unit's own name, never taken from the record.
func (r *callRuntime) landSignal(runCtx context.Context, rec extension.Record, declared extension.IngressSource) (extension.Result, error) {
	sig := rec.Signal
	if !slices.Contains(declared.Lands, extension.RecordKind(sig.Subject.Type)) {
		return extension.Result{}, fmt.Errorf("%w: %q does not land signals about %q records", extension.ErrIngressNotDeclared, rec.System, sig.Subject.Type)
	}
	subject, err := ids.Parse(sig.Subject.ID)
	if err != nil {
		return extension.Result{}, fmt.Errorf("%w: the subject id is not a canonical UUID", extension.ErrInvalid)
	}
	in := signals.ExtensionSignal{
		Unit: extension.Name(r.unit), Kind: extension.SignalKindName(extension.Name(r.unit), sig.Kind),
		EntityType: sig.Subject.Type, EntityID: subject,
		Summary: sig.Evidence.Summary, Fields: sig.Evidence.Fields, Fingerprint: rec.Key,
	}
	var id ids.UUID
	var created bool
	err = database.WithWorkspaceTx(runCtx, r.deps.pool, func(tx pgx.Tx) error {
		filed, made, err := signals.RecordExtensionTx(runCtx, tx, in)
		id, created = ids.UUID(filed.Id), made
		return err
	})
	if err != nil {
		return extension.Result{}, r.ingressRefusal(runCtx, err)
	}
	disposition := extension.DispositionAccepted
	if !created {
		disposition = extension.DispositionAlreadyRaised
	}
	return extension.Result{Ref: extension.Ref{Type: tableSignal, ID: id.String()}, Disposition: disposition}, nil
}
