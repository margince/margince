// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The person object's own write and collision paths.
//
// Split from csvwriters.go and csvcollision.go for the reason csvpersonfields.go
// is split from csvfields.go: a person's identity is an email held in a child
// table with estate-wide uniqueness behind it, so both paths answer differently
// from the company ones beside them. A duplicate company name creates a twin and
// files a review pair; a duplicate person email is REFUSED, because one is a real
// key and the other is not. Keeping the two apart is what stops either answer
// being read as the general rule.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// createPerson lands one row as a person, through the same CreatePersonTx seam
// the flip's own bulk person import uses — so the row runs the identity ladder,
// and the person and its identity-map row commit or roll back together.
//
// An address that already belongs to someone else is a SKIP with a reason, never
// an automatic merge: two records that look like one person is a question for a
// human, and answering it by writing is the one move no undo reverses.
func (w *csvWriters) createPerson(ctx context.Context, row migration.Row) (migration.EnsureResult, error) {
	in := personCreateFrom(textFields(row.Fields), w.provenanceOf(row.ExternalID))
	if in.FullName == "" {
		return migration.EnsureResult{Skipped: true, SkipReason: "the row carries neither a name nor an email, so it names no person"}, nil
	}
	err := w.land(ctx, row.ExternalID, func(tx pgx.Tx) (ids.UUID, error) {
		person, err := w.people.CreatePersonTx(ctx, tx, in)
		if err != nil {
			return ids.UUID{}, fmt.Errorf("import: creating person %s: %w", row.ExternalID, err)
		}
		return ids.UUID(person.Id), nil
	})
	var dup *people.DuplicateEmailError
	if errors.As(err, &dup) {
		// The same sentence the preview used, so an approval decided on
		// "skipped, the address is held" is not answered by different words.
		return migration.EnsureResult{Skipped: true, SkipReason: personEmailClaimedReason}, nil
	}
	if err != nil {
		return migration.EnsureResult{}, err
	}
	return migration.EnsureResult{Created: true}, nil
}

// personEmailIsClaimed answers whether the estate already holds this row's
// address, WITHOUT regard to who holds it.
//
// It is deliberately not visibility-filtered, and that is the opposite of the
// rule the collision check below follows. The reason is that the two answer
// different questions. A duplicate company NAME is a judgement — the commit
// creates a twin and files a review pair — so telling a caller about an
// incumbent they cannot see would disclose one. A duplicate email is not a
// judgement: uq_person_email_dedupe is estate-wide, so the commit REFUSES the
// row whoever holds the address, and a preview that promised a create would
// simply be wrong.
//
// So the preview reports the skip that will happen. What it must not do is say
// why, and it does not: the reason names the row's own address, which the caller
// supplied, and never the incumbent or their existence beyond the fact that this
// address is spoken for.
func (w *csvWriters) personEmailAlreadyHeld(ctx context.Context, row migration.Row) (bool, error) {
	if w.object != migration.ObjectPerson {
		return false, nil
	}
	email := strings.TrimSpace(textFields(row.Fields)[fieldEmail])
	if email == "" {
		return false, nil
	}
	var claimed bool
	if err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM person_email WHERE email = lower($1) AND archived_at IS NULL)`,
			email).Scan(&claimed)
	}); err != nil {
		return false, fmt.Errorf("import: checking whether %q is already held: %w", email, err)
	}
	return claimed, nil
}

// personEmailClaimedReason is what the report says for such a row. It names the
// address the file supplied and nothing else — not the incumbent, not their
// owner, not whether the caller could have seen them.
const personEmailClaimedReason = "this email address is already held in the CRM, so the row cannot create a second person under it"

// personCollides answers whether this row names someone the caller can already
// see, running the same ladder the create path runs.
//
// The visibility asymmetry with the organization arm is real and is not an
// oversight: DedupePerson answers ONE person, not a ranked set, so there is no
// candidate behind the winner for an invisible record to mask. The single match
// is the only row to ask about, and asking about it is the whole filter.
func (w *csvWriters) personCollides(ctx context.Context, row migration.Row) (bool, error) {
	fields := textFields(row.Fields)
	email := strings.TrimSpace(fields[fieldEmail])
	name := personFullName(fields)
	if email == "" && name == "" {
		return false, nil
	}
	var visible bool
	if err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
		// The workspace's own free-mail carve-outs, not the shipped baseline:
		// without them a company that has asserted its domain is not free mail
		// has its people scored as unrelated, and the preview then disagrees
		// with the commit — the one thing this check exists to prevent.
		//
		// Read through capture.MatcherTx, which is what wires the same list into
		// the people store (captureensurer.go). Going through the reader rather
		// than exporting a store accessor keeps every exported store method
		// RBAC-gated: this list is workspace configuration, not anyone's record.
		consumerMail, err := capture.MatcherTx(ctx, tx)
		if err != nil {
			return err
		}
		var emails []string
		if email != "" {
			emails = []string{email}
		}
		match, err := people.DedupePerson(ctx, tx, people.PersonCandidate{
			FullName: name, Emails: emails, ConsumerMail: consumerMail,
			// The preview writes nothing and routes nothing, so it has no
			// business filing a review pair for what it merely looked at.
			QueueNameCollisions: false,
		})
		if err != nil || match.Decision == people.DecisionNoMatch {
			return err
		}
		visible, err = auth.VisibleTo(ctx, tx, "person", match.PersonID.UUID)
		return err
	}); err != nil {
		named := email
		if named == "" {
			named = name
		}
		return false, fmt.Errorf("import: checking %q against the people already held: %w", named, err)
	}
	return visible, nil
}
