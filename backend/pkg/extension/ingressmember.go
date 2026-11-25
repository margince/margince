// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What a member-owned chat source of a unit may state beyond a plain record.
// That is the contact or lead a member chose to file a message on, and how
// precisely its time is known. It also covers the posture its messages use.
//
//margince:extension-surface

package extension

import (
	"errors"
	"fmt"
	"slices"
)

// SubjectKind is the kind of record a member chose to file a message on.
type SubjectKind string

// The kinds of record a member may choose for a message.
const (
	SubjectContact SubjectKind = "contact"
	SubjectLead    SubjectKind = "lead"
)

var subjectKinds = []SubjectKind{SubjectContact, SubjectLead}

// ChosenSubject is the contact or lead a member chose for a message. The core
// checks that the member may see it and files the message on it directly. It
// does not resolve the message's counterparty by identity.
//
// A subject the member cannot see answers DispositionSubjectNotFound, like one
// that does not exist. A unit learns nothing about records the member may not see.
type ChosenSubject struct {
	Kind SubjectKind
	// ID is the canonical row id of the contact or lead.
	ID string
}

func (s ChosenSubject) validate() error {
	if !slices.Contains(subjectKinds, s.Kind) {
		return fmt.Errorf("extension: a chosen subject is a %q or a %q, not %q", string(SubjectContact), string(SubjectLead), string(s.Kind))
	}
	if !uuidGrammar.MatchString(s.ID) {
		return errors.New("extension: the chosen subject is not a canonical row id")
	}
	return nil
}

// OccurredPrecision says how well the original time of a record is known. It is
// the time the message happened, never the time the unit submitted it: the core
// stamps the submission itself.
type OccurredPrecision string

const (
	// PrecisionInstant is a moment. It is also what an empty precision means.
	PrecisionInstant OccurredPrecision = "instant"
	// PrecisionDay is a date with no time of day, such as a message a member
	// filed by hand from memory. The activity shows the date only.
	PrecisionDay OccurredPrecision = "day"
	// PrecisionUnknown says the unit does not know when the message happened.
	// The record may carry no OccurredAt, and the core dates it on the day it
	// received it, as a day. The timeline has no separate "unknown" state, so
	// it reads like a day entry.
	PrecisionUnknown OccurredPrecision = "unknown"
)

// precisionOf is the precision a record states: the explicit field, else the
// day marker the activity already carried, else an instant.
func (r Record) precisionOf() OccurredPrecision {
	switch {
	case r.OccurredPrecision != "":
		return r.OccurredPrecision
	case r.Activity.DateOnly:
		return PrecisionDay
	}
	return PrecisionInstant
}

// Precision is the precision the core files the record under.
func (r Record) Precision() OccurredPrecision { return r.precisionOf() }

func (r Record) validatePrecision() error {
	switch r.OccurredPrecision {
	case "", PrecisionInstant, PrecisionDay, PrecisionUnknown:
	default:
		return fmt.Errorf("extension: %q is not an occurred precision (%q, %q or %q)", string(r.OccurredPrecision),
			string(PrecisionInstant), string(PrecisionDay), string(PrecisionUnknown))
	}
	if r.Activity.DateOnly && r.OccurredPrecision == PrecisionInstant {
		return errors.New("extension: the activity says its time is a date and the record says it is an instant — state one")
	}
	if r.precisionOf() != PrecisionInstant && !r.Activity.DueAt.IsZero() {
		return errors.New("extension: a due date and a day-only occurred time belong to different kinds of record")
	}
	return nil
}

// Posture is the posture a member-owned source pins for what it files.
type Posture string

const (
	// PostureClassified files each message held until the confidentiality
	// engine has judged its thread, as classified mail is.
	PostureClassified Posture = "classified"
	// PostureHeld files each message held for its member until they release it.
	PostureHeld Posture = "held"
)

// validatePinnedPosture admits the two postures that narrow a reading. Shared
// is not one: a unit cannot widen who reads its member's correspondence.
func (s IngressSource) validatePinnedPosture() error {
	switch s.PinnedPosture {
	case "":
		return nil
	case PostureClassified, PostureHeld:
	default:
		return fmt.Errorf("extension: ingress source %q pins posture %q — a member-owned source may pin %q or %q, never a wider one",
			s.System, string(s.PinnedPosture), string(PostureClassified), string(PostureHeld))
	}
	if !slices.Contains(s.Lands, KindActivity) {
		return fmt.Errorf("extension: ingress source %q pins a posture but lands no activities — the posture is for a member's chat messages", s.System)
	}
	return nil
}

// MemberPosition is a member's saved place in a member-owned source, with the
// version a writer must present to move it.
type MemberPosition struct {
	Position BatchPosition
	// Version is 0 before the first save and grows by one with each.
	Version PositionVersion
}

// PositionVersion guards a member's saved position. A save names the version it
// read, and the core refuses it if another save moved the position since.
type PositionVersion int64

// ErrPositionConflict is a save of a member's position that named a version
// another save has since replaced. The writer read the position, so it is stale:
// read it again and continue from what is saved. It is also an ErrConflict.
var ErrPositionConflict = fmt.Errorf("%w: the member's saved position moved since it was read — read it again", ErrConflict)

// MemberBatchResult is what a member-scoped batch did.
type MemberBatchResult struct {
	Results []Result
	// Position is the member's position after the batch, with its new version.
	Position MemberPosition
}
