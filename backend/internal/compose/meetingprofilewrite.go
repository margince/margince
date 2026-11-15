// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/meetingprofile"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// applyMeetingProfile writes what a transcript states about how the companies
// on the other side sell. Each value is said by a verified outside participant
// and lands on that contact's current employer; a value no such speaker says
// lands nowhere. A refused reply leaves the earlier reading's values standing.
// A profile the reader may not update is skipped and the reading still counts.
func (p *TranscriptProposer) applyMeetingProfile(
	ctx context.Context, tx pgx.Tx, raw json.RawMessage, lineCount int,
	transcript ids.ActivityID, parties []commitmentParty,
) error {
	entries, fault := meetingprofile.Read(raw, lineCount)
	if fault != "" {
		p.log.WarnContext(ctx, "meeting company profile refused", "reason", fault)
		return nil
	}
	stated, err := statedByCompany(ctx, tx, entries, parties)
	if err == nil {
		err = contacts.ApplyMeetingProfileTx(ctx, tx, transcript, stated)
	}
	if errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound) {
		p.log.InfoContext(ctx, "meeting company profile skipped", "reason", err)
		return nil
	}
	return err
}

// statedByCompany groups the entries by the employer of the outside contact
// who said each one. An entry whose speaker is a colleague, unverified, or
// employed nowhere today is left out.
func statedByCompany(
	ctx context.Context, tx pgx.Tx, entries []meetingprofile.Entry, parties []commitmentParty,
) (map[ids.CompanyID][]contacts.MeetingProfileField, error) {
	refs := commitmentPartyRefs(parties)
	speakers := map[int]ids.UUID{}
	var contactIDs []ids.UUID
	for i, entry := range entries {
		party, found := referencedPromiser(partyKey(refs, entry.SpeakerRef), parties)
		if found && party.Contact != nil && party.Seat == nil {
			speakers[i] = *party.Contact
			contactIDs = append(contactIDs, *party.Contact)
		}
	}
	stated := map[ids.CompanyID][]contacts.MeetingProfileField{}
	if len(contactIDs) == 0 {
		return stated, nil
	}
	employers, err := contacts.CurrentEmployersTx(ctx, tx, contactIDs)
	if err != nil {
		return nil, err
	}
	for i, contact := range speakers {
		employer, employed := employers[contact]
		if !employed {
			continue
		}
		company := ids.From[ids.CompanyKind](ids.UUID(employer.CompanyId))
		entry := entries[i]
		stated[company] = append(stated[company], contacts.MeetingProfileField{
			Field: entry.Field, Value: entry.Value, Confidence: entry.Confidence,
		})
	}
	return stated, nil
}
