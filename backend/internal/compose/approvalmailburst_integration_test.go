// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// stageBurst stages one close-date correction on each of n fresh deals, with a
// summary of its own, and lets the fan-out write each decider's notice.
func (e *noticeMailEnv) stageBurst(t *testing.T, summaries ...string) []ids.UUID {
	t.Helper()
	e.grantRole(t, e.e.AdminUser)
	for i, summary := range summaries {
		deal := e.deal
		if i > 0 {
			deal = e.dealBeside(t, summary)
		}
		proposal, err := json.Marshal(deals.CloseDateCorrection{
			DealID: ids.From[ids.DealKind](deal), ExpectedCloseDate: "2026-12-01", Basis: "the date has passed",
		})
		if err != nil {
			t.Fatal(err)
		}
		_, env := e.stage(e.proposerCtx(ids.Nil), t, approvals.StageInput{
			Kind: deals.CloseDateCorrectionKind, ProposedChange: proposal, DiffHash: "h-" + deal.String(),
			TargetType: "deal", TargetID: deal, Summary: summary,
		})
		e.deliver(t, env)
	}
	var notices []ids.UUID
	for _, row := range e.delivered(t) {
		notices = append(notices, row.id)
	}
	if len(notices) != len(summaries) {
		t.Fatalf("the fan-out wrote %d notices for %d approvals", len(notices), len(summaries))
	}
	return notices
}

// dealBeside creates one more deal in the seeded deal's pipeline and stage.
func (e *noticeMailEnv) dealBeside(t *testing.T, name string) ids.UUID {
	t.Helper()
	pipeline, err := ids.Parse(e.e.WsScalar(t, `SELECT pipeline_id::text FROM deal WHERE id = $1`, e.deal))
	if err != nil {
		t.Fatal(err)
	}
	stage, err := ids.Parse(e.e.WsScalar(t, `SELECT stage_id::text FROM deal WHERE id = $1`, e.deal))
	if err != nil {
		t.Fatal(err)
	}
	owner := ids.From[ids.UserKind](e.e.AdminUser)
	deal, err := e.e.Deals.CreateDeal(e.e.Admin(), deals.CreateDealInput{
		Name: name, PipelineID: ids.From[ids.PipelineKind](pipeline),
		StageID: ids.From[ids.StageKind](stage), OwnerID: &owner, Source: "manual",
	})
	if err != nil {
		t.Fatalf("seeding a deal beside the first: %v", err)
	}
	return ids.UUID(deal.Id)
}

// An agent's burst of approvals reaches its decider as one message, and the
// jobs of the notices it carried send nothing more.
func TestABurstOfApprovalsLeavesInOneMessage(t *testing.T) {
	e := setupNoticeMail(t)
	summaries := []string{"Fleet renewal date passed", "Harbor renewal date passed", "Depot renewal date passed"}
	notices := e.stageBurst(t, summaries...)

	for _, notice := range notices {
		e.work(t, notice)
	}

	if got := e.relay.count(); got != 1 {
		t.Fatalf("the relay was handed %d messages for one burst, want 1", got)
	}
	if !strings.Contains(e.relay.subjects[0], "3 requests") {
		t.Errorf("the subject %q does not count the burst", e.relay.subjects[0])
	}
	for _, summary := range summaries {
		if !strings.Contains(e.relay.bodies[0], summary) {
			t.Errorf("the message does not carry %q:\n%s", summary, e.relay.bodies[0])
		}
	}
	for _, notice := range notices {
		if stamp, cause := e.emailState(t, notice); stamp == nil || cause != nil {
			t.Errorf("notice %s: claim %v, cause %v; a carried notice is spent and not failed", notice, stamp, cause)
		}
	}
}

// A notice the decider already answered on screen is not carried.
func TestABurstLeavesOutWhatTheDeciderAlreadyRead(t *testing.T) {
	e := setupNoticeMail(t)
	notices := e.stageBurst(t, "Fleet renewal date passed", "Harbor renewal date passed")
	e.e.WsExec(t, `UPDATE notice SET read_at = now() WHERE id = $1`, notices[1])

	e.work(t, notices[0])

	if got := e.relay.count(); got != 1 {
		t.Fatalf("the relay was handed %d messages, want 1", got)
	}
	if body := e.relay.bodies[0]; strings.Contains(body, "Harbor renewal date passed") {
		t.Errorf("a notice read on screen was mailed:\n%s", body)
	}
	if strings.Contains(e.relay.subjects[0], "requests") {
		t.Errorf("a single carried notice was mailed as a burst: %q", e.relay.subjects[0])
	}
}
