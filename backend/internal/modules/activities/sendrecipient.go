// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// An account-started send names its own addressees, and a reply inherits
// them. A new address gets a contact first, through RecipientFiler. An address
// that still names nobody the sender can see is refused. The refusal is a
// FIELD fault naming it, so the composer can point at it.

import (
	"context"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

// RecipientDirectory answers whether an address belongs to a contact or lead
// the caller may read. The contacts capability implements it and compose injects
// it, because a module never imports a sibling.
//
// It answers about VISIBILITY, not existence: an address on a contact the
// caller's row scope excludes is answered the same as one on nobody at all.
// Telling the two apart would leak the existence of a record through a
// composer field, which is the disclosure the row-scope gate exists to
// prevent everywhere else.
type RecipientDirectory interface {
	// VisibleAddresses returns the subset of addresses that belong to a
	// contact or lead this caller can read, normalized lowercase.
	VisibleAddresses(ctx context.Context, tx pgx.Tx, addresses []string) (map[string]bool, error)
}

// WithRecipientDirectory returns a store whose account-started sends resolve
// their addressees. A store without one refuses an account-started send
// rather than mailing an address nobody is accountable for.
func (s *Store) WithRecipientDirectory(dir RecipientDirectory) *Store {
	clone := *s
	clone.recipients = dir
	return &clone
}

// RecipientFiler makes a minimal contact for each typed address no live
// contact or lead holds. The caller owns it. It returns the addresses it filed.
// Contacts implements it and compose injects it.
//
// It writes on the caller's transaction, so a preview can file, ask the
// consent engine and roll both back.
type RecipientFiler interface {
	FileTypedRecipientsTx(ctx context.Context, tx pgx.Tx, addresses []string) ([]string, error)
}

// WithRecipientFiler returns a store whose account-started sends and previews
// file a new address as a contact before the consent engine is asked.
func (s *Store) WithRecipientFiler(filer RecipientFiler) *Store {
	clone := *s
	clone.recipientFiler = filer
	return &clone
}

// fileTypedRecipients commits a contact for each new address before the send
// is staged. The consent engine and the dispatcher then read a recipient the
// record names. PrepareSend calls it after the sender's own checks pass.
func (s *Store) fileTypedRecipients(ctx context.Context, origin SendOrigin, recipients []string) error {
	if origin.isReply() || s.recipientFiler == nil {
		return nil
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		_, err := s.recipientFiler.FileTypedRecipientsTx(ctx, tx, recipients)
		return err
	})
}

// UnresolvedRecipientError refuses an account-started send that names an
// address belonging to no contact the sender can see.
//
// It carries ONE address even when several failed, because the composer
// fixes them one at a time and the first unresolved address is the one the
// rep is looking at. The count travels beside it so the message can say
// there are more without listing addresses the caller may not be entitled
// to reason about.
type UnresolvedRecipientError struct {
	Address string
	More    int
}

func (e *UnresolvedRecipientError) Error() string {
	msg := "no contact or lead you can see has the address " + e.Address +
		", and it cannot be added as a new contact — pick a different recipient"
	switch {
	case e.More == 1:
		msg += " (and 1 other address like it)"
	case e.More > 1:
		msg += " (and " + strconv.Itoa(e.More) + " other addresses like it)"
	}
	return msg
}

// FieldFault names the address the caller must correct.
func (e *UnresolvedRecipientError) FieldFault() (field, code, message string) {
	return "to", "recipient_not_on_file", e.Error()
}

// NoRecipientDirectoryError refuses an account-started send on a surface
// wired without address resolution. Like errNoDeliveryStager it is a
// composition defect rather than a client-correctable condition, so it
// carries no FieldFault and must surface as the 500 it is — a caller told
// to fix their address list would be told something untrue.
type NoRecipientDirectoryError struct{}

func (e *NoRecipientDirectoryError) Error() string {
	return "activities: account-started send has no recipient directory wired"
}

// resolveRecipients refuses before anything is staged when an addressee
// belongs to nobody the sender can read.
//
// A REPLY skips this entirely, and that is the decision rather than an
// oversight: its addressees come from a conversation the workspace already
// captured, so an address on it is evidence of correspondence that happened
// whether or not anyone ever filed a contact for it. Refusing there would
// block answering mail the product itself put on the timeline.
func (s *Store) resolveRecipients(ctx context.Context, tx pgx.Tx, origin SendOrigin, recipients []string) error {
	if origin.isReply() {
		return nil
	}
	if s.recipients == nil {
		return &NoRecipientDirectoryError{}
	}
	visible, err := s.recipients.VisibleAddresses(ctx, tx, recipients)
	if err != nil {
		return err
	}
	var first string
	unresolved := 0
	for _, addr := range recipients {
		if normalized := strings.ToLower(strings.TrimSpace(addr)); !visible[normalized] {
			if first == "" {
				first = addr
			}
			unresolved++
		}
	}
	if unresolved > 0 {
		return &UnresolvedRecipientError{Address: first, More: unresolved - 1}
	}
	return nil
}
