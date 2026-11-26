// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/compose/languages"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/signaltypes"
	"github.com/margince/margince/backend/pkg/extension"
	"github.com/margince/margince/backend/pkg/extension/crm"
)

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

// preflightExtensionKinds refuses a set whose company types, record kinds,
// reports, signal types or languages are malformed, before anything is registered.
func preflightExtensionKinds(exts []extension.Extension) error {
	for _, preflight := range []func([]extension.Extension) error{preflightCompanyTypes, preflightKindsLinksSignals, preflightReports, languages.Preflight} {
		if err := preflight(exts); err != nil {
			return err
		}
	}
	return nil
}

// registerExtensionKinds fills the registries the preflight just passed.
func registerExtensionKinds(exts []extension.Extension) {
	for _, register := range []func([]extension.Extension){registerCompanyTypes, registerRecordKinds, registerReports, registerSignalTypes, languages.Register} {
		register(exts)
	}
}
