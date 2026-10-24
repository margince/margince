// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The contact rename re-check: the company re-check (renamerecheck.go) applied
// to contacts.
//
// A contact is checked for duplicates once, when it is created, with the name
// it has then. A contact captured from mail or imported without a name starts
// as its address ("sahner"), and the real name arrives later by an edit
// ("Gerhard Sahner"). That edit is the moment the duplicate becomes visible —
// the same contact already sat under another address — and nothing looked.
//
// Every write that changes full_name or the addresses re-runs the review lanes
// and files what they find. Unlike the company re-check it
// also runs for a human's edit: a human correcting a name is exactly who
// reveals the twin. It never merges and never overrules the rename; the pair
// goes on the review queue.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/freemail"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// contactRenameRecheckSource names this detector on the queue rows it files.
const contactRenameRecheckSource = "contact_rename_recheck"

// renamesAContact reports whether an update moves the contact's name. A re-sent
// spelling of the same name is no rename: it costs a scan and reveals nothing.
func renamesAContact(currentName string, in UpdateContactInput) bool {
	return in.FullName != nil && NormalizeContactName(*in.FullName) != NormalizeContactName(currentName)
}

// recheckIfEdited runs the re-check when an update moved the contact's name or
// replaced its addresses. A new address can reveal a twin too: the same mailbox
// under a plus tag, or a subdomain of the twin's employer.
//
// The lane locks were taken before the edit wrote anything
// (lockEditedContactLanes), so the re-check's own locks are already held.
func recheckIfEdited(ctx context.Context, tx pgx.Tx, id ids.ContactID, currentName string, in UpdateContactInput, emailsChanged bool) error {
	if !renamesAContact(currentName, in) && !emailsChanged {
		return nil
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	return recheckContactForDuplicates(ctx, tx, id, contactRenameRecheckSource, by)
}

// lockEditedContactLanes locks the contact's row, then the lanes for the
// addresses and the name it will hold, before the edit writes an address
// (lockorder.go). An edit that neither renames nor sends addresses takes none.
func lockEditedContactLanes(ctx context.Context, tx pgx.Tx, id ids.ContactID, current crmcontracts.Contact, in UpdateContactInput) error {
	if !renamesAContact(current.FullName, in) && in.Emails == nil {
		return nil
	}
	if _, err := storekit.LockRow(ctx, tx, "contact", id.UUID, storekit.LiveOnly); err != nil {
		return err
	}
	name := current.FullName
	if in.FullName != nil {
		name = *in.FullName
	}
	var emails []string
	if in.Emails != nil {
		for _, e := range in.Emails {
			emails = append(emails, e.Email)
		}
	} else if current.Emails != nil {
		for _, e := range *current.Emails {
			emails = append(emails, string(e.Email))
		}
	}
	return lockContactLanes(ctx, tx, emails, name)
}

// recheckContactForDuplicates scores an edited contact against the rest of the
// workspace and files the pairs the review lanes name (reviewContact). It takes
// the address-base and name locks in the create's order. So an edit and a
// create converging on one human cannot both read no incumbent.
//
// The contact's current primary employer counts as evidence. That lets an edge
// write find a same-name twin at that employer (fileEmployerTwins).
//
// The exact lanes are skipped: the contact holds its own addresses and phones,
// so they would name itself. A claimed address cannot arrive by an edit at all,
// because contact_email is unique on the address.
func recheckContactForDuplicates(ctx context.Context, tx pgx.Tx, id ids.ContactID, source, by string) error {
	subject, found, err := readContactSubject(ctx, tx, id)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("contacts: reading the edited contact: %w", pgx.ErrNoRows)
	}
	if err := lockContactLanes(ctx, tx, subject.emails, subject.name); err != nil {
		return err
	}
	_, err = subject.fileDuplicates(ctx, tx, nil, source, by)
	return err
}

// contactSubject is what the review lanes read about an existing contact.
type contactSubject struct {
	id     ids.ContactID
	name   string
	emails []string
}

// readContactSubject reads a live contact's name and live addresses, and
// reports whether the contact is still live.
func readContactSubject(ctx context.Context, tx pgx.Tx, id ids.ContactID) (contactSubject, bool, error) {
	subject := contactSubject{id: id}
	err := tx.QueryRow(ctx, `
		SELECT p.full_name,
		       coalesce((SELECT array_agg(e.email ORDER BY e.email) FROM contact_email e
		                  WHERE e.contact_id = p.id AND e.archived_at IS NULL), '{}')
		  FROM contact p
		 WHERE p.id = $1 AND p.archived_at IS NULL`, id).Scan(&subject.name, &subject.emails)
	if errors.Is(err, pgx.ErrNoRows) {
		return contactSubject{}, false, nil
	}
	if err != nil {
		return contactSubject{}, false, fmt.Errorf("contacts: reading the contact to re-check: %w", err)
	}
	return subject, true, nil
}

// fileDuplicates runs the review lanes for an existing contact, files every
// pair they name, and answers how many are new on the queue. A nil matcher is
// the shipped consumer-mail list.
func (s contactSubject) fileDuplicates(ctx context.Context, tx pgx.Tx, consumerMail *freemail.Matcher, source, by string) (int, error) {
	employer, _, err := readCurrentEmployer(ctx, tx, s.id.UUID)
	if err != nil {
		return 0, err
	}
	match, err := reviewContact(ctx, tx, ContactCandidate{
		FullName: s.name, Emails: s.emails, ConsumerMail: consumerMail, QueueNameCollisions: true, ExcludeID: &s.id,
		CurrentPrimaryCompanyID: employer,
	})
	if err != nil {
		return 0, err
	}
	return match.recordIfReview(ctx, tx, s.id, s.name, source, by)
}
