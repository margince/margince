// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The subject's own route to stopping more than an unsubscribe stops.
//
// One-click unsubscribe withdraws the marketing-class purposes, which is what a
// subscription link offers. This is the other act: an Art. 21 objection, which
// says the processing must stop whether or not consent was ever its basis. That
// is the only thing that reaches business correspondence, and it is why the two
// are separate doors rather than two sizes of one.
//
// Before this existed the unsubscribe press did both jobs badly: it swept every
// purpose in the catalog, so it stopped correspondence nobody had subscribed to
// while recording the act as a withdrawal of a consent that was never the basis
// for those messages. Narrowing the press without building this would have left
// a subject with no way to say "stop entirely".

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// PublicStopAction is what the subject asked for.
type PublicStopAction string

const (
	// StopAllMarketing objects to direct marketing under Art. 21(2). It leaves
	// correspondence and invoices running.
	StopAllMarketing PublicStopAction = "stop_all_marketing"
	// StopAllContact asks for contact to stop entirely. It binds every category
	// except the few that survive an Art. 18 restriction.
	StopAllContact PublicStopAction = "stop_all_contact"
)

// PublicStopResult is what the page tells the subject.
type PublicStopResult struct {
	// Recorded is false on a replay, because the stop was already standing.
	// The page tells a first press from a second one by this rather than by
	// showing a fresh confirmation for a no-op.
	Recorded bool
	// ReceiptReference is what the subject quotes when asking what happened.
	ReceiptReference string
}

// PublicStop records a stop the SUBJECT asked for, on the anonymous preference
// edge.
//
// NO SEAT GATE, and that is the difference from Suppress beside it. That door
// is for a rep relaying a phone call, so it asks whether the caller may write
// about this contact. Here the token IS the capability: the middleware resolved
// it to this contact before the handler ran, and there is no seat to ask about.
// Asking auth.Require would refuse the subject their own Art. 21 right.
//
// LEVEL SUBJECT for both actions, whatever authorityOf would say. The subject
// is the one pressing, so no seat may lift what they recorded — which is the
// whole point of an objection the controller has to answer for.
func (s *Store) PublicStop(
	ctx context.Context, contactID ids.ContactID, action PublicStopAction, statement string,
) (PublicStopResult, error) {
	kind, err := publicStopKind(action)
	if err != nil {
		return PublicStopResult{}, err
	}
	if err := boundStatement(statement); err != nil {
		return PublicStopResult{}, err
	}
	sub, err := consentSubject(RecordInput{ContactID: contactID})
	if err != nil {
		return PublicStopResult{}, err
	}
	in := SuppressInput{ContactID: contactID, Kind: kind, Reason: statement}

	var result PublicStopResult
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		// THE LOCK FIRST, and it is the same one suppressAdmittedTx takes.
		//
		// Taking it there only would let two presses both read "no stop
		// standing", then queue on the lock and each insert a row, an audit
		// entry and an event — two records of one request, both reporting
		// recorded=true. There is no unique index to catch that: the one on
		// this table covers address-scoped hard bounces.
		//
		// Re-entrant, so the write taking it again inside is free.
		//
		// THIS ID FIRST, then the survivor below. They are the same key unless
		// a merge has retired this contact, and when they differ both must be
		// held: the read below and the write inside would otherwise queue on
		// different keys and the race would be open again. Original-then-
		// survivor is the order the merge itself takes them in, so two
		// transactions cannot take them in opposite orders and deadlock.
		if err := lockSubjectSuppressions(ctx, tx, sub.id); err != nil {
			return err
		}
		// THE SURVIVOR, asked before the replay check rather than only inside
		// the write.
		//
		// A token minted for a contact keeps working after that contact is
		// merged away, and suppressAdmittedTx writes onto the survivor because
		// no reader of communication_suppression walks merged_into_id. Checking
		// the press against the RETIRED id would find no stop every time and
		// insert another row on the survivor at every press, so the replay
		// answer would be wrong for exactly the subject whose record moved.
		//
		// The lock inside the write is what makes a merge either committed
		// before this read or not yet begun; this reads the same pointer that
		// write will follow.
		surviving, err := survivingSubject(ctx, tx, sub.id)
		if err != nil {
			return err
		}
		if surviving != sub.id {
			if err := lockSubjectSuppressions(ctx, tx, surviving); err != nil {
				return err
			}
			sub.id = surviving
		}

		standing, err := publicStopStandingTx(ctx, tx, sub, kind)
		if err != nil {
			return err
		}
		// A REPLAY RECORDS NOTHING NEW. Suppress's own door records both
		// occasions deliberately, because a rep relaying a second phone call is
		// a second occasion somebody asked and the reason may differ. A subject
		// pressing the same link twice is one occasion and a stuck browser, so
		// this one answers the receipt it already has.
		if standing {
			result = PublicStopResult{Recorded: false, ReceiptReference: publicStopReceipt(sub.id, kind)}
			return nil
		}
		if err := s.suppressAdmittedTx(ctx, tx, in, sub, commsauthz.LevelSubject); err != nil {
			return err
		}
		result = PublicStopResult{Recorded: true, ReceiptReference: publicStopReceipt(sub.id, kind)}
		return nil
	})
	if err != nil {
		return PublicStopResult{}, err
	}
	return result, nil
}

