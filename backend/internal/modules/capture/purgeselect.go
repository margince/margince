// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Which messages an owner's purge may destroy, and which it may only let go of.
//
// A message is stored once however many mailboxes received it, so "delete what
// my connection brought in" cannot mean "delete every message matching the
// rule". A colleague who also imported it has their own claim on the same row,
// and destroying it would take their correspondence away to satisfy somebody
// else's rule.
//
// So the purge splits its subject in two: the messages only THIS seat imported,
// which are destroyed outright, and the messages somebody else also imported,
// where this seat's own contribution goes and the row stays.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// StatutoryFloor carries the shield every destructive activity path in this
// installation applies, handed in by the compose seam.
//
// Capture cannot import privacy, and the predicate lives over there — a second
// copy is how one destructive path quietly stops shielding what the others do.
// So it travels: the seam reads it from privacy and passes it here.
//
// Held by: TestTheStatutoryFloorIsSpelledOnce (backend/gates/statutoryfloorsingle_test.go).
type StatutoryFloor struct {
	// Clause filters an activity aliased `a`, in the positive form: it is TRUE
	// for a row the law still requires the installation to keep.
	Clause func(intervalArg, anchorArg int) string
	// Interval is the retention period, as a SQL interval literal; Anchor says
	// whether the window runs from the end of the calendar year.
	Interval string
	Anchor   bool
}

// column renders the shield as a boolean expression and appends its two
// arguments, returning where they landed.
func (f StatutoryFloor) column(used int, args []any) (string, []any) {
	if f.Clause == nil {
		// No floor supplied. Shield EVERYTHING rather than nothing: a purge
		// that cannot ask what the law requires must not guess that the answer
		// is "nothing", because that guess destroys correspondence.
		return "true", args
	}
	return f.Clause(used+1, used+2), append(args, f.Interval, f.Anchor)
}

// PurgeSubject is what a purge found: what it will destroy, and what it will
// only release.
type PurgeSubject struct {
	// SoleImports are the activities whose only capture_import row is this
	// seat's. Nobody else's claim survives them, so they are destroyed.
	SoleImports []ids.UUID
	// SharedImports are the activities a colleague also imported. This seat's
	// import row and participant row go; the message stays, for them.
	SharedImports []ids.UUID
	// Restricted are the activities under a statutory hold or an open erasure
	// request. They survive both arms and are reported, because a purge that
	// silently skipped them would tell an owner their mail is gone when it is
	// not.
	Restricted []ids.UUID
}

// Total is how many messages the rule matched at all.
func (s PurgeSubject) Total() int {
	return len(s.SoleImports) + len(s.SharedImports) + len(s.Restricted)
}

// SelectPurgeSubjectTx finds what a purge of one exclusion rule would touch,
// for one seat.
//
// The rule is matched the way every other capture rule is matched — an exact
// address, or a domain covering its subdomains — so an owner who excluded
// `studiolegal.de` reaches `mail.studiolegal.de` here exactly as the hold and
// the ingress gate do.
//
// Read-only. A preview and the purge itself call this with the same arguments
// and get the same answer, which is what makes the preview's counts honest.
func SelectPurgeSubjectTx(
	ctx context.Context, tx pgx.Tx, user ids.UUID, kind, value string, floor StatutoryFloor,
) (PurgeSubject, error) {
	var subject PurgeSubject
	if user == ids.Nil || value == "" {
		return subject, nil
	}
	match, args := purgeMatchClause(user, kind, value)
	// The statutory shield, computed as its OWN column rather than as a filter.
	// A row inside the retention window is not destroyed and not released — it
	// is reported, because an owner told their mail is gone must not find it
	// still there, and a rule that silently kept some of its matches would be
	// exactly that.
	shielded, args := floor.column(len(args), args)
	rows, err := tx.Query(ctx, `
		SELECT a.id,
		       (a.restricted_at IS NOT NULL OR (`+shielded+`)) AS withheld,
		       (SELECT count(*) FROM capture_import o WHERE o.activity_id = a.id) AS importers
		  FROM activity a
		  JOIN capture_import i ON i.activity_id = a.id AND i.user_id = $1
		 WHERE `+match+`
		 ORDER BY a.id`, args...)
	if err != nil {
		return subject, fmt.Errorf("capture: selecting what a purge would destroy: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id ids.UUID
		var withheld bool
		var importers int
		if err := rows.Scan(&id, &withheld, &importers); err != nil {
			return subject, fmt.Errorf("capture: selecting what a purge would destroy: %w", err)
		}
		switch {
		case withheld:
			// A statutory hold, or commercial correspondence still inside its
			// legal retention window. Neither destroyed nor released: the row
			// is an obligation the installation owes somebody else, and an
			// owner's rule does not outrank the law.
			subject.Restricted = append(subject.Restricted, id)
		case importers > 1:
			subject.SharedImports = append(subject.SharedImports, id)
		default:
			subject.SoleImports = append(subject.SoleImports, id)
		}
	}
	if err := rows.Err(); err != nil {
		return subject, fmt.Errorf("capture: selecting what a purge would destroy: %w", err)
	}
	return subject, nil
}

// purgeMatchClause builds the address-or-domain match, in the shape every other
// capture rule uses: an address matches exactly, a domain matches itself and
// everything under it.
func purgeMatchClause(user ids.UUID, kind, value string) (string, []any) {
	if kind == ExclusionKindDomain {
		// ESCAPE '\' and the value escaped to match: a rule value reaches a
		// LIKE pattern here, and `%` or `_` inside one matches far more than
		// the domain somebody named. ValidOwnDomain refuses both characters
		// today, so this escapes nothing in practice — it is here because the
		// safety would otherwise be a property of a validator in another file,
		// and a purge is not the place to inherit that quietly.
		//
		// The leading `@` and `.` are what keep `example.test` from matching
		// `evil-example.test`: an address either ends at the domain or at a
		// subdomain of it, never mid-label.
		return `(a.counterparty_email LIKE '%@' || $2 ESCAPE '\'
		      OR a.counterparty_email LIKE '%.' || $2 ESCAPE '\')`,
			[]any{user, escapeLikeValue(value)}
	}
	// An address matches exactly, so no pattern and nothing to escape. A `%`
	// in a local part is legal and stays a literal here.
	return `a.counterparty_email = $2`, []any{user, value}
}

// escapeLikeValue neutralises the LIKE metacharacters, so a rule value is
// matched as the text somebody typed.
func escapeLikeValue(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "%", `\%`)
	return strings.ReplaceAll(value, "_", `\_`)
}

