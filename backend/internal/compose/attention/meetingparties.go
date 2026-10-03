// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// Who a meeting row's meeting was with and who hosted it. A meeting's title is
// whatever the calendar said ("Weekly sync"), so without these a reader opens
// the row to learn which customer and whose calendar it is.

import (
	"context"
	"fmt"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ContactEmployers answers where each of a set of contacts works today, under
// the caller's own grants. A contact whose employer the caller may not see is
// absent from the answer.
type ContactEmployers interface {
	CurrentEmployers(ctx context.Context, contactIDs []ids.UUID) (map[ids.UUID]crmcontracts.ContactEmployer, error)
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

// nameTheEmployers puts the counterparty's employer on every meeting row that
// names one, in one read for the page.
func (s *Service) nameTheEmployers(ctx context.Context, rows []crmcontracts.WorklistItem) error {
	if s.employers == nil {
		return nil
	}
	var wanted []ids.UUID
	seen := map[ids.UUID]bool{}
	for _, row := range rows {
		if !isMeetingRow(row) || row.Contact == nil || seen[ids.UUID(row.Contact.Id)] {
			continue
		}
		seen[ids.UUID(row.Contact.Id)] = true
		wanted = append(wanted, ids.UUID(row.Contact.Id))
	}
	if len(wanted) == 0 {
		return nil
	}
	employers, err := s.employers.CurrentEmployers(ctx, wanted)
	if err != nil {
		return fmt.Errorf("attention: naming where the meetings' contacts work: %w", err)
	}
	for i := range rows {
		if !isMeetingRow(rows[i]) || rows[i].Contact == nil {
			continue
		}
		if employer, known := employers[ids.UUID(rows[i].Contact.Id)]; known {
			rows[i].Contact.Employer = &employer
		}
	}
	return nil
}
