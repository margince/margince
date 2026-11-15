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

// applyMeetingProfile writes what a transcript states about how the company
// on the other side sells onto that company's profile. The company is the one
// current employer of the meeting's outside contacts; with none, or several,
// the reading cannot say whose profile it describes, so it writes nothing.
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
	if len(entries) == 0 {
		return nil
	}
	company, found, err := meetingCompany(ctx, tx, parties)
	if err != nil || !found {
		return skippedProfile(err)
	}
	fields := make([]contacts.MeetingProfileField, 0, len(entries))
	for _, entry := range entries {
		fields = append(fields, contacts.MeetingProfileField{Field: entry.Field, Value: entry.Value, Confidence: entry.Confidence})
	}
	if _, err := contacts.ApplyMeetingProfileTx(ctx, tx, company, transcript, fields); err != nil {
		if skipped := skippedProfile(err); skipped == nil {
			p.log.InfoContext(ctx, "meeting company profile skipped", "reason", err)
			return nil
		}
		return err
	}
	return nil
}

// meetingCompany is the one current employer of the meeting's outside
// contacts, if there is exactly one.
func meetingCompany(ctx context.Context, tx pgx.Tx, parties []commitmentParty) (ids.CompanyID, bool, error) {
	var outside []ids.UUID
	for _, party := range parties {
		if party.Contact != nil && party.Seat == nil {
			outside = append(outside, *party.Contact)
		}
	}
	if len(outside) == 0 {
		return ids.CompanyID{}, false, nil
	}
	employers, err := contacts.CurrentEmployersTx(ctx, tx, outside)
	if err != nil {
		return ids.CompanyID{}, false, err
	}
	companies := map[ids.UUID]bool{}
	for _, employer := range employers {
		companies[ids.UUID(employer.CompanyId)] = true
	}
	if len(companies) != 1 {
		return ids.CompanyID{}, false, nil
	}
	for company := range companies {
		return ids.From[ids.CompanyKind](company), true, nil
	}
	return ids.CompanyID{}, false, nil
}

// skippedProfile is nil for a refusal that only means this reader may not see
// or change the company; any other fault is the reading's to retry.
func skippedProfile(err error) error {
	if errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound) {
		return nil
	}
	return err
}
