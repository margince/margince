// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Naming who a reply is TO.
//
// A drafter that knows who is sending and not who is receiving greets the only
// name it has, which is the sender's — producing a message addressed to its own
// author. That is not hypothetical: the certification judge floored a draft for
// it, and it is the one defect holding the reply site back from certified.
//
// The name is not on the activity. It is on the contact the message was WITH,
// which activity_participant records with the role they held, so this reads the
// participants rather than inventing a second notion of "who this was with".

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/mailrole"
	"github.com/margince/margince/backend/internal/shared/kernel/contactaddress"
	"github.com/margince/margince/backend/internal/shared/kernel/greetingregister"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ReplyRecipient is who a reply to an activity is addressed to.
//
// Every field may be empty, and that is an answer rather than a failure: an
// activity linked to no contact, or to one this caller cannot read, leaves the
// drafter with no name — and a draft that opens "Hallo," is correct there,
// where one that guesses is not.
type ReplyRecipient struct {
	// FullName as recorded, for the drafter to greet by.
	FullName string
	// FirstName is what a familiar greeting uses. Split here rather than in
	// the prompt: a model asked to shorten a name will shorten
	// "Dr. Anne-Marie Weiß-Konrad" differently on every call.
	FirstName string
	// LastName is what a FORMAL greeting uses, and the two are not
	// interchangeable. German is the case that shows it: "Sehr geehrter Herr"
	// takes a surname, and a model given only a first name either drops the
	// register or invents the missing half — "Sehr geehrte Frau/Herr Dietmar"
	// is what that looks like in a draft a rep was about to send.
	//
	// Empty where the record holds no surname, which is an answer: the greeting
	// falls back to the familiar form rather than to a guess.
	LastName string
	// GreetingRegister is the register a rep stored on this contact, empty when
	// none is set.
	GreetingRegister greetingregister.Register
	// WrittenAbout marks a row that records work about a contact rather than
	// correspondence with one: its To comes from FiledAddress, never the participants.
	WrittenAbout bool
	// FiledAddress is the reachable address of a written-about row's one readable linked
	// contact, never a seat's or a shared mailbox's. Several linked contacts leave it empty.
	FiledAddress string
}

// GreetingNames are the first name (or full name when there is none) and the
// surname a greeting uses.
func (r ReplyRecipient) GreetingNames() (greeting, surname string) {
	if r.FirstName != "" {
		return r.FirstName, r.LastName
	}
	return r.FullName, r.LastName
}

// ReplyRecipientFor names the contact a reply to this activity is delivered to.
//
// It asks deliveryOf who that is, so name, surname, register and address come
// from one ranking and one own-domain filter. The contact is read again under
// the caller's row scope. An activity the caller cannot reach and a contact they
// cannot read both answer the empty recipient, and no name leaks.
//
// One contact, not a list. A row that carries no participants at all, such as a
// note, falls back to the contact it is filed on. A message whose delivered-to
// participant is unknown or unreadable greets nobody: a copied contact never
// stands in for it. colleague is the same predicate ReplyAddresseeFor takes.
func (s *Store) ReplyRecipientFor(ctx context.Context, id ids.ActivityID, colleague func(address string) bool) (ReplyRecipient, error) {
	// Naming a contact is a contact read, so it takes the contact read GRANT as
	// well as the row scope. A caller permitted to read activities but not
	// contacts would otherwise be told a name through this door that the contacts
	// surface refuses them.
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return ReplyRecipient{}, err
	}
	delivered, err := s.deliveryOf(ctx, id, colleague)
	if _, none := errors.AsType[*NoReplyAddressError](err); err != nil && !none {
		return ReplyRecipient{}, err
	}

	var (
		out     ReplyRecipient
		written bool
		target  EmailTarget
	)
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		activity, err := readActivityContent(ctx, tx, id, storekit.LiveOnly)
		if err != nil {
			return err
		}
		if written, target, err = writtenAboutTarget(ctx, tx, id, activity.Kind); err != nil {
			return err
		}
		var address string
		out, address, err = greetingContact(ctx, tx, id, delivered.Counterparty)
		if err != nil {
			return err
		}
		// A shared mailbox has a record and no contact behind it, and its display
		// name splits into a first name like any other. It is cleared entirely,
		// because a half-cleared recipient would greet "steireif Partnernet,".
		if mailrole.GreetsNobody(out.FullName, address) {
			out = ReplyRecipient{}
		}
		return nil
	})
	if err != nil {
		return ReplyRecipient{}, err
	}
	out.WrittenAbout = written
	if target.offered() {
		out.FiledAddress = target.Address
	}
	// A lead has no contact record to greet by, so its own name is used.
	if out.FullName == "" && target.EntityType == crmcontracts.ActivityEmailTargetEntityTypeLead && target.offered() {
		out.FullName = target.Name
	}
	return out, nil
}

// greetingContact reads the names, register and reachable address of the contact
// a reply is greeted by. The row scope sits in the same statement as the
// archived test, so a contact flipped to private between two reads is not named.
// delivered is the counterparty deliveryOf resolved. Without one, only an
// activity with no participants falls back to the contact it is filed on.
func greetingContact(ctx context.Context, tx pgx.Tx, id ids.ActivityID, delivered string) (ReplyRecipient, string, error) {
	var args []any
	arg := func(v any) int {
		args = append(args, v)
		return len(args)
	}
	scope, err := auth.ScopeClauseFor(ctx, "contact", "p", arg)
	if err != nil {
		return ReplyRecipient{}, "", err
	}
	q := `
		SELECT p.full_name, coalesce(p.first_name, ''), coalesce(p.last_name, ''),
		       coalesce(p.greeting_register, ''),
		       coalesce((SELECT pe.email
		                   FROM contact_email pe
		                  WHERE pe.contact_id = p.id AND pe.archived_at IS NULL` +
		contactaddress.ReachableOrder + `
		                  LIMIT 1), '')
		  FROM contact p`
	if delivered != "" {
		q += fmt.Sprintf(` WHERE p.id = $%d AND p.archived_at IS NULL`, arg(delivered))
	} else {
		activity := arg(id)
		q += fmt.Sprintf(`
		  JOIN activity_link l ON l.contact_id = p.id AND l.activity_id = $%[1]d
		 WHERE p.archived_at IS NULL
		   AND NOT EXISTS (SELECT 1 FROM activity_participant ap WHERE ap.activity_id = $%[1]d)`, activity)
	}
	if scope != "" {
		q += ` AND (` + scope + `)`
	}
	if delivered == "" {
		q += ` ORDER BY l.created_at, l.id`
	}
	q += ` LIMIT 1`

	var out ReplyRecipient
	var address string
	err = tx.QueryRow(ctx, q, args...).Scan(&out.FullName, &out.FirstName, &out.LastName, &out.GreetingRegister, &address)
	if errors.Is(err, pgx.ErrNoRows) {
		// Linked to nobody, or to a contact out of scope: no name, which the
		// floor renders as an unnamed greeting rather than a refused draft.
		return ReplyRecipient{}, "", nil
	}
	return out, address, err
}

// writtenAboutTarget says whether a reply to this kind writes about the record,
// and the target it writes to when it does.
func writtenAboutTarget(ctx context.Context, tx pgx.Tx, id ids.ActivityID, kind crmcontracts.ActivityKind) (bool, EmailTarget, error) {
	if !slices.Contains(writtenAboutKinds, kind) {
		return false, EmailTarget{}, nil
	}
	targets, err := emailTargets(ctx, tx, []ids.UUID{id.UUID})
	if err != nil {
		return false, EmailTarget{}, err
	}
	return true, targets[id.UUID], nil
}
