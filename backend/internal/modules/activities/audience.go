// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Who may read one activity's content. The row scope decides who may learn an
// activity EXISTS (the link walk, platform/auth); the audience set here decides
// who reads what was said. It is per message by design — limiting one email
// says nothing about its thread siblings or the people on it — and it is a
// human's call about correspondence they have write authority over.

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// AudienceMember names one user or team admitted to a `selected` audience.
type AudienceMember struct {
	SubjectType string // "user" | "team"
	SubjectID   ids.UUID
}

// SetAudienceInput is one audience write: the new audience and, for
// `selected`, the full member set that replaces the previous one.
type SetAudienceInput struct {
	Audience  string
	Members   []AudienceMember
	IfVersion *int64
}

// maxAudienceMembers bounds one `selected` audience — the contract's maxItems.
const maxAudienceMembers = 200

// SetAudience limits (or re-opens) who may read the activity's content.
// Human-only: an agent acting for a human never narrows or widens what the
// human's colleagues read. The caller needs write authority over the row,
// which is the author, the assignee or host, or a linked record that is
// theirs to change. The audience column, the member rows, the audit row and
// the activity.updated event commit together; a row held under a retention
// obligation is refused by the restriction guard (423).
func (s *Store) SetAudience(ctx context.Context, id ids.ActivityID, in SetAudienceInput) (crmcontracts.Activity, error) {
	if err := auth.RequireHuman(ctx); err != nil {
		return crmcontracts.Activity{}, err
	}
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return crmcontracts.Activity{}, err
	}
	if !crmcontracts.ActivityAudience(in.Audience).Valid() {
		return crmcontracts.Activity{}, &InvalidAudienceError{Audience: in.Audience}
	}
	members, err := audienceMembersFor(in)
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	var out crmcontracts.Activity
	err = s.tx(ctx, func(tx pgx.Tx) error {
		held, err := lockActivityForWrite(ctx, tx, id.UUID)
		if err != nil {
			return err
		}
		if err := auth.EnsureActivityWritableIn(ctx, tx, id.UUID, !held); err != nil {
			return err
		}
		// A CAPTURED message is not one person's to set.
		//
		// Its audience is derived across every mailbox that imported it, and
		// each importer's contribution is theirs alone — so writing the column
		// directly would either override a colleague's hold or be silently
		// undone by the next recompute, and both are worse than a refusal. The
		// owner endpoint changes the contribution instead, which is the thing
		// the derivation actually reads.
		//
		// A hand-logged row has no contributors and stays writable here.
		//
		// held skips this: a captured, held row is refused by
		// activity_refuse_restricted_mutation below either way, and that
		// refusal outranks this one — 422 tells the caller their request is
		// fixable by asking the owner endpoint instead, which a held row's
		// request never is.
		if !held {
			if err := refuseCapturedAudienceWrite(ctx, tx, id); err != nil {
				return err
			}
		}
		if err := ensureVersion(ctx, tx, id, in.IfVersion, held); err != nil {
			return err
		}
		if err := ensureAudienceSubjectsExist(ctx, tx, members); err != nil {
			return err
		}
		current, err := readAudienceImage(ctx, tx, id)
		if err != nil {
			return err
		}
		// The reason moves with the audience, and becomes `manual`.
		//
		// Not NULL, and not the previous reason. Leaving the previous one would
		// re-narrow the row: RecomputeAudienceTx reads audience_reason to
		// recognise a hold no capture_import row records, so a stale
		// `workspace_floor` on a row a person just opened is read as a live hold
		// on the next sync of any mailbox that has the message. Clearing it to
		// NULL loses the opposite thing — that a HUMAN decided this — and the
		// derivation would widen the row back for the same reason.
		//
		// `manual` says both: a person set this, and no derivation may move it.
		if _, err := tx.Exec(ctx,
			`UPDATE activity SET audience = $2, audience_reason = $3 WHERE id = $1`,
			id, in.Audience, ReasonManual); err != nil {
			return err
		}
		if err := replaceAudienceMembers(ctx, tx, id, members); err != nil {
			return err
		}
		stored, err := readAudienceImage(ctx, tx, id)
		if err != nil {
			return err
		}
		before, after := storekit.ChangedColumns(current, stored)
		auditID, err := storekit.Audit(ctx, tx, "update", "activity", id.UUID, before, after)
		if err != nil {
			return err
		}
		audience := crmcontracts.PublicEventActivityChangedFieldsAudience(in.Audience)
		if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventActivityUpdated{
			ChangedFields: crmcontracts.PublicEventActivityChangedFields{Audience: &audience},
		}); err != nil {
			return err
		}
		out, err = readActivity(ctx, tx, id, storekit.LiveOnly)
		return err
	})
	return out, err
}