// ReleaseImportTx drops this seat's claim on a message a colleague also
// imported: the import row and the participant row that made it readable.
//
// The message itself is untouched. What the owner asked for was that THEIR
// mailbox stop bringing this correspondent in, and a colleague who received the
// same message keeps their own copy of the answer.
func ReleaseImportTx(ctx context.Context, tx pgx.Tx, activityID, user ids.UUID) error {
	if _, err := tx.Exec(ctx,
		`DELETE FROM capture_import WHERE activity_id = $1 AND user_id = $2`, activityID, user); err != nil {
		return fmt.Errorf("capture: releasing a seat's import of a shared message: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM activity_participant WHERE activity_id = $1 AND user_id = $2`, activityID, user); err != nil {
		return fmt.Errorf("capture: releasing a seat's participation in a shared message: %w", err)
	}
	return nil
}

// SelectPurgeablePeopleTx finds the people a purge may anonymise: those this
// seat's capture minted from the mail being destroyed, whom nothing else holds.
//
// "Nothing else holds them" is the whole rule, and it is deliberately
// conservative. A person the owner later linked to a deal, gave a second
// address outside the rule, or that a colleague's own mailbox also produced is
// somebody the CRM has an independent reason to know — destroying that record
// to satisfy one seat's mailbox rule takes away work somebody did.
//
// Anonymised rather than deleted, through the retention module's own
// person/anonymize action: the row survives with its identifying columns gone,
// because deleting it would cascade into records that legitimately reference it
// and leave a colleague's deal pointing at nothing.
func SelectPurgeablePeopleTx(
	ctx context.Context, tx pgx.Tx, user ids.UUID, kind, value string,
) ([]ids.UUID, error) {
	if user == ids.Nil || value == "" {
		return nil, nil
	}
	// The rule is applied to the PERSON's addresses here rather than to an
	// activity's, so the two arms below read the same column and the match is
	// written once for both.
	match, args := purgeMatchClause(user, kind, value)
	match = strings.ReplaceAll(match, "a.counterparty_email", "pe.email")
	outside := strings.ReplaceAll(match, "pe.email", "other.email")
	// Four conditions, and every one of them is somebody else's claim on this
	// record:
	//
	//   a second live address outside the rule — the contact is more than what
	//     the rule describes;
	//   a deal — they are a stakeholder on one, or their mail is filed against
	//     one, either way somebody did work against this person;
	//   another seat's import of their mail — a colleague knows them
	//     independently of this mailbox;
	//   no import of this seat's own — then this seat's mail is not why the
	//     record exists, and the rule has no standing over it.
	//
	// "capture minted them" is asked through the import row, not through
	// person.source: source carries the PROVIDER name (gmail, outlook), so a
	// filter on the literal 'capture' would match nothing and silently
	// anonymise no one.
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT p.id
		  FROM person p
		  JOIN person_email pe ON pe.person_id = p.id AND pe.archived_at IS NULL
		 WHERE `+match+`
		   AND p.archived_at IS NULL
		   AND EXISTS (
		     SELECT 1 FROM activity a
		       JOIN capture_import i ON i.activity_id = a.id AND i.user_id = $1
		      WHERE lower(a.counterparty_email) = lower(pe.email))
		   AND NOT EXISTS (
		     SELECT 1 FROM person_email other
		      WHERE other.person_id = p.id AND other.archived_at IS NULL
		        AND NOT (`+outside+`))
		   AND NOT EXISTS (
		     SELECT 1 FROM relationship r
		      WHERE r.person_id = p.id AND r.kind = 'deal_stakeholder'
		        AND r.archived_at IS NULL)
		   AND NOT EXISTS (
		     SELECT 1 FROM activity_link pl
		       JOIN activity_link dl ON dl.activity_id = pl.activity_id
		                            AND dl.entity_type = 'deal'
		      WHERE pl.person_id = p.id AND pl.entity_type = 'person')
		   AND NOT EXISTS (
		     SELECT 1 FROM capture_import mine
		       JOIN activity ma ON ma.id = mine.activity_id
		      WHERE lower(ma.counterparty_email) = lower(pe.email)
		        AND mine.user_id <> $1)
		 ORDER BY p.id`, args...)
	if err != nil {
		return nil, fmt.Errorf("capture: selecting the people a purge may anonymise: %w", err)
	}
	defer rows.Close()
	var people []ids.UUID
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("capture: selecting the people a purge may anonymise: %w", err)
		}
		people = append(people, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("capture: selecting the people a purge may anonymise: %w", err)
	}
	return people, nil
}
