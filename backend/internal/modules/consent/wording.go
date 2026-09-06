// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// What a proof row may claim about what the subject was shown.
//
// Art. 7(1) asks the controller to demonstrate what the subject agreed TO, so a
// grant that cannot say what was on the screen is a grant nobody can stand
// behind. The confirm-details door enforced this at its own edge from the day it
// shipped; every other door reached the same writer without it, and the writer
// filled the gap with the literal 'recorded via API'. One rule, at the writer
// both doors go through.

import (
	"fmt"
	"strings"
)

// defaultWordingVersion names wording a door recorded without versioning it.
//
// Not a claim that the sentence never changes: it is the version the proof row
// carries when the door showing it does not manage versions of its own, which
// is every door but the booking form today. When consent_text_version lands,
// this is what a row pointing at a published version stops needing.
const defaultWordingVersion = "v1"

// wordingFor settles what the proof row actually stores, for both columns at
// once, because consent_event_wording_pairs stores them both-or-neither.
//
// ONLY A GRANT CARRIES WORDING. A withdrawal demonstrates nothing — there is no
// screen to quote — so anything a caller passes for one is dropped rather than
// written. Several fixtures pass the same input for both states to keep one
// helper shape, and without this a withdrawal row would claim the sentence that
// accompanied a GRANT.
//
// A grant with no version named gets the default: doors that show wording
// without versioning it (the preference centre, the confirm link) would each
// otherwise invent one, which is a second answer to "which wording was this".
//
// Settled HERE and not in admitRecord, which takes its input by value: a value
// written there never reaches the INSERT, and the CHECK would refuse every row
// those doors write.
func wordingFor(state ConsentState, text, version *string) (*string, *string) {
	if state != StateGranted {
		return nil, nil
	}
	if text == nil {
		return nil, nil
	}
	if version == nil {
		// A local copy: the default is a const, and every row must carry its own
		// value rather than share one address across concurrent writes.
		fallback := defaultWordingVersion
		return text, &fallback
	}
	return text, version
}

// requireRecordableWording is the whole rule, at the one place every door
// reaches. The sentence and the version that names it are stored together, so
// they are checked together — a bound on one alone leaves the same hole open a
// column across.
func requireRecordableWording(state ConsentState, wording, version *string) error {
	if err := requireWordingForGrant(state, wording); err != nil {
		return err
	}
	return requireBoundedVersion(version)
}

func requireWordingForGrant(state ConsentState, wording *string) error {
	if state != StateGranted {
		return nil
	}
	if wording == nil || strings.TrimSpace(*wording) == "" {
		return &ValidationError{
			Field:  "wording",
			Reason: "a grant records the exact wording shown to the subject",
		}
	}
	// The contract's own maxLength, enforced here because nothing generated
	// does: unchecked, one caller stores a megabyte on a proof row that every
	// later reader of this person's consent history is served in full, and the
	// subject access export reads it back.
	if len([]rune(*wording)) > maxWordingRunes {
		return &ValidationError{
			Field:  "wording",
			Reason: fmt.Sprintf("the recorded wording is at most %d characters", maxWordingRunes),
		}
	}
	return nil
}

// requireBoundedVersion bounds the OTHER half of the pair.
//
// The wording and the version that names it are stored or absent together, so a
// bound on one alone leaves the same hole open: an anonymous booking may set the
// version, it is written verbatim, and the subject access export reads it back
// in full. A version id is a short token — the ceiling only has to be low enough
// that a proof row cannot become a payload.
func requireBoundedVersion(version *string) error {
	if version == nil {
		return nil
	}
	// Blank is not "absent": absent means the writer picks the default, while a
	// caller-supplied empty string stores wording that names no version at all.
	if strings.TrimSpace(*version) == "" {
		return &ValidationError{
			Field:  "policy_version",
			Reason: "a recorded wording version cannot be blank",
		}
	}
	if len([]rune(*version)) > maxVersionRunes {
		return &ValidationError{
			Field:  "policy_version",
			Reason: fmt.Sprintf("the recorded wording version is at most %d characters", maxVersionRunes),
		}
	}
	return nil
}