// publicStopKind maps the action to the suppression kind that records it.
//
// TOTAL over the declared actions, and an unrecognised one is refused rather
// than defaulted. Defaulting to the narrower kind would silently under-record a
// subject who asked for everything to stop; defaulting to the broader one would
// stop invoices somebody never asked to stop.
func publicStopKind(action PublicStopAction) (string, error) {
	switch action {
	case StopAllMarketing:
		return commsauthz.ReasonObjection, nil
	case StopAllContact:
		return suppressibleKind, nil
	}
	return "", &ValidationError{
		Field:  "action",
		Reason: "a stop is either stop_all_marketing or stop_all_contact",
	}
}

// publicStopStandingTx reports whether this kind of stop already stands for this
// subject AT THEIR OWN AUTHORITY.
//
// THE LEVEL IS PART OF THE QUESTION, and leaving it out was a defect Codex
// found. A rep relaying a phone call writes a subject_request at LevelUser,
// which an admin may lift. Asking only "does a subject_request stand" would
// then answer yes to the subject's own press, record nothing, and hand them a
// receipt — after which an admin lifts the rep's row and the mail resumes, with
// the subject's own decision never written down.
//
// So a weaker row does not satisfy a stronger press. The subject's row is
// recorded beside it, and liveSuppression takes the strongest.
func publicStopStandingTx(ctx context.Context, tx pgx.Tx, sub subject, kind string) (bool, error) {
	var standing bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM communication_suppression
		   WHERE `+sub.column+` = $1 AND kind = $2 AND revoked_at IS NULL
		     AND decided_by_level = $3)`,
		sub.id, kind, string(commsauthz.LevelSubject)).Scan(&standing)
	if err != nil {
		return false, fmt.Errorf("consent: reading whether this stop already stands: %w", err)
	}
	return standing, nil
}

// publicStopReceipt is what the subject quotes when asking what happened.
//
// DERIVED, not stored, and stable across a replay: the same subject and the
// same kind answer the same reference, so somebody who pressed twice and got
// two confirmations is not holding two different receipts for one request.
// It carries no address and nothing a stranger could work backwards from.
func publicStopReceipt(subjectID ids.UUID, kind string) string {
	short := subjectID.String()
	return "STOP-" + kind[:3] + "-" + short[len(short)-12:]
}

// boundStatement caps the subject's own words.
//
// SAME BOUND as boundReason beside it and a different FIELD NAME, which is the
// whole reason it is not that function. boundReason hard-codes "reason", and
// this route's body calls the field "statement" — a 422 naming a field the
// caller never sent cannot be attached to the input that caused it, so the page
// would show the error nowhere.
func boundStatement(statement string) error {
	if n := len([]rune(statement)); n > reasonMax {
		return &ValidationError{
			Field: "statement",
			Reason: fmt.Sprintf("your words are kept at up to %d characters; this is %d",
				reasonMax, n),
		}
	}
	return nil
}
