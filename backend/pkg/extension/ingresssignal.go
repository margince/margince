// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The signal payload a Record may carry in place of an activity.
//
//margince:extension-surface

package extension

import (
	"errors"
	"fmt"
	"unicode/utf8"
)

// KindSignal is a signal the unit raises about a record. Record.Key is the
// unit's key for the event. Core derives the fingerprint from the kind, the
// subject and that key, so a repeat of the event is one signal.
const KindSignal RecordKind = "signal"

// DispositionAlreadyRaised is a signal the installation already holds under the
// same kind, subject and key, raised by this unit or by a user. It is a
// SUCCESS carrying the held signal's Ref, so the cursor advances as it does for
// Accepted. A dismissed signal answers it too, so a replay never undoes a
// dismissal.
const DispositionAlreadyRaised Disposition = "already_raised"

// SignalFields is a signal on its way into the CRM.
type SignalFields struct {
	// Kind is the short name of a signal type the unit declared. Core builds the
	// stored name from it (SignalKindName), so a unit cannot name another's.
	Kind string
	// Subject is the record the signal is about. Its Type must be a record kind
	// the source declared in Lands, and the member the call acts for must be able
	// to see it.
	Subject  SignalSubject
	Evidence SignalEvidence
}

func (s SignalFields) validate() error {
	switch {
	case !recordKindGrammar.MatchString(s.Kind) || len(s.Kind) > maxRecordKindLength:
		return errors.New("extension: the signal names no valid signal type")
	case s.Subject.Type == "" || !uuidShape.MatchString(s.Subject.ID):
		return errors.New("extension: the signal names its subject by type and id")
	case utf8.RuneCountInString(s.Evidence.Summary) > MaxSubjectRunes:
		return fmt.Errorf("extension: the signal summary is over the %d-rune cap", MaxSubjectRunes)
	}
	return nil
}
