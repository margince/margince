// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// Who a meeting row's meeting was with and who hosted it. A meeting's title is
// whatever the calendar said ("Weekly sync"), so without these a reader opens
// the row to learn which customer and whose calendar it is.

import (
	"context"
	"errors"
	"fmt"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ContactEmployers answers where each of a set of contacts works today, under
// the caller's own grants. A contact whose employer the caller may not see is
// absent from the answer; a caller who may not read contacts or companies at
// all is refused with apperrors.ErrPermissionDenied.
type ContactEmployers interface {
	CurrentEmployers(ctx context.Context, contactIDs []ids.UUID) (map[ids.UUID]crmcontracts.ContactEmployer, error)
}

// MeetingOrganizers answers which colleague organized each of a set of
// meetings, under the caller's own grants. A meeting with no organizing
// colleague is absent from the answer.
type MeetingOrganizers interface {
	MeetingOrganizers(ctx context.Context, meetingIDs []ids.UUID) (map[ids.UUID]ids.UUID, error)
}

// nameTheOrganizers makes a meeting row's host the colleague who organized
// the meeting, where one did. The calendar a meeting came off can be an
// invited colleague's, which synced first. The row's owner stays that
// calendar's seat: who answers for the row is not what this changes.
func (s *Service) nameTheOrganizers(ctx context.Context, rows []crmcontracts.WorklistItem) error {
	if s.organizers == nil {
		return nil
	}
	var meetings []ids.UUID
	for _, row := range rows {
		if isMeetingRow(row) && row.Subject != nil {
			meetings = append(meetings, ids.UUID(row.Subject.Id))
		}
	}
	if len(meetings) == 0 {
		return nil
	}
	organizers, err := s.organizers.MeetingOrganizers(ctx, meetings)
	if err != nil {
		return fmt.Errorf("attention: naming who organized the meetings: %w", err)
	}
	for i := range rows {
		if !isMeetingRow(rows[i]) || rows[i].Subject == nil {
			continue
		}
		if organizer, known := organizers[ids.UUID(rows[i].Subject.Id)]; known {
			rows[i].Host = &crmcontracts.WorklistOwner{
				Kind: crmcontracts.WorklistOwnerKindWorklistOwnerUser, Id: idPtr(organizer),
			}
		}
	}
	return nil
}

// hostOnTheWire is the seat a meeting row's meeting came off, with its name
// still to resolve; nil where no calendar claims the meeting. It reads the same
// host id the row's owner is drawn from.
func hostOnTheWire(item crmcontracts.AttentionItem) *crmcontracts.WorklistOwner {
	host := hostOf(item)
	if host.IsZero() {
		return nil
	}
	return &crmcontracts.WorklistOwner{Kind: crmcontracts.WorklistOwnerKindWorklistOwnerUser, Id: idPtr(host)}
}

// isMeetingRow reports whether a row is about a meeting, before or after it.
func isMeetingRow(row crmcontracts.WorklistItem) bool {
	return row.Source == sourceMeeting || row.Source == sourceMeetingOutcome
}

// nameTheEmployers puts the contact's employer on every row that names a
// contact, in one read for the page, so each card can name the account.
func (s *Service) nameTheEmployers(ctx context.Context, rows []crmcontracts.WorklistItem) error {
	if s.employers == nil {
		return nil
	}
	var wanted []ids.UUID
	seen := map[ids.UUID]bool{}
	for _, row := range rows {
		if row.Contact == nil || seen[ids.UUID(row.Contact.Id)] {
			continue
		}
		seen[ids.UUID(row.Contact.Id)] = true
		wanted = append(wanted, ids.UUID(row.Contact.Id))
	}
	if len(wanted) == 0 {
		return nil
	}
	employers, err := s.employers.CurrentEmployers(ctx, wanted)
	// Refused is withheld: the row still names its contact, and no account.
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("attention: naming where the contacts work: %w", err)
	}
	for i := range rows {
		if rows[i].Contact == nil {
			continue
		}
		if employer, known := employers[ids.UUID(rows[i].Contact.Id)]; known {
			rows[i].Contact.Employer = &employer
		}
	}
	return nil
}
