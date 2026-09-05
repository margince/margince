// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The colleague roster: who works here, as opposed to who we sell to.
//
// SeatNames beside this answers "what is this id called" for ids a caller
// already holds. This answers the question that comes first — WHICH id — and
// nothing could ask it before: app_user appeared nowhere on the tool surface,
// so an assistant asked to assign work searched `person`, found a customer
// contact with a similar name, and offered that. The distinction between a
// colleague and a contact was missing as a concept, not merely as a lookup.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// colleagueCap bounds one answer. A roster is tens of people, not thousands,
// and a caller wanting a particular one narrows with q rather than paging —
// so this is a ceiling that says "narrow it", never a page boundary a reader
// has to walk.
const colleagueCap = 200

// Colleague is one seat: who they are, and whether they can still be given
// work. Nothing about permissions — what a colleague may do is a question for
// the operation they are named on, not for a roster.
type Colleague struct {
	UserID      ids.UUID
	DisplayName string
	Email       string
	SeatType    string
	// No Active field, and its absence is the answer: only a seat that can
	// actually receive work is listed at all. Suspended, deactivated and
	// locked-out seats are filtered in the query rather than reported with a
	// flag, because WHICH colleague is suspended is an admin's fact — the REST
	// roster honours `include_inactive` only for an admin caller, and a tool
	// any read passport can call must not be the way around that.
	// IsAgent marks a machine seat (the installation's own agent account). Named
	// rather than filtered out, because an assistant listing colleagues should
	// not silently pretend the agent seat does not exist — and must not offer
	// it as a person to give work to either.
	IsAgent bool
}

// Colleagues lists the installation's seats, newest-relevant first by name, with
// an optional case-insensitive filter over display name and email.
//
// Archived seats are absent: a person who has left is not a colleague, and
// naming one would offer work to an account that cannot receive it.
func (s *Service) Colleagues(ctx context.Context, q string) ([]Colleague, bool, error) {
	trimmed := strings.TrimSpace(q)
	// `%` and `_` are LIKE metacharacters, and the tool advertises this as a
	// plain narrowing filter — a caller typing an underscore in a name means
	// that character, not "any character".
	escaped := strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(trimmed)
	pattern := "%" + escaped + "%"
	var out []Colleague
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, display_name, email, seat_type, is_agent
			  FROM app_user
			 WHERE `+LiveMemberSQL("")+`
			   AND locked_until IS NULL
			   AND ($1 = '' OR display_name ILIKE $2 OR email ILIKE $2)
			 ORDER BY display_name, id
			 LIMIT $3`, trimmed, pattern, colleagueCap+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c Colleague
			if err := rows.Scan(&c.UserID, &c.DisplayName, &c.Email, &c.SeatType, &c.IsAgent); err != nil {
				return err
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, false, fmt.Errorf("identity: listing colleagues: %w", err)
	}
	// One over the cap was asked for, so a full page is distinguishable from a
	// truncated one. A roster that silently stopped at 200 would read as the
	// whole roster, and an assistant would report that a colleague does not
	// work here.
	if len(out) > colleagueCap {
		return out[:colleagueCap], true, nil
	}
	return out, false, nil
}

// ResolveColleague finds the ONE live seat a name refers to, or answers that it
// does not.
//
// For a writer that must decide whether to assign work to somebody a machine
// read out of a document. The bar is deliberately higher than the roster
// filter's, because the two answer different questions: Colleagues narrows a
// list a person is about to look at, and a near-miss there costs them a glance.
// This one hands a task to a seat, and a near-miss there gives somebody else's
// work to the wrong colleague — who then does not do it, because it was never
// theirs.
//
// So it resolves only an EXACT match, case- and space-insensitive, on the
// display name or the email. "Lena" does not resolve to "Lena Fischer": it is
// consistent with her, and with a Lena Bauer the installation may also employ,
// and a writer cannot tell which from the string alone.
//
// Two matches answer not-found, the same as none. A tie is not a weaker answer
// than an absence — it is the same absence of an answer, and picking the first
// row would make the outcome depend on an ORDER BY nobody chose for this.
//
// An AGENT seat never resolves. The installation's own machine account can be
// named in a transcript like anyone else, and giving it a person's promise
// would file the work where no person will see it.
//
// The truncation flag is honoured: a filter that hit the roster cap has not
// been shown its whole answer, so a "unique" match inside it is not known to be
// unique.
func (s *Service) ResolveColleague(ctx context.Context, name string) (Colleague, bool, error) {
	wanted := strings.ToLower(strings.TrimSpace(name))
	if wanted == "" {
		return Colleague{}, false, nil
	}
	candidates, truncated, err := s.Colleagues(ctx, wanted)
	if err != nil {
		return Colleague{}, false, err
	}
	if truncated {
		return Colleague{}, false, nil
	}
	var found Colleague
	var matches int
	for _, c := range candidates {
		if c.IsAgent {
			continue
		}
		if strings.ToLower(strings.TrimSpace(c.DisplayName)) == wanted ||
			strings.ToLower(strings.TrimSpace(c.Email)) == wanted {
			found, matches = c, matches+1
		}
	}
	if matches != 1 {
		return Colleague{}, false, nil
	}
	return found, true, nil
}
