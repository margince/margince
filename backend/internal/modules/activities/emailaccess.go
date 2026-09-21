// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
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
// change_mode is decided by activityWasImported — the same helper the write
// calls, not a second copy of its question. refuseCapturedAudienceWrite
// refuses a direct audience write on a message any
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
		status, err := statusForAudience(ctx, tx, id, aud)
		if err != nil {
			return crmcontracts.EmailAccess{}, err
		}
		out.DisplayStatus = status
	}
	// The reason is content: it describes what the message is about. It travels
	// only with a message the caller may read, which is the branch this is in.
	out.Explanation = activity.AudienceReason

	imported, err := activityWasImported(ctx, tx, id)
	if err != nil {
		return crmcontracts.EmailAccess{}, err
	}

	// Denial and failure are different answers and must not collapse into one.
	// A == nil test would report can_change:false on a transient database
	// error, which reaches the reader as the Access control quietly not being
	// there — indistinguishable from the product deciding they lack authority.
	writable := false
	switch err := auth.EnsureActivityWritable(ctx, tx, id.UUID); {
	case err == nil:
		writable = true
	case errors.Is(err, apperrors.ErrNotFound), errors.Is(err, apperrors.ErrPermissionDenied):
		// Denied. Not an error to report: the caller may read this message and
		// simply may not change who else does.
	default:
		return crmcontracts.EmailAccess{}, err
	}
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
// read.
//
// A workspace audience is not by itself the whole workspace: the linked
// record's own scope still decides who may discover the row. So the two words
// are told apart by ASKING — a stranger with no team, no ownership and nothing
// shared with them either reaches this row or does not, and that is exactly the
// difference between "everyone here" and "whoever this record admits".
//
// Answered rather than assumed because the badge is a privacy label, and the
// label used to say `team` beside a sentence reading "everyone in the company
// can read this". A reader who takes workspace-wide correspondence for
// team-restricted correspondence has been told something false about who is
// reading, on the one surface where the read boundary is real.
func statusForAudience(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID, aud crmcontracts.ActivityAudience,
) (crmcontracts.EmailAccessStatus, error) {
	narrow := narrowStatusForAudience(aud)
	if narrow != crmcontracts.EmailAccessStatusTeam {
		return narrow, nil
	}
	reach, err := auth.ActivitiesReachingEverySeat(ctx, tx, []ids.UUID{id.UUID})
	if err != nil {
		return "", err
	}
	if reach[id.UUID] {
		return crmcontracts.EmailAccessStatusWorkspace, nil
	}
	return crmcontracts.EmailAccessStatusTeam, nil
}

// narrowStatusForAudience is the word the audience column alone can justify.
// It is the FLOOR of the two-step above and of the page pass in
// emailrowfacts.go, so the editor and every list row start from one reading of
// the column and are widened by one reading of the gate.
func narrowStatusForAudience(aud crmcontracts.ActivityAudience) crmcontracts.EmailAccessStatus {
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
