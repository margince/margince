// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// WHO MAY SEE A COMPANY, and the two rules the column needs that no other
// field on the company patch does.
//
// The column is the contact column's twin and gets the contact column's
// treatment. Capture mints a company owner-scoped from a message nothing has
// judged yet, and until this field became writable the only way out was a
// sender verdict — a machine deciding who may read a record, with no door for
// the human it belongs to. contactvisibility.go states the reasoning in full;
// this file is the same reasoning applied to the other record, and the two are
// deliberately the same shape so a reader of one recognises the other.
//
// Ordinary in who may write it; not ordinary in what a wrong write costs. An
// industry written over a stale read is a wrong industry. This column decides
// who may read the record, so it carries a staleness check every other column
// tolerates going without, and refuses a result no seat could read.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// refuseStaleCompanyVisibility refuses a visibility write built on a row that
// has moved since it was read.
//
// updateCompanyInTx reads the company, decides admission, and only then locks
// the row — an unconditional patch takes its FOR UPDATE inside ApplyGuarded
// (storekit.LockRow). Every other column tolerates that window: last-write-wins
// over an industry is a wrong industry. This one is different. A colleague
// whose PATCH was admitted while the company was public can land `workspace`
// after its owner has just made it private, RE-PUBLISHING a record somebody
// deliberately closed, and the audit records `workspace -> workspace` because
// the before-image came from the stale read.
//
// So this LOCKS the row and then asks both questions again against it.
//
// Comparing the column to what the caller was shown is not enough on its own,
// and the reason is worth stating because the check looks complete without it.
// auth.EnsureWritable runs at the top of updateCompanyInTx, before the name
// lock and before the row is read. Under READ COMMITTED a privatization
// committing in that window is INVISIBLE to the comparison: the caller's own
// `current` is read after the commit too, so both sides say 'owner', they
// match, and a colleague who may no longer see the record at all is admitted
// to re-publish it. Driven as a real interleaving, that is exactly what
// happened — the guard passed and the write would have landed.
//
// So authorization is re-asked HERE, under the lock, where the answer cannot
// go stale before the write. A caller the row no longer admits gets the
// existence-hiding refusal EnsureWritable gives, not a conflict: which of the
// two they get must not disclose that the record still exists.
//
// The column comparison stays, for the case it does answer: the caller may
// still write the row, and is simply writing over an answer they never saw.
// That one is a conflict. If-Match makes the whole question moot — that path
// never reaches here — and the header panel sends one; this protects every
// OTHER client.
func refuseStaleCompanyVisibility(
	ctx context.Context, tx pgx.Tx, id ids.CompanyID, current crmcontracts.Company,
) error {
	var live string
	if err := tx.QueryRow(ctx,
		`SELECT visibility FROM company WHERE id = $1 FOR UPDATE`, id).Scan(&live); err != nil {
		return fmt.Errorf("contacts: re-reading company visibility under the row lock: %w", err)
	}
	// Ordered before the comparison on purpose: a caller the locked row does
	// not admit must be refused as though it were not there, whatever the
	// column says. Answering "conflict" first would tell them it exists.
	if err := auth.EnsureWritable(ctx, tx, "company", id.UUID); err != nil {
		return err
	}
	was := ""
	if current.Visibility != nil {
		was = string(*current.Visibility)
	}
	if live != was {
		return apperrors.ErrConflict
	}
	return nil
}

// refuseUnreadableCompany refuses a patch whose result no seat could read.
//
// The row-scope arm for a reader reads the pair as
// `visibility <> 'owner' OR owner_id = me`, so a row that says 'owner' and
// names nobody satisfies neither side and is invisible to EVERY seat — its
// author, an admin, and any later attempt to repair it through this same
// endpoint. That is a record destroyed rather than a record made private, and
// it is reachable two ways: an unbounded caller narrowing a row that already
// has no owner, and any owner sending
// `{"visibility":"owner","owner_id":null}`, since owner_id is clearable.
//
// Refused rather than repaired by naming the caller. Whose company it becomes
// is a decision, and quietly answering it for somebody would hand the row to
// whoever happened to press the button.
//
// The database holds the same invariant in company_owner_private_names_its_owner,
// and that constraint is the enforcement. This is the refusal the constraint
// cannot give: a check violation reaches a client as a server fault naming a
// constraint, where this names the field the caller has to supply.
func refuseUnreadableCompany(current crmcontracts.Company, in UpdateCompanyInput) error {
	private := current.Visibility != nil && string(*current.Visibility) == visibilityOwner
	if in.Visibility != nil {
		private = *in.Visibility == visibilityOwner
	}
	if !private {
		return nil
	}
	named := current.OwnerId != nil
	if in.OwnerID != nil {
		named = true
	}
	for _, cleared := range in.Clear {
		if cleared == filterOwnerID {
			named = false
		}
	}
	if !named {
		return &RequiredFieldError{Field: filterOwnerID}
	}
	return nil
}
