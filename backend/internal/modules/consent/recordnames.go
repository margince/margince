// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"log/slog"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// RecordNames answers the display names of one entity type that the caller may
// read, through the owning module's gated read, which compose injects.
type RecordNames interface {
	Labels(ctx context.Context, entityType string, want []ids.UUID) (map[ids.UUID]string, error)
}

// WithRecordNames returns a copy that names the contacts and leads its queues
// point at. Unwired, every name is withheld rather than guessed.
func (h Handlers) WithRecordNames(n RecordNames) Handlers {
	h.names = n
	return h
}

// namesOf never fails its caller, since a write may already have committed.
// A failed read logs and answers no names, so nobody retries the write.
func (h Handlers) namesOf(ctx context.Context, entityType string, want []ids.UUID) map[ids.UUID]string {
	if h.names == nil || len(want) == 0 {
		return map[ids.UUID]string{}
	}
	labels, err := h.names.Labels(ctx, entityType, want)
	if err != nil {
		slog.ErrorContext(ctx, "consent: privacy queue records could not be named; their names read null",
			"entity_type", entityType, "records", len(want), "err", err)
		return map[ids.UUID]string{}
	}
	return labels
}

func (h Handlers) noticeContactNames(ctx context.Context, cases ...NoticeCase) map[ids.UUID]string {
	contacts := make([]ids.UUID, 0, len(cases))
	for _, c := range cases {
		contacts = append(contacts, c.ContactID.UUID)
	}
	return h.namesOf(ctx, entityContact, contacts)
}

// dsrSubject is the record a request's subject resolved to, as its reader may
// see it.
type dsrSubject struct {
	kind  crmcontracts.DataSubjectRequestSubjectKind
	label string
}

// dsrSubjectLabels resolves each subject to a contact first, then a lead for
// the ids no visible contact claimed: two reads per page at most.
func (h Handlers) dsrSubjectLabels(ctx context.Context, requests ...dsrRow) map[ids.UUID]dsrSubject {
	subjects := make([]ids.UUID, 0, len(requests))
	for _, d := range requests {
		if subject, ok := resolveDSRSubject(d); ok {
			subjects = append(subjects, subject)
		}
	}
	resolved := map[ids.UUID]dsrSubject{}
	for id, name := range h.namesOf(ctx, entityContact, subjects) {
		resolved[id] = dsrSubject{kind: crmcontracts.DataSubjectRequestSubjectKindContact, label: name}
	}
	unclaimed := make([]ids.UUID, 0, len(subjects))
	for _, subject := range subjects {
		if _, named := resolved[subject]; !named {
			unclaimed = append(unclaimed, subject)
		}
	}
	for id, name := range h.namesOf(ctx, entityLead, unclaimed) {
		resolved[id] = dsrSubject{kind: crmcontracts.DataSubjectRequestSubjectKindLead, label: name}
	}
	return resolved
}

// labelOf answers a name the reader may see, or nil. An empty name is a record
// with none, which the wire says as null rather than as a blank.
func labelOf(labels map[ids.UUID]string, id ids.UUID) *string {
	name, ok := labels[id]
	if !ok || name == "" {
		return nil
	}
	return &name
}
