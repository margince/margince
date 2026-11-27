// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// The engine and importer each admit only their own reserved identities.
// Keeping these sets apart prevents either writer from impersonating the other.
type provenanceAdmission struct{ engineTask, importer bool }

// admits answers whether this writer may spell one reserved source system.
func (a provenanceAdmission) admits(sourceSystem string) bool {
	if a.engineTask && provenance.EngineTaskSource(sourceSystem) {
		return true
	}
	return a.importer && provenance.ImporterNamespace(sourceSystem)
}

// refuseReservedProvenance guards the two provenance fields on a create wire,
// and holds source to the channel list. The store keys its replay on
// (source_system, source_id). A caller spelling the importer's prefix could
// plant a row under an incumbent id, and a later import would take it as there.
// The engine may spell only its task identities, the importer only its namespace.
func refuseReservedProvenance(req crmcontracts.CreateActivityRequest, adm provenanceAdmission) error {
	if req.SourceSystem != nil {
		if !adm.admits(*req.SourceSystem) {
			if err := provenance.Refuse("source_system", *req.SourceSystem); err != nil {
				return err
			}
		}
		// Outside the admission: `email` is the identity every mail transport
		// shares, and no writer on this wire may claim it, the importer included.
		if *req.SourceSystem == connector.EmailSourceSystem {
			return &ReservedMailIdentityError{}
		}
	}
	// `source` is nobody's to forge, and the admission says nothing about it.
	if err := provenance.Refuse("source", req.Source); err != nil {
		return err
	}
	// After the namespace refusal, which names the reserved field more precisely.
	return provenance.AdmitRecordSource(req.Source)
}
