// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import (
	"testing"
	"time"
)

// twoDeals is a pair that is one buying motion: one shared meeting, one
// pipeline and close dates a month apart. Each case below varies one field.
func twoDeals() PairSubject {
	return PairSubject{
		DealID: "b", OtherDealID: "a", Owner: "u1", AmountMinor: new(int64(500_000)), Currency: "EUR",
		PipelineID: "p1", OtherPipelineID: "p1",
		ExpectedClose: new(at(time.June, 30)), OtherExpectedClose: new(at(time.June, 1)),
		SharedActivities: 1,
	}
}

func TestDuplicateMotionFiresOnSharedEvidenceInOnePipelineWithinTheWindow(t *testing.T) {
	t.Parallel()
	found := duplicateMotion(twoDeals())
	if found == nil {
		t.Fatal("two deals sharing a meeting, one pipeline and close dates 29 days apart raised nothing")
	}
	if found.Type != TypeDuplicateMotion || found.SubjectID != "b" || found.OtherID != "a" {
		t.Fatalf("finding = %+v, want %s filed on the later deal b, naming a", found, TypeDuplicateMotion)
	}
	if found.Observed[SlotCloseGapDays] != 29 {
		t.Fatalf("close gap = %v, want 29", found.Observed[SlotCloseGapDays])
	}
}

func TestDuplicateMotionFiresOnAStakeholderAlone(t *testing.T) {
	t.Parallel()
	s := twoDeals()
	s.SharedActivities, s.SharedStakeholders = 0, 1
	if duplicateMotion(s) == nil {
		t.Fatal("a shared stakeholder with no shared meeting raised nothing")
	}
}

func TestDuplicateMotionRefusesWhatIsNotOneMotion(t *testing.T) {
	t.Parallel()
	for name, change := range map[string]func(*PairSubject){
		"other pipeline":              func(s *PairSubject) { s.OtherPipelineID = "p2" },
		"no close date":               func(s *PairSubject) { s.ExpectedClose = nil },
		"the other has no close date": func(s *PairSubject) { s.OtherExpectedClose = nil },
		"no shared evidence":          func(s *PairSubject) { s.SharedActivities = 0 },
	} {
		s := twoDeals()
		change(&s)
		if found := duplicateMotion(s); found != nil {
			t.Errorf("%s: raised %+v", name, found)
		}
	}
}

func TestDuplicateMotionWindowEndsAtSixtyDays(t *testing.T) {
	t.Parallel()
	s := twoDeals()
	s.OtherExpectedClose = new(at(time.May, 1))
	if duplicateMotion(s) == nil {
		t.Error("60 days apart raised nothing; the window includes its last day")
	}
	s.OtherExpectedClose = new(at(time.April, 30))
	if duplicateMotion(s) != nil {
		t.Error("61 days apart raised a finding")
	}
}

func TestLogicalKeyOfAPairIsTheSameWhicheverDealItIsFiledOn(t *testing.T) {
	t.Parallel()
	one := LogicalKey(Finding{Type: TypeDuplicateMotion, SubjectID: "b", OtherID: "a"})
	other := LogicalKey(Finding{Type: TypeDuplicateMotion, SubjectID: "a", OtherID: "b"})
	if one != other {
		t.Fatalf("keys %q and %q differ for one pair", one, other)
	}
	if one == LogicalKey(Finding{Type: TypeDuplicateMotion, SubjectID: "b", OtherID: "c"}) {
		t.Fatal("two different pairs share a key")
	}
}

func TestSeparateIsAnAnswerWithNoExpiry(t *testing.T) {
	t.Parallel()
	expiry, err := checkResolution(Resolution{Outcome: OutcomeSeparate}, at(time.May, 14))
	if err != nil || expiry != nil {
		t.Fatalf("separate = (%v, %v), want no expiry and no refusal", expiry, err)
	}
}
