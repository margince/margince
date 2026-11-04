// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// What a jurisdiction requires of a composed message, checked against the
// message itself.
//
// gates/messagingruleapplied_test.go has carried "nothing prepends it" about
// SubjectPrefix since the Vietnamese pack shipped: the decree fixes a [QC]
// label for advertising mail, the pack declares it, and no step between a
// composed subject and the provider ever consulted the rules. An advertising
// message left unmarked and nothing reported it.
//
// THIS ASKS THE MESSAGE, not the code that produced it. Every obligation is
// rendered somewhere earlier, and every one of those steps can be skipped by a
// path that composes its own body — which three of them do, recorded in that
// same gate's register. A check on the finished bytes is the one that cannot be
// walked past.

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
)

// The requirement names, in the vocabulary a reader of a parked delivery sees.
const (
	// RequirementSubjectPrefix is a pack's advertising label, which the
	// Vietnamese decree fixes as "[QC]".
	RequirementSubjectPrefix = "subject_prefix"
)

// CheckMessage answers one finding per requirement this message does not meet.
//
// AN EMPTY ANSWER IS THE ORDINARY ONE. A message under no applicable pack, or
// one that is not advertising, owes nothing here — and that is reported as
// nothing owed rather than as nothing checked, which are the same value and
// different facts. The caller parks on a finding, so answering findings for a
// message that owes none would refuse lawful mail.
func (s *Store) CheckMessage(
	ctx context.Context, deliveryID, decisionSetID, subject string,
) ([]RequirementFinding, error) {
	// GATED, on the object the delivery belongs to.
	//
	// The send worker runs as a system principal and passes, so this costs the
	// only production caller nothing. What it closes is a probe: the answer
	// varies with what the engine decided about a delivery, so an ungated
	// exported method would let anybody holding a delivery id learn whether it
	// was judged as advertising by watching a finding appear and disappear.
	//
	// `activity` at READ rather than a settings grant, and the difference
	// matters. Gating on installation_settings would be the mistake the
	// disclosure readers beside this one argue against: a rep holds contact and
	// activity grants and no settings grant, so it would refuse the check for
	// exactly the seats that compose the mail. This asks whether the caller may
	// read the timeline the delivery hangs off, which every legitimate one can.
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	var findings []RequirementFinding
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rulesets, found, err := advertisedRulesetsTx(ctx, tx, deliveryID, decisionSetID)
		if err != nil || !found {
			// NO DECISION, NO FINDING. A delivery the engine never judged is
			// one this check knows nothing about, and inventing a requirement
			// for it would park mail on an absence of evidence rather than on
			// evidence of absence.
			return err
		}
		// THE PREFIX IS READ HERE, off the rule set this lookup answered, rather
		// than inside the helper below taking it as a parameter.
		//
		// gates/messagingruleapplied_test.go scans for reads of a rule field on
		// a local bound from applicableRules, which is how it can tell an
		// applied obligation from a declared one. A helper reading the field
		// from its argument satisfies a human reader and not that scan, so the
		// register would still call this obligation unapplied while it was
		// being applied — which is the failure the register exists to prevent,
		// running backwards.
		//
		// One subject line goes to every recipient, so it owes the label of
		// each pack that judged an advertising row.
		for _, codes := range rulesets {
			rules, _, applicable := applicableRules(codes...)
			if !applicable {
				continue
			}
			for _, f := range subjectPrefixFindings(rules.SubjectPrefix, string(rules.Jurisdiction), subject) {
				if !slices.Contains(findings, f) {
					findings = append(findings, f)
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return findings, nil
}

// RequirementFinding is one requirement a message does not meet. It mirrors
// comms.RequirementFinding across the module boundary: consent may not import
// comms, and the composition root adapts between them.
//
// Held by: TestTheRequirementFindingAgreesAcrossTheSeam
// (backend/gates/requirementseam_test.go)
type RequirementFinding struct {
	// Requirement names what is missing, in the vocabulary the packs use.
	Requirement string
	// Detail says what was expected, for whoever reads the record.
	Detail string
}

// subjectPrefixFindings reports an advertising message whose subject does not
// carry the label its pack fixes.
//
// ADVERTISING ONLY. The packs declare the prefix as a marking on advertising,
// and putting it on an invoice would tell a recipient their payment reminder is
// an advertisement — a false statement made to satisfy a check, which is worse
// than the omission it would be fixing.
//
// The comparison is case-insensitive on a trimmed subject. A composer that
// wrote "[qc]" met the decree's purpose, and refusing over the case of a label
// would park lawful mail on a typographic reading nobody argues for.
func subjectPrefixFindings(declared, jurisdiction, subject string) []RequirementFinding {
	prefix := strings.TrimSpace(declared)
	if prefix == "" {
		return nil
	}
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(subject)), strings.ToUpper(prefix)) {
		return nil
	}
	return []RequirementFinding{{
		Requirement: RequirementSubjectPrefix,
		Detail: fmt.Sprintf("an advertising subject carries %q in %s, and this one begins %q",
			prefix, jurisdiction, firstRunes(subject, 24)),
	}}
}

// advertisedRulesetsTx answers the ruleset of every recipient this send
// advertises to, within the one decision set it was authorized on. found is
// false when the set holds no decision at all.
//
// Bound to the set, never to the delivery id alone. A delivery can hold
// several sets: a paced message redispatched later writes a fresh one beside
// the old. Reading by delivery would judge the message by a category it no
// longer has.
//
// Each row is read on its own. A message can be a deal follow-up for one
// addressee and advertising for another. It still advertises to the second,
// and that recipient's pack is owed its label. The rulesets are the ones each
// decision recorded, so the check asks the rules that judged the send.
func advertisedRulesetsTx(
	ctx context.Context, tx pgx.Tx, deliveryID, decisionSetID string,
) (rulesets [][]jurisdiction.Code, found bool, err error) {
	rows, err := tx.Query(ctx, `
		SELECT resolved_category, ruleset_codes FROM communication_decision
		 WHERE delivery_id = $1 AND decision_set_id = $2`, deliveryID, decisionSetID)
	if err != nil {
		return nil, false, fmt.Errorf(
			"consent: reading what this delivery was judged to be: %w", err)
	}
	type judged struct {
		Category string
		Codes    []string
	}
	all, err := pgx.CollectRows(rows, pgx.RowToStructByPos[judged])
	if err != nil {
		return nil, false, fmt.Errorf(
			"consent: reading what this delivery was judged to be: %w", err)
	}
	for _, j := range all {
		if commsauthz.Category(j.Category) != commsauthz.CategoryMarketing {
			continue
		}
		codes := make([]jurisdiction.Code, 0, len(j.Codes))
		for _, c := range j.Codes {
			codes = append(codes, jurisdiction.Code(c))
		}
		rulesets = append(rulesets, codes)
	}
	return rulesets, len(all) > 0, nil
}

// firstRunes bounds what a finding quotes back.
//
// A finding is written into a parked delivery's reason, which an operator
// reads. Quoting a whole subject line would put the message's own words into a
// field meant to say what is wrong with it.
func firstRunes(s string, n int) string {
	// The conversion re-encodes a broken byte as U+FFFD whether or not the cut happens.
	return textcut.RunesMarked(string([]rune(strings.TrimSpace(s))), n, "…")
}
