// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

// The activity anchor: preparing for a meeting by DEREFERENCING it.
//
// An activity is still a link, not a thing links hang off — graph.go's walk is
// unchanged and the graph keeps exactly the anchors it had. Naming an activity
// as the anchor asks a different question: which records is this event about?
// The event answers from its own links and participants, ONE of those records
// becomes the subject, and the ordinary record walk runs around that.
//
// The answer says what it chose and what it did not. `prepared_for` names the
// subject the walk used, `also_present` names every other record the event
// resolved to, and `unresolved_attendees` names the addresses that matched
// nobody — so an empty prep is actionable ("this event names nobody we hold,
// and here is who was on it") rather than silent.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// assembleActivityWithin builds the context for an activity anchor.
func (s *Store) assembleActivityWithin(ctx context.Context, tx pgx.Tx, activityID ids.UUID, maxItems int, within projectScope) ([]graphSection, error) {
	profile, err := activityProfile(ctx, tx, activityID, within)
	if err != nil {
		return nil, err
	}
	subjects, err := activitySubjects(ctx, tx, activityID)
	if err != nil {
		return nil, err
	}
	unresolved, err := unresolvedAttendees(ctx, tx, activityID)
	if err != nil {
		return nil, err
	}

	sections := []graphSection{{name: sectionProfile, items: []graphItem{profile}}}
	if len(subjects) > 0 {
		sections = append(sections, graphSection{
			name: "prepared_for", items: []graphItem{subjectItem(subjects[0])},
		})
		// max_items bounds this exactly as it bounds every other section, and
		// the order makes a cut survivable: the next-best subject first. The
		// contract states one per-section cap for the whole response, and a
		// section that quietly ignored it would be the surprise — a caller
		// preparing for a large meeting raises max_items rather than
		// discovering which sections opted out.
		if also := subjectItems(subjects[1:], maxItems); len(also) > 0 {
			sections = append(sections, graphSection{name: "also_present", items: also})
		}
	}
	if len(unresolved) > maxItems {
		unresolved = unresolved[:maxItems] // bounded like the rest; organizer first
	}
	if len(unresolved) > 0 {
		sections = append(sections, graphSection{name: "unresolved_attendees", items: unresolved})
	}
	if len(subjects) == 0 {
		return sections, nil
	}

	walk, err := s.assembleRecordWithin(ctx, tx, subjects[0].entityType, subjects[0].id, maxItems, within)
	if err != nil {
		return nil, err
	}
	// The event's own profile already opened the answer, and prepared_for
	// already names the subject; the subject's profile section would repeat it
	// under a heading that reads as the meeting's.
	for _, section := range walk {
		if section.name != sectionProfile {
			sections = append(sections, section)
		}
	}
	return sections, nil
}

// activityProfile is the existence and visibility gate for the whole assembly:
// an event the caller cannot see yields the same not-found any other anchor
// gives, never a leak of who was in someone else's meeting.
//
// EnsureActivityContentVisibleLive, not EnsureActivityContentVisible: this serves stored
// content, so an archived event must not answer and an unbounded actor does
// not skip the existence probe.
//
// The project scope is applied HERE, to the anchor itself, and that is what
// keeps it off the subjects and attendees: an event filed under another
// project is outside the scoped picture entirely, so it answers the same
// not-found an invisible one does, before a participant or a linked record
// is read. Filtering the walk around it while still naming the room and who
// was in it would hand over the other engagement under a scope that claims
// to have excluded it.
func activityProfile(ctx context.Context, tx pgx.Tx, activityID ids.UUID, within projectScope) (graphItem, error) {
	// Object RBAC before row scope: a caller with no read grant on activity at
	// all is denied the type (403), not handed the subset of events their row
	// scope would have admitted.
	if err := auth.Require(ctx, string(datasource.EntityActivity), principal.ActionRead); err != nil {
		return graphItem{}, err
	}
	if err := auth.EnsureActivityContentVisibleLive(ctx, tx, activityID); err != nil {
		return graphItem{}, err
	}
	var title, kind string
	var occurredAt time.Time
	args := []any{activityID}
	arg := func(v any) int { args = append(args, v); return len(args) }
	filed := ""
	if clause := within.clause("a", arg); clause != "" {
		filed = " AND " + clause
	}
	err := tx.QueryRow(ctx, `
		SELECT coalesce(a.subject, a.channel_provider, a.kind), a.kind, a.occurred_at
		  FROM activity a WHERE a.id = $1 AND a.archived_at IS NULL`+filed, args...).
		Scan(&title, &kind, &occurredAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return graphItem{}, apperrors.ErrNotFound
		}
		return graphItem{}, fmt.Errorf("search: reading the event a prep is anchored on: %w", err)
	}
	// When it happens is half of what a prep is for, and the title alone does
	// not carry it — a subject line reads the same the day before and the week
	// after.
	return graphItem{
		entityType: string(datasource.EntityActivity), id: activityID,
		summary: fmt.Sprintf("%s — %s on %s", title, kind, occurredAt.UTC().Format(time.RFC3339)),
		// Also as a FIELD, not only inside the sentence. The prep's anchor is
		// the most date-sensitive item it has, and a reader told to prefer the
		// structured date would read its absence as "not an event".
		occurredAt: occurredAt,
	}, nil
}

// unresolvedAttendees reads the addresses on the event that matched no record.
//
// This is a deliberate disclosure, not a leak. The addresses are content of an
// event the caller has already been admitted to read, and returning them is
// what makes an empty prep actionable: an agent holding them can call
// resolve_entities, where withholding them would answer a prep with silence.
//
// The items carry the EVENT as their ref, because an attendee we hold no
// record for has no id of their own to name — the ref says where the address
// came from, and the summary is the address and the part they played.
//
// A party matched to a person the caller cannot see is neither here nor a
// subject: it resolved to a record, and reclassifying it as an unmatched
// address would disclose by the back door exactly what the row scope withheld.
// Colleagues (user_id) are likewise absent — they resolved to a member.
func unresolvedAttendees(ctx context.Context, tx pgx.Tx, activityID ids.UUID) ([]graphItem, error) {
	// DISTINCT ON the address, not on (address, role): one party copied as both
	// `to` and `cc` is one person in the room, and listing them twice reads as
	// two. The role kept is the most significant one they held, which is also
	// the order the window is cut by.
	rows, err := tx.Query(ctx, `
		SELECT address, role FROM (
		    SELECT DISTINCT ON (ap.address) ap.address, ap.role, `+participantRoleOrder("ap")+` AS rank
		      FROM activity_participant ap
		     WHERE ap.activity_id = $1
		       AND ap.person_id IS NULL AND ap.user_id IS NULL AND ap.address IS NOT NULL
		     ORDER BY ap.address, rank
		) parties
		 ORDER BY rank, address LIMIT $2`, activityID, graphExpansionLimit)
	if err != nil {
		return nil, fmt.Errorf("search: reading the addresses on an event that matched nobody: %w", err)
	}
	defer rows.Close()
	var out []graphItem
	for rows.Next() {
		var address, role string
		if err := rows.Scan(&address, &role); err != nil {
			return nil, err
		}
		out = append(out, graphItem{
			entityType: string(datasource.EntityActivity), id: activityID,
			summary: fmt.Sprintf("%s — %s", address, role),
		})
	}
	return out, rows.Err()
}
