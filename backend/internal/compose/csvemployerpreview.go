// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

func (w *csvWriters) rememberPredictedEmployer(external string, preview contacts.ContactEmployerPreview) {
	if w.predictedEmployers == nil {
		w.predictedEmployers = map[string]contacts.ContactEmployerPreview{}
	}
	w.predictedEmployers[external] = preview
}

func (r *linkResolver) previewPlannedEmployer(preview contacts.ContactEmployerPreview) (contacts.ContactEmployerPreview, error) {
	name := employerKey(preview.CompanyName)
	for _, domain := range preview.Domains {
		prior, planned := r.plannedEmployerDomains[domain.Domain]
		if !planned {
			continue
		}
		if preview.CompanyID != nil || (r.names[name].landing > 0 && prior != name) {
			return contacts.ContactEmployerPreview{}, httperr.Validation(csvEmployerName, "conflict", "the company name and work domain name different employers; choose the employer")
		}
		preview.Effect = contacts.ContactEmployerResolveEffect
		return preview, nil
	}
	if preview.Effect == contacts.ContactEmployerCreateEffect && r.names[name].landing > 0 {
		preview.Effect = contacts.ContactEmployerResolveEffect
	}
	return preview, nil
}

func (r *linkResolver) markEmployerPending(preview contacts.ContactEmployerPreview) {
	if preview.Effect != contacts.ContactEmployerCreateEffect {
		return
	}
	name := employerKey(preview.CompanyName)
	if name == "" || r.names[name].landing > 0 {
		return
	}
	for _, domain := range preview.Domains {
		if key, valid := naturalKeyFor(migration.ObjectCompany, domain.Domain); valid && r.pendingNatural[key] {
			return
		}
	}
	if r.plannedEmployerNames == nil {
		r.plannedEmployerNames = map[string]bool{}
		r.plannedEmployerDomains = map[string]string{}
	}
	r.markNamesPending(true, preview.CompanyName)
	r.plannedEmployerNames[name] = true
	for _, domain := range preview.Domains {
		r.markNaturalPending(migration.ObjectCompany, domain.Domain)
		r.plannedEmployerDomains[domain.Domain] = name
	}
	clear(r.cache)
}

// Implicit employers have no company input row to bind a pending key to.
// Their released link is checked against this run's created-company identity instead.
func (r *linkResolver) plannedEmployerLink(link migration.ReviewLink) bool {
	if link.Object != migration.ObjectCompany {
		return false
	}
	if r.plannedEmployerNames[employerKey(link.Key)] {
		return true
	}
	key, valid := naturalKeyFor(link.Object, link.Key)
	return valid && r.plannedEmployerDomains[key.Key] != ""
}
