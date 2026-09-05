// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package commsauthz

import "testing"

// The override rule, stated as a table so a reader sees the whole matrix at
// once rather than inferring it from four separate assertions.
//
// Every pair is here, including the ones that are true for boring reasons. A
// matrix with holes is a matrix where the next reader cannot tell an intended
// answer from a case nobody thought about.
func TestWhoMayOverruleWhom(t *testing.T) {
	t.Parallel()

	levels := []AuthorityLevel{LevelMachine, LevelUser, LevelAdmin, LevelSubject}
	// want[caller][decided]: may the caller overrule a decision at that level?
	want := map[AuthorityLevel]map[AuthorityLevel]bool{
		LevelMachine: {LevelMachine: false, LevelUser: false, LevelAdmin: false, LevelSubject: false},
		LevelUser:    {LevelMachine: true, LevelUser: false, LevelAdmin: false, LevelSubject: false},
		LevelAdmin:   {LevelMachine: true, LevelUser: true, LevelAdmin: false, LevelSubject: false},
		LevelSubject: {LevelMachine: true, LevelUser: true, LevelAdmin: true, LevelSubject: false},
	}

	for _, caller := range levels {
		for _, decided := range levels {
			got := caller.CanOverrule(decided)
			if got != want[caller][decided] {
				t.Errorf("%s overruling %s = %v, want %v", caller, decided, got, want[caller][decided])
			}
		}
	}
}

// TestNobodyOverrulesTheSubject is the row of the matrix above that carries a
// legal obligation rather than a product preference, so it is asserted on its
// own where a reader deleting it has to notice what they are deleting.
//
// An Art. 21 objection to direct marketing is absolute. A product that let an
// administrator lift one would be offering a control that cannot lawfully be
// used, and the person who pressed it would believe the send was permitted.
func TestNobodyOverrulesTheSubject(t *testing.T) {
	t.Parallel()

	for _, caller := range []AuthorityLevel{LevelMachine, LevelUser, LevelAdmin, LevelSubject} {
		if caller.CanOverrule(LevelSubject) {
			t.Errorf("%s may overrule the subject; nothing may", caller)
		}
	}
}

// TestAPeerDoesNotSilentlyOverruleAPeer holds the equal-rank case.
//
// Two reps disagreeing about a contact is a real situation, and the answer is
// not that whoever sends second wins. Reversing a peer's decision stays a
// deliberate act with its own record, rather than something that happens as a
// side effect of composing a message.
func TestAPeerDoesNotSilentlyOverruleAPeer(t *testing.T) {
	t.Parallel()

	if LevelUser.CanOverrule(LevelUser) {
		t.Error("a rep may overrule another rep's decision by sending; it must be deliberate")
	}
	if LevelAdmin.CanOverrule(LevelAdmin) {
		t.Error("an admin may overrule another admin's decision by sending; it must be deliberate")
	}
}

// TestAnUnknownLevelIsTreatedAsStrongerThanAnyKnownOne is the version-skew case.
//
// A row written by a newer build carries a level this one cannot name. Ranking
// it low would let this build overrule a decision it does not understand, which
// is the one direction that loses somebody's objection. Unknown means "at least
// as strong as anything here", so nothing overrules it.
func TestAnUnknownLevelIsTreatedAsStrongerThanAnyKnownOne(t *testing.T) {
	t.Parallel()

	future := AuthorityLevel("regulator")
	for _, caller := range []AuthorityLevel{LevelMachine, LevelUser, LevelAdmin, LevelSubject} {
		if caller.CanOverrule(future) {
			t.Errorf("%s overrules an unrecognised level; an unknown decision must stand", caller)
		}
	}
	if future.Valid() {
		t.Error("an unrecognised level reports Valid; storage and requests must reject it")
	}
}

