// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// WHICH part of the grammar refused a record, as a closed vocabulary.
//
// The refusal SENTENCE cannot be kept. Validate quotes the record back to say
// what is wrong with it — a participant's account id, a provider name, a role
// the unit invented — and those are third-party content the core deliberately
// does not store: the extension tier has no raw-payload table and no retention
// apparatus to go with one, and it must not acquire either by accident. So the
// sentence travels to the unit, which already holds the record, and only the
// class is written down.
//
// Six, one per check Validate runs, which is the granularity an operator
// actually acts on: "every record this connector sends is failing its
// participants" names the mapping to fix. A seventh would mean a seventh check.
//
//margince:extension-surface

package extension

import "errors"

// RecordRefusal names the check that refused a record.
type RecordRefusal string

const (
	// RefusalKey is the ingress system or the idempotency key: absent, blank
	// or over its cap.
	RefusalKey RecordRefusal = "key"
	// RefusalActivity is the activity itself — no kind, no occurred-at, an
	// over-long subject or body, an unknown direction.
	RefusalActivity RecordRefusal = "activity"
	// RefusalAddresses is the address set the internal-message gate reads
	// every party from.
	RefusalAddresses RecordRefusal = "addresses"
	// RefusalCounterparty is the human the record is about, or the channel
	// identity naming them.
	RefusalCounterparty RecordRefusal = "counterparty"
	// RefusalParticipants is the roster: over the cap, a party with no
	// identity, or a role outside the closed set.
	RefusalParticipants RecordRefusal = "participants"
	// RefusalSize is a record within the grammar and over a byte cap — the
	// thread key or the raw bytes every landed record pays for.
	RefusalSize RecordRefusal = "size"
)

// RecordRefusedError is what Validate answers: the class, and the sentence
// that says what to change.
//
// A unit reads the sentence and branches on the class; the core records the
// class and never the sentence. Both halves are needed — a class with no
// sentence cannot be acted on by the unit that built the record, and a
// sentence with no class cannot be counted by anybody.
type RecordRefusedError struct {
	Refusal RecordRefusal
	Cause   error
}

func (e *RecordRefusedError) Error() string { return e.Cause.Error() }

func (e *RecordRefusedError) Unwrap() error { return e.Cause }

// RefusalOf reports the class a refusal carries, or "" for an error that is
// not one — so a caller reading a class never has to know which error shape it
// was handed.
func RefusalOf(err error) RecordRefusal {
	var refused *RecordRefusedError
	if errors.As(err, &refused) {
		return refused.Refusal
	}
	return ""
}

// refuse wraps a check's complaint in its class. Validate's own helper: every
// branch below already writes the sentence, and this is what stops the class
// being a second thing each of them has to remember.
func refuse(class RecordRefusal, err error) error {
	if err == nil {
		return nil
	}
	return &RecordRefusedError{Refusal: class, Cause: err}
}
