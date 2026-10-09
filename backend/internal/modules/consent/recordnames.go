// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// RecordNames answers the display names of one entity type that the READER may
// read, through the owning module's own gated read. Compose injects the shared
// resolver, so a name here is exactly as visible as the record it names.
type RecordNames interface {
	Labels(ctx context.Context, entityType string, want []ids.UUID) (map[ids.UUID]string, error)
}

// WithRecordNames returns a copy that names the contacts and leads its queues
// point at. Unwired, every name is withheld rather than guessed.
func (h Handlers) WithRecordNames(n RecordNames) Handlers {
	h.names = n
	return h
}

// namesOf answers one type's names for a page, in one batched read.
func (h Handlers) namesOf(ctx context.Context, entityType string, want []ids.UUID) (map[ids.UUID]string, error) {
	if h.names == nil || len(want) == 0 {
		return map[ids.UUID]string{}, nil
	}
	return h.names.Labels(ctx, entityType, want)
}

// noticeContactNames names the contacts a page of duties is about.
func (h Handlers) noticeContactNames(ctx context.Context, cases ...NoticeCase) (map[ids.UUID]string, error) {
	contacts := make([]ids.UUID, 0, len(cases))
	for _, c := range cases {
		contacts = append(contacts, c.ContactID.UUID)
	}
	return h.namesOf(ctx, entityContact, contacts)
}

// dsrSubjectLabels names what each request's subject resolves to: a contact
// first, then a lead for the ids no visible contact claimed. Two reads per page
// at most, whatever its length.
func (h Handlers) dsrSubjectLabels(ctx context.Context, requests ...dsrRow) (map[ids.UUID]string, error) {
	subjects := make([]ids.UUID, 0, len(requests))
	for _, d := range requests {
		if subject, ok := resolveDSRSubject(d); ok {
			subjects = append(subjects, subject)
		}
	}
	labels, err := h.namesOf(ctx, entityContact, subjects)
	if err != nil {
		return nil, err
	}
	unclaimed := make([]ids.UUID, 0, len(subjects))
	for _, subject := range subjects {
		if _, named := labels[subject]; !named {
			unclaimed = append(unclaimed, subject)
		}
	}
	leads, err := h.namesOf(ctx, entityLead, unclaimed)
	if err != nil {
		return nil, err
	}
	for id, name := range leads {
		labels[id] = name
	}
	return labels, nil
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