// TestAbsoluteAndOverrulableAreDifferentAxes is the distinction the model got
// wrong once and must not get wrong again.
//
// commsauthz.Absolute names the refusals no ROLLOUT MODE may soften. That is a
// statement about how hard a refusal binds. It is NOT a statement about whose
// decision it was, and the two were briefly conflated here — every absolute
// reason was mapped to LevelSubject, which made a duplicate contact and a
// rolling frequency window permanently unliftable by anybody.
//
// A refusal can bind absolutely and still be nobody's decision. A dead mailbox
// is corrected, not overruled. Two people sharing an address are merged. A
// volume window clears itself. Each of those is a human act on the CRM, and a
// model that forbade them would leave an admin looking at a dead button for a
// problem they could fix in one click.
func TestAbsoluteAndOverrulableAreDifferentAxes(t *testing.T) {
	t.Parallel()

	// Absolute, and nobody's decision: a seat may clear each one.
	factsAboutTheWorld := []string{
		ReasonHardBounce,          // correct the address
		ReasonFrequencyCapReached, // wait for the window
		ReasonNoSubject,           // merge the duplicate records
		ReasonUnconfirmedDOI,      // record the opt-in the installation holds
	}
	for _, reason := range factsAboutTheWorld {
		if !Absolute(reason) {
			t.Errorf("%q is no longer an absolute denial; this test's premise moved", reason)
		}
		if !LevelUser.CanOverrule(LevelForReason(reason)) {
			t.Errorf("%q cannot be cleared by a rep, but it is a fact to correct "+
				"rather than a decision to respect", reason)
		}
	}

	// Absolute AND the subject's own act: no seat clears these.
	theSubjectSpoke := []string{ReasonObjection, ReasonRestricted, ReasonConsentWithdrawn}
	for _, reason := range theSubjectSpoke {
		if !Absolute(reason) {
			t.Errorf("%q is no longer an absolute denial; this test's premise moved", reason)
		}
		if LevelAdmin.CanOverrule(LevelForReason(reason)) {
			t.Errorf("an admin may clear %q; it is the subject's own act and binds absolutely", reason)
		}
	}
}

// TestOnlyTheFourLevelsAreValid guards the storage constraint's twin. The CHECK
// holds the database; this holds everything that reaches a decision without
// passing through one.
func TestOnlyTheFourLevelsAreValid(t *testing.T) {
	t.Parallel()

	for _, valid := range []AuthorityLevel{LevelMachine, LevelUser, LevelAdmin, LevelSubject} {
		if !valid.Valid() {
			t.Errorf("%q is a defined level but reports invalid", valid)
		}
	}
	for _, invalid := range []AuthorityLevel{"", "Machine", "root", "system"} {
		if AuthorityLevel(invalid).Valid() {
			t.Errorf("%q reports valid; only the four defined levels are", invalid)
		}
	}
}

// TestEveryAbsoluteReasonIsClassifiedDeliberately is the census.
//
// LevelForReason has a default arm, which means a NEW absolute reason added to
// absoluteDenials gets LevelMachine silently — overrulable by any rep, with no
// test failing and no reviewer prompted. That is the shape of gap this codebase
// keeps finding: under-recognition reports PASS.
//
// So the classification is written down here, beside the rule rather than
// inside it, and a reason missing from this map fails. Adding one is a
// deliberate line, which is the point.
func TestEveryAbsoluteReasonIsClassifiedDeliberately(t *testing.T) {
	t.Parallel()

	classified := map[string]AuthorityLevel{
		// The subject's own act. Nothing lifts these.
		ReasonObjection:        LevelSubject,
		ReasonRestricted:       LevelSubject,
		ReasonConsentWithdrawn: LevelSubject,
		// Absolute, but nobody's decision — each names a fact a human corrects.
		ReasonHardBounce:          LevelMachine,
		ReasonFrequencyCapReached: LevelMachine,
		ReasonNoSubject:           LevelMachine,
		ReasonUnconfirmedDOI:      LevelMachine,
	}

	for reason := range absoluteDenials {
		want, named := classified[reason]
		if !named {
			t.Errorf("absolute reason %q has no deliberate authority level: add an arm to "+
				"LevelForReason and a row here, or it silently becomes overrulable by any rep", reason)
			continue
		}
		if got := LevelForReason(reason); got != want {
			t.Errorf("LevelForReason(%q) = %q, want %q", reason, got, want)
		}
	}

	// The map must not outgrow its subject either: a row for a reason that is
	// no longer absolute is a stale claim about a rule that moved.
	for reason := range classified {
		if !absoluteDenials[reason] {
			t.Errorf("%q is classified here but is no longer an absolute denial; "+
				"remove the row or restore the reason", reason)
		}
	}
}
