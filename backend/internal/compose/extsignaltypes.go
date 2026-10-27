// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/signaltypes"
	"github.com/margince/margince/backend/pkg/extension"
	"github.com/margince/margince/backend/pkg/extension/crm"
)

// preflightSignalTypes refuses a signal type core could not carry, and a type
// one unit declares twice. Two units never collide, because the unit's name is
// part of the kind name.
func preflightSignalTypes(exts []extension.Extension) error {
	for _, e := range exts {
		seen := make(map[string]bool, len(e.SignalTypes))
		for _, t := range e.SignalTypes {
			if err := t.Validate(); err != nil {
				return fmt.Errorf("compose: extension %q: %w", e.Name, err)
			}
			if seen[t.Kind] {
				return fmt.Errorf("compose: extension %q declares signal type %q twice", e.Name, t.Kind)
			}
			seen[t.Kind] = true
		}
	}
	return nil
}

// registerSignalTypes fills the registry the signals store and the erasure path
// read.
func registerSignalTypes(exts []extension.Extension) {
	for _, e := range exts {
		signaltypes.Register(e.Name, e.SignalTypes)
	}
}

// Create files one signal of a kind this unit declares, on the unit's own
// transaction, so the signal and the unit's rows commit together.
func (s extensionSignals) Create(ctx context.Context, kind string, subject extension.SignalSubject, evidence extension.SignalEvidence, fingerprint string) (crm.Signal, bool, error) {
	if err := s.core.refuseUnattended(); err != nil {
		return crm.Signal{}, false, err
	}
	ctx, err := s.core.authorised(ctx)
	if err != nil {
		return crm.Signal{}, false, err
	}
	subjectID, err := ids.Parse(subject.ID)
	if err != nil {
		return crm.Signal{}, false, fmt.Errorf("%w: %q is not a UUID", extension.ErrInvalid, subject.ID)
	}
	filed, created, err := signals.RecordExtensionTx(ctx, s.core.tx, signals.ExtensionSignal{
		Unit: extension.Name(s.core.unit), Kind: extension.SignalKindName(extension.Name(s.core.unit), kind),
		EntityType: subject.Type, EntityID: subjectID,
		Summary: evidence.Summary, Fields: evidence.Fields, Fingerprint: fingerprint,
	})
	if err != nil {
		return crm.Signal{}, false, portRefusal(err)
	}
	published, err := publishOne[crm.Signal](filed)
	return published, created, err
}
