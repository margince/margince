// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// stageMailedRequest captures a colleague's mailed promise and stages it
// unsure, so it is proposed to them, and returns the request as they read it.
func stageMailedRequest(t *testing.T) (*integration.Env, crmcontracts.Approval) {
	t.Helper()
	e, _ := seedMailedPromise(t, "active")
	e.WsExec(t, `UPDATE app_user SET display_name = 'Lena Fischer' WHERE id = $1`, e.Rep2)
	message, err := ids.Parse(e.WsScalar(t, `SELECT activity_id::text FROM activity_identity WHERE identity_kind='mail' AND identity_key='thread-seat@buyer.example'`))
	if err != nil {
		t.Fatal(err)
	}
	if err := runSignalPass(t, e, mailedCommitmentReply(t, message, e.Rep2, 0.75)); err != nil {
		t.Fatal(err)
	}
	id, err := ids.Parse(e.WsScalar(t, `SELECT id::text FROM approval WHERE kind = 'commitment_task'`))
	if err != nil {
		t.Fatal(err)
	}
	wire, err := NewApprovalsService(e.DB()).GetWire(e.As(e.Rep2, []ids.UUID{e.Team1}, integration.AdminPerms), ids.From[ids.ApprovalKind](id))
	if err != nil {
		t.Fatal(err)
	}
	return e, wire
}

func factsText(facts approvalMailFacts) string {
	var out []string
	for _, line := range facts.Lines {
		out = append(out, line.Label+": "+line.Value)
	}
	return facts.Heading + "\n" + strings.Join(out, "\n")
}

// The email names the company, the source mail, the promiser, the promise and
// why the reader is asked, in the card's own words.
func TestACommitmentEmailNamesWhatTheCardShowsItsPromiser(t *testing.T) {
	e, wire := stageMailedRequest(t)
	words := mailcopy.For("en")
	reader := e.As(e.Rep2, []ids.UUID{e.Team1}, integration.AdminPerms)
	facts, err := newApprovalPreviewReader(e.DB()).commitmentMailFacts(reader, wire, words, func(at time.Time) time.Time { return at.UTC() })
	if err != nil {
		t.Fatal(err)
	}
	text := factsText(facts)
	for _, want := range []string{
		"About: Acme", "Read from: Pricing, ", "Promised by: Lena Fischer",
		"In their words: " + seatStatusPromise, "Why you: " + reviewerSentence(wire.Preview.ReviewerReason, words),
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the email facts do not say %q:\n%s", want, text)
		}
	}
	if facts.Heading != "Acme" {
		t.Errorf("the heading is %q, want the company the promise is about", facts.Heading)
	}
	if strings.Contains(text, "Ines") {
		t.Errorf("the email names the contact:\n%s", text)
	}
}

// A seat who may not read the private source gets no line about it and no
// quote. One who may not see the company gets no heading.
func TestACommitmentEmailLeavesOutWhatTheReaderMayNotSee(t *testing.T) {
	e, wire := stageMailedRequest(t)
	e.WsExec(t, `UPDATE contact SET visibility = 'owner', owner_id = $1
		 WHERE id IN (SELECT contact_id FROM activity_link WHERE entity_type = 'contact' AND contact_id IS NOT NULL)`, e.Rep2)
	e.WsExec(t, `UPDATE company SET visibility = 'owner', owner_id = $1`, e.Rep2)
	stranger := e.As(e.Rep3, nil, integration.RepPerms)
	reader := newApprovalPreviewReader(e.DB())
	preview, err := reader.read(stranger, wire)
	if err != nil {
		t.Fatal(err)
	}
	wire.Preview = preview
	facts, err := reader.commitmentMailFacts(stranger, wire, mailcopy.For("en"), func(at time.Time) time.Time { return at })
	if err != nil {
		t.Fatal(err)
	}
	text := factsText(facts)
	for _, hidden := range []string{"Pricing", seatStatusPromise, "Acme"} {
		if strings.Contains(text, hidden) {
			t.Errorf("a seat who may not see it is told %q:\n%s", hidden, text)
		}
	}
	if !strings.Contains(text, "Promised by:") {
		t.Errorf("the facts lost the line the seat may see:\n%s", text)
	}
}

// Only the single email reads a commitment's facts. The digest line reads
// none, so no failed read of them can cost the seat the whole digest.
func TestADigestLineReadsNoCommitmentFacts(t *testing.T) {
	e, wire := stageMailedRequest(t)
	id := ids.From[ids.ApprovalKind](ids.UUID(wire.Id))
	attempt := notices.EmailAttempt{
		ApprovalID: &id, Notice: notices.Notice{Kind: notices.KindApprovalPending}, Recipient: ids.From[ids.UserKind](e.Rep2),
	}
	reader := e.As(e.Rep2, []ids.UUID{e.Team1}, integration.AdminPerms)
	for use, rich := range map[approvalNoticeUse]bool{forDigestLine: false, forSingleEmail: true} {
		_, facts, send, err := refreshApprovalNotice(reader, e.Pool, attempt, clocktest.Now(t), use)
		if err != nil || !send {
			t.Fatalf("refreshing the notice for use %v: send=%v, %v", use, send, err)
		}
		if facts.Rich != rich {
			t.Errorf("use %v read the commitment facts: %v, want %v", use, facts.Rich, rich)
		}
	}
}
