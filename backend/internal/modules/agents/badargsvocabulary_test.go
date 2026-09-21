// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// A refusal that names a closed vocabulary delivers it, whatever the caller sent.
//
// BadArgsError splits its message by provenance for this reason: Cause may quote
// the CALLER, so it is bounded and escaped at maxBadArgsDetail, while Guidance is
// OURS and is appended whole. A vocabulary therefore survives in Guidance and can
// be truncated in Cause — which is not a style preference but the difference
// between a caller who can fix their call and one who guesses.
//
// FOUR SITES GOT IT WRONG, in two shapes. `promote_lead`'s trigger, a project
// phase and a relink target named NO vocabulary at all, so an agent was refused
// against a closed set of a handful and told nothing about it — while the REST
// twin of the first spelled all four out. `read_import_run`'s object and a list
// filter's operand put the SET in Cause, where it shares a budget sized for an
// echo of a caller's own word and neither said which value had been refused.
//
// So this drives each producer twice: once with an ordinary bad value, and once
// with a flood, because a vocabulary that survives the first and not the second
// is one a caller can erase.

import (
	"strings"
	"testing"
)

// vocabularyRefusal is one closed set, and the way to be refused against it.
type vocabularyRefusal struct {
	what    string
	members []string
	refuse  func(value string) error
}

// vocabularyRefusals takes every set from the predicate that admits it, so a
// member added or removed moves this test rather than leaving it certifying a
// vocabulary that no longer exists.
func vocabularyRefusals(t *testing.T) []vocabularyRefusal {
	t.Helper()
	subjects := []vocabularyRefusal{
		{
			what:    "the engagement a promotion rests on",
			members: genuineTriggerNames(),
			refuse:  requireGenuineTrigger,
		},
		{
			what:    "a project's phase ladder",
			members: projectPhaseNames(),
			refuse:  func(v string) error { return requireProjectPhase(v, nil) },
		},
		{
			what:    "what an activity may be relinked onto",
			members: relinkTargetNames(),
			refuse:  requireLinkTarget,
		},
		{
			what:    "the objects an import takes",
			members: importObjectEnum,
			refuse:  refuseUnimportableObject,
		},
		{
			what:    "the values one list filter takes",
			members: []string{"open", "won", "lost"},
			refuse: func(v string) error {
				return refuseUnaskableOperand(
					listFilter{Name: "status", Enum: []string{"open", "won", "lost"}}, v)
			},
		},
	}
	for _, s := range subjects {
		if len(s.members) == 0 {
			t.Fatalf("%s came back empty, so its case certifies nothing", s.what)
		}
	}
	return subjects
}

func TestEveryRefusedVocabularyReachesTheCaller(t *testing.T) {
	t.Parallel()
	for _, subject := range vocabularyRefusals(t) {
		t.Run(subject.what, func(t *testing.T) {
			t.Parallel()
			for _, value := range map[string]string{
				"an ordinary wrong value": "not-a-member",
				// The caller's own word is bounded; the vocabulary is not, so a
				// flood must not be able to push the set out of the answer.
				"a flooded value": strings.Repeat("k", 4000),
			} {
				err := subject.refuse(value)
				if err == nil {
					t.Fatalf("%q was admitted, so nothing is refused against %s", value[:16], subject.what)
				}
				said := err.Error()
				for _, member := range subject.members {
					if !strings.Contains(said, member) {
						t.Errorf("the refusal never names %q, so a caller cannot pick it:\n%s",
							member, said)
					}
				}
			}
		})
	}
}

// The other half of the split, and the reason a vocabulary belongs in Guidance:
// a caller's word must not be able to make the answer arbitrarily long.
func TestARefusedValueIsBoundedInWhatItEchoes(t *testing.T) {
	t.Parallel()
	for _, subject := range vocabularyRefusals(t) {
		t.Run(subject.what, func(t *testing.T) {
			t.Parallel()
			flood := strings.Repeat("k", 4000)
			said := subject.refuse(flood).Error()

			if strings.Contains(said, strings.Repeat("k", maxBadArgsDetail+1)) {
				t.Errorf("%s echoed more than maxBadArgsDetail (%d) of the caller's value, so the "+
					"caller chose how much this writes into the transcript",
					subject.what, maxBadArgsDetail)
			}
		})
	}
}

// WHAT THIS CANNOT SEE, stated rather than left to chance: two vocabularies on
// this surface are refused inline inside an argument reader rather than by a
// named predicate — enrich's depth and an approval's decision. Both were moved
// to Guidance in the same change that added this file, and reaching them from
// here means extracting the check the way requireGenuineTrigger already is,
// which is worth doing when one of them next changes.
