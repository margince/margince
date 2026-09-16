// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companyscan

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Retraction: what happens to STORED advice when the record it was written
// from stops standing.
//
// A scan's findings are written once and replayed on every open, so they
// outlive their own evidence. Archive the email nine minutes after the scan
// ran and the card is still there, still quoting the subject, still offering
// "Create draft" — and the composer, which reads the anchor live, then
// refuses. Archiving is meant to take a record out of view, and the advice
// written about it is part of that view.
//
// Every PRODUCER already filters archived rows at write time. This is the
// replay's half of the same rule.

// standingCitations asks which activities these findings cite that still
// stand for this reader: live, and inside the reader's own discover scope.
//
// The DISCOVER question, asked of the row, and deliberately not answered from
// the email-summary reader that enriches the same list a few lines later.
// That reader is narrower twice over — it admits only `kind = 'email'`, and it
// gates on content because it prints a subject and a body preview. Reading
// "gone" out of its silence would retract a finding citing a live MEETING, and
// one citing a live email whose body this reader may not open. Both are advice
// that is perfectly valid, and dropping them is a worse defect than the card
// this retraction exists to remove.
func (s *Service) standingCitations(
	ctx context.Context, findings []crmcontracts.Company360Suggestion,
) (map[ids.UUID]bool, error) {
	cited := citedActivities(findings)
	if len(cited) == 0 {
		return map[ids.UUID]bool{}, nil
	}
	var standing map[ids.UUID]bool
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		var readErr error
		standing, readErr = activities.StandingActivities(ctx, tx, cited)
		return readErr
	})
	return standing, err
}

// citedActivities collects the activities these findings name, deduped: the
// records their evidence rests on, plus the anchor an action would open.
func citedActivities(findings []crmcontracts.Company360Suggestion) []ids.UUID {
	seen := map[ids.UUID]bool{}
	var out []ids.UUID
	add := func(id ids.UUID) {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for _, finding := range findings {
		if finding.Action != nil && finding.Action.ActivityId != nil {
			add(ids.UUID(*finding.Action.ActivityId))
		}
		for _, evidence := range finding.Evidence {
			if evidence.EntityType == crmcontracts.CompanyBriefEvidenceEntityTypeActivity {
				add(ids.UUID(evidence.EntityId))
			}
		}
	}
	return out
}

// keepCited drops a finding that has lost the record it stands on.
func keepCited(
	findings []crmcontracts.Company360Suggestion, standing map[ids.UUID]bool,
) []crmcontracts.Company360Suggestion {
	kept := make([]crmcontracts.Company360Suggestion, 0, len(findings))
	for _, finding := range findings {
		if !retracted(finding, standing) {
			kept = append(kept, finding)
		}
	}
	return kept
}

// retracted answers whether this finding has lost the record it stands on.
//
// Not standing covers archived, outside this reader's discover scope, and
// deleted alike — all three are reasons this reader should not be shown the
// record, so all three retract the advice written about it.
//
// Two ways to lose it, and either is enough:
//
// Every activity it cites is gone, so the contract's promise that evidence is
// "always ones this reader can open" no longer holds for any of it. A finding
// naming one standing record among several archived ones stays — it still has
// evidence a reader can open.
//
// Or its action anchors on an activity that is gone. That is the "Create
// draft" button: the composer it opens reads the anchor live and refuses, so
// an anchor that no longer stands is a control that cannot do what it offers.
// A rule firing on the account's shape rather than on one record carries no
// anchor and no citation, and is untouched.
func retracted(
	finding crmcontracts.Company360Suggestion, standing map[ids.UUID]bool,
) bool {
	if finding.Action != nil && finding.Action.ActivityId != nil &&
		!standing[ids.UUID(*finding.Action.ActivityId)] {
		return true
	}
	cited := 0
	for _, evidence := range finding.Evidence {
		if evidence.EntityType != crmcontracts.CompanyBriefEvidenceEntityTypeActivity {
			continue
		}
		cited++
		if standing[ids.UUID(evidence.EntityId)] {
			return false
		}
	}
	return cited > 0
}

// applyCap trims the list to what the page draws and answers how many rows
// that left out — the number the wire field reports.
//
// It runs AFTER retraction, so a finding quoting an archived record does not
// hold a slot against a live one. Capping first would report the live row as
// dropped by the cap and show the retracted one in its place.
func applyCap(
	findings []crmcontracts.Company360Suggestion,
) ([]crmcontracts.Company360Suggestion, int) {
	if len(findings) > maxAdvice {
		return findings[:maxAdvice], len(findings) - maxAdvice
	}
	return findings, 0
}
