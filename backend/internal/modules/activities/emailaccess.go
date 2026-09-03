// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Who reads one email, and what this caller may do about that.
//
// Kept beside the presentation rather than in it because the question is
// about the message rather than in it, and because the answer has to agree
// with the write: the mode this reports is decided by the same test
// refuseCapturedAudienceWrite applies.

// readEmailAccess assembles who reads this message and what this caller may do
// about it.
//
// change_mode is decided here, by the same test the write itself applies:
// refuseCapturedAudienceWrite refuses a direct audience write on a message any
// mailbox brought in, because a captured message's audience is derived from
// its importers rather than declared. The browser has been guessing this from
// the "connector:" prefix on captured_by, which puts a backend ownership rule
// in display code and gets a hand-typed threaded reply wrong. The server knows
// which write it would accept, so the server says.
func readEmailAccess(
	ctx context.Context,
	tx pgx.Tx,
	id ids.ActivityID,
	activity crmcontracts.Activity,
) (crmcontracts.EmailAccess, error) {
	out := crmcontracts.EmailAccess{
		ContentState:  crmcontracts.EmailAccessContentStateAvailable,
		ChangeMode:    crmcontracts.EmailAccessChangeModeNone,
		ChangeScope:   ptr(crmcontracts.EmailAccessChangeScopeNone),
		DisplayStatus: crmcontracts.EmailAccessStatusTeam,
	}
	if activity.Audience != nil {
		aud := crmcontracts.ActivityAudience(*activity.Audience)
		out.Audience = &aud
		out.DisplayStatus = statusForAudience(aud)
	}
	// The reason is content: it describes what the message is about. It travels
	// only with a message the caller may read, which is the branch this is in.
	out.Explanation = activity.AudienceReason

	var imported bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM capture_import WHERE activity_id = $1)`,
		id.UUID).Scan(&imported); err != nil {
		return crmcontracts.EmailAccess{}, fmt.Errorf("activities: reading whether a message was imported: %w", err)
	}

	writable := auth.EnsureActivityWritable(ctx, tx, id.UUID) == nil
	switch {
	case imported:
		// A captured message: the caller changes their own contribution to the
		// thread, and only their own. Every importing seat holds one, so this
		// is offered to an importer rather than to a writer of the row.
		sender, err := callerIsSenderSeat(ctx, tx, id)
		if err != nil {
			return crmcontracts.EmailAccess{}, err
		}
		out.CanChange = sender
		if sender {
			out.ChangeMode = crmcontracts.EmailAccessChangeModeThreadContribution
			out.ChangeScope = ptr(crmcontracts.EmailAccessChangeScopeThread)
		}
	case writable:
		// Hand-logged: its audience is exactly what somebody set, so a writer
		// of the row sets it.
		out.CanChange = true
		out.ChangeMode = crmcontracts.EmailAccessChangeModeMessageAudience
		out.ChangeScope = ptr(crmcontracts.EmailAccessChangeScopeMessage)
	}

	// Who is named on a selected audience, read back only for the caller who
	// may change the set. A reader with no standing to edit it has none to
	// enumerate it either.
	if out.CanChange && activity.Audience != nil && *activity.Audience == crmcontracts.ActivityAudienceSelected {
		members, err := readSelectedMembers(ctx, tx, id)
		if err != nil {
			return crmcontracts.EmailAccess{}, err
		}
		out.SelectedMembers = &members
	}
	return out, nil
}

// statusForAudience is the word the badge prints for a message the caller can
// read. "team" never means the whole workspace: the linked record's own scope
// still decides who may discover the row at all.
func statusForAudience(aud crmcontracts.ActivityAudience) crmcontracts.EmailAccessStatus {
	switch aud {
	case crmcontracts.ActivityAudienceParticipants:
		return crmcontracts.EmailAccessStatusParticipants
	case crmcontracts.ActivityAudienceSelected:
		return crmcontracts.EmailAccessStatusSelected
	default:
		return crmcontracts.EmailAccessStatusTeam
	}
}

func readSelectedMembers(ctx context.Context, tx pgx.Tx, id ids.ActivityID) ([]crmcontracts.AudienceMember, error) {
	rows, err := tx.Query(ctx, `
		SELECT subject_type, subject_id
		  FROM activity_audience_member
		 WHERE activity_id = $1
		 ORDER BY subject_type, subject_id`, id.UUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []crmcontracts.AudienceMember{}
	for rows.Next() {
		var subjectType string
		var subjectID ids.UUID
		if err := rows.Scan(&subjectType, &subjectID); err != nil {
			return nil, err
		}
		out = append(out, crmcontracts.AudienceMember{
			SubjectType: crmcontracts.AudienceMemberSubjectType(subjectType),
			SubjectId:   openapi_types.UUID(subjectID),
		})
	}
	return out, rows.Err()
}