// readAudienceImage renders one side of the audience audit diff: the column on
// the activity row, and the member rows that qualify it.
//
// Both sides come from the tables rather than from the request. The member
// write dedupes on conflict and a non-`selected` audience leaves no member rows
// at all, so an image assembled from the input would name a set the row does
// not hold.
//
// Members render as sorted `type:id` words so an unchanged set compares equal
// across two reads: the rows come back in whatever order the index hands them
// over, and an order difference is not a change anyone made.
func readAudienceImage(ctx context.Context, tx pgx.Tx, id ids.ActivityID) (map[string]any, error) {
	var audience string
	var reason *string
	if err := tx.QueryRow(ctx,
		`SELECT audience, audience_reason FROM activity WHERE id = $1`, id).Scan(&audience, &reason); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx,
		`SELECT subject_type, subject_id FROM activity_audience_member WHERE activity_id = $1`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	members := []string{}
	for rows.Next() {
		var subjectType string
		var subjectID ids.UUID
		if err := rows.Scan(&subjectType, &subjectID); err != nil {
			return nil, err
		}
		members = append(members, subjectType+":"+subjectID.String())
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	slices.Sort(members)
	why := ""
	if reason != nil {
		why = *reason
	}
	return map[string]any{auditFieldAudience: audience, auditFieldAudienceReason: why, "members": members}, nil
}

// audienceMembersFor validates the member set: read only for `selected`,
// bounded, and every subject type a word the table admits.
func audienceMembersFor(in SetAudienceInput) ([]AudienceMember, error) {
	if in.Audience != string(crmcontracts.ActivityAudienceSelected) {
		return nil, nil
	}
	if len(in.Members) > maxAudienceMembers {
		return nil, &InvalidAudienceError{Audience: in.Audience, Reason: "too many members"}
	}
	for _, m := range in.Members {
		if m.SubjectType != "user" && m.SubjectType != "team" {
			return nil, &InvalidAudienceError{Audience: in.Audience, Reason: "unknown subject_type " + m.SubjectType}
		}
		if m.SubjectID == ids.Nil {
			return nil, &InvalidAudienceError{Audience: in.Audience, Reason: "a member needs a subject_id"}
		}
	}
	return in.Members, nil
}

// ensureVersion is the If-Match compare, against the row the caller just
// locked; a mismatch is version skew, which the wire answers as 409.
//
// held=true skips the compare entirely: every write to a held row is
// refused by activity_refuse_restricted_mutation regardless of which
// version the caller supplied, so a stale version on a held row still owes
// 423, the reachable answer the write below reaches — not 409, which
// would tell the caller a refetch-and-retry could ever succeed against a
// row nothing can write to.
//
// No archived filter otherwise: lockActivityForWrite already proved the
// row exists, live or held, before this runs, so re-filtering here would
// hide a held row's version from under a lock that already resolved it —
// exactly the 404-instead-of-423 lockActivityForWrite exists to remove.
func ensureVersion(ctx context.Context, tx pgx.Tx, id ids.ActivityID, ifVersion *int64, held bool) error {
	if ifVersion == nil || held {
		return nil
	}
	var current int64
	if err := tx.QueryRow(ctx, `SELECT version FROM activity WHERE id = $1`, id).Scan(&current); err != nil {
		return err
	}
	if current != *ifVersion {
		return apperrors.ErrVersionSkew
	}
	return nil
}

// ensureAudienceSubjectsExist refuses a member that names no live user or
// team of this workspace: the table carries no FK for its polymorphic subject,
// so the check is here, and a guessed id answers like a malformed one.
//
// LIVE is both halves. Deactivating an account sets `status` and leaves
// `archived_at` NULL, so the archived-only test admitted a colleague who has
// left — this comment said "live" while the query asked something weaker, and
// an audience is a list of people expected to read the thing. Spelled out
// because a module never imports a sibling (ADR-0054 §3); identity owns
// app_user and TestOnlyOneSpellingOfALiveMember holds the two together.
func ensureAudienceSubjectsExist(ctx context.Context, tx pgx.Tx, members []AudienceMember) error {
	for _, m := range members {
		var exists bool
		var q string
		if m.SubjectType == "user" {
			q = `SELECT EXISTS (SELECT 1 FROM app_user
			                 WHERE id = $1 AND status = 'active' AND archived_at IS NULL)`
		} else {
			q = `SELECT EXISTS (SELECT 1 FROM team WHERE id = $1)`
		}
		if err := tx.QueryRow(ctx, q, m.SubjectID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return &InvalidAudienceError{Audience: "selected", Reason: fmt.Sprintf("%s %s does not exist", m.SubjectType, m.SubjectID)}
		}
	}
	return nil
}

// replaceAudienceMembers writes the full member set, replacing whatever a
// previous `selected` audience named. An audience other than `selected`
// leaves no member rows behind.
func replaceAudienceMembers(ctx context.Context, tx pgx.Tx, id ids.ActivityID, members []AudienceMember) error {
	if _, err := tx.Exec(ctx, `DELETE FROM activity_audience_member WHERE activity_id = $1`, id); err != nil {
		return err
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		return errors.New("activities: no principal bound for the audience write")
	}
	for _, m := range members {
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity_audience_member (activity_id, subject_type, subject_id, created_by)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (activity_id, subject_type, subject_id) DO NOTHING`,
			id, m.SubjectType, m.SubjectID, actor.ID); err != nil {
			return err
		}
	}
	return nil
}

// readActivityContent is readActivity for a caller about to USE the content —
// reply to it, transcribe it, send on its thread: the audience gate runs as a
// probe first, so a limited conversation answers ErrNotFound rather than a
// row with its text blanked.
func readActivityContent(ctx context.Context, tx pgx.Tx, id ids.ActivityID, archived storekit.ArchivedFilter) (crmcontracts.Activity, error) {
	if err := auth.EnsureActivityContentVisible(ctx, tx, id.UUID); err != nil {
		return crmcontracts.Activity{}, err
	}
	return readActivity(ctx, tx, id, archived)
}

// GetActivityContent is GetActivity for a caller about to USE the content —
// draft a reply, quote the thread — rather than show the row: a limited
// conversation the caller may discover answers ErrNotFound here, never a row
// with its text blanked that a model would then write a reply to.
func (s *Store) GetActivityContent(ctx context.Context, id ids.ActivityID, archived storekit.ArchivedFilter) (crmcontracts.Activity, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return crmcontracts.Activity{}, err
	}
	var out crmcontracts.Activity
	err := s.tx(ctx, func(tx pgx.Tx) (err error) {
		out, err = readActivityContent(ctx, tx, id, archived)
		return err
	})
	return out, err
}

// InvalidAudienceError is a malformed audience write: an unknown audience, a
// member set the contract does not admit, or a subject that does not exist.
type InvalidAudienceError struct {
	Audience string
	Reason   string
}

func (e *InvalidAudienceError) Error() string {
	if e.Reason == "" {
		return "activities: invalid audience " + e.Audience
	}
	return "activities: invalid audience write: " + e.Reason
}

// FieldFault answers the malformed audience write as a 422 on the audience
// field.
func (e *InvalidAudienceError) FieldFault() (field, code, message string) {
	return "audience", "invalid_audience", e.Error()
}

// refuseCapturedAudienceWrite stops a direct audience write on a message a
// mailbox brought in.
//
// The test is whether any seat has an import row: that is what makes the
// audience derived rather than declared. A row with none was hand-logged, and
// its audience is exactly what somebody set.
func refuseCapturedAudienceWrite(ctx context.Context, tx pgx.Tx, id ids.ActivityID) error {
	var imported bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM capture_import WHERE activity_id = $1)`,
		id.UUID).Scan(&imported); err != nil {
		return fmt.Errorf("activities: reading whether a message was imported: %w", err)
	}
	if imported {
		return &CapturedAudienceError{}
	}
	return nil
}

// CapturedAudienceError refuses a direct audience write on captured mail and
// says where the decision belongs instead.
type CapturedAudienceError struct{}

func (e *CapturedAudienceError) Error() string {
	return "activities: a captured message's audience is derived from its importers; " +
		"share or hold the thread instead"
}

// FieldFault maps the refusal onto the wire.
func (e *CapturedAudienceError) FieldFault() (field, code, message string) {
	return "audience", "audience_is_derived",
		"This message came from a mailbox, so who can read it follows from what each " +
			"importing mailbox asks for. Share or keep the thread private instead."
}
