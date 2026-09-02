// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// ownedFiring is the context an OWNED automation's Apply runs under: the system
// actor with the owner bound, which is what withSendingOwner builds.
//
// A bare context is not that. It models a firing no human owns, which
// draft_email refuses — so a drafting test written on one would stop at that
// refusal and prove nothing about the case it is named for.
func ownedFiring() context.Context {
	ctx := principal.WithActor(context.Background(),
		principal.Principal{Type: principal.PrincipalSystem, ID: systemActor})
	return principal.WithSendingHuman(ctx, ids.NewV7())
}

// fakeComms is a DB-free stand-in for the Comms seam: records every
// DraftEmail call. The seam declares no send verb at all, so "draft_email
// never sends" is a structural property of this interface, not merely a
// behavioral one — there is nothing here a test could call to send.
type fakeComms struct {
	subject, body string
	err           error
	// address is what ReplyAddress answers. It defaults to a real one below
	// rather than to the empty string: a fake that hands back "" would let a
	// draft stage with no addressee and agree with exactly the defect the
	// resolution exists to prevent.
	address      string
	addressErr   error
	addressCalls []ids.UUID
	calls        []struct {
		anchor ids.UUID
		intent string
	}
}

// replyAddressDefault is what the fake answers unless a test asks for another
// outcome, so every test that does not care about addressing still exercises a
// draft that could actually have been sent.
const replyAddressDefault = "counterparty@example.com"

func (f *fakeComms) ReplyAddress(_ context.Context, anchor ids.UUID) (string, error) {
	f.addressCalls = append(f.addressCalls, anchor)
	if f.addressErr != nil {
		return "", f.addressErr
	}
	if f.address == "" {
		return replyAddressDefault, nil
	}
	return f.address, nil
}

func (f *fakeComms) DraftEmail(_ context.Context, anchor ids.UUID, intent string) (string, string, error) {
	f.calls = append(f.calls, struct {
		anchor ids.UUID
		intent string
	}{anchor, intent})
	return f.subject, f.body, f.err
}

// fakeNotifier is a DB-free stand-in for the Notifier seam: this repo
// wires none in compose, but the seam must still work once something
// does, so its wired path gets its own test.
type fakeNotifier struct {
	err   error
	calls []struct {
		recipient     ids.UUID
		subject, body string
	}
}

func (f *fakeNotifier) Notify(_ context.Context, recipient ids.UUID, subject, body string) error {
	f.calls = append(f.calls, struct {
		recipient     ids.UUID
		subject, body string
	}{recipient, subject, body})
	return f.err
}

// fakeUpdateProvider implements only the write path
// datasource.SystemOfRecordProvider tests here ever reach: Update.
// Every other method panics — a test that reaches one is exercising the
// wrong branch, and a panic says so louder than a wrong zero value would.
type fakeUpdateProvider struct {
	err   error
	calls []datasource.UpdateInput
}

func (p *fakeUpdateProvider) Update(_ context.Context, in datasource.UpdateInput) (datasource.EntityRef, error) {
	p.calls = append(p.calls, in)
	return in.Ref, p.err
}

func (p *fakeUpdateProvider) Read(_ context.Context, ref datasource.EntityRef) (datasource.Record, error) {
	return ownerlessRecord(ref), nil
}

// ownerlessRecord is what the doubles in this package answer Provider.Read
// with: the record that was asked for, carrying no fields.
//
// No owner is the honest answer. Every case here is about how many tasks a
// firing mints and what it names them after; the task effect reads the
// triggering record only to ATTRIBUTE the task, so "nobody owns it" is a
// complete answer to the only question asked of it.
//
// A body rather than the embedded interface's nil: a double that embeds an
// interface panics on every method it does not write, and a panic is not a
// loud failure in a test binary — it is the end of the binary. One unmodelled
// method stopped every other test in this package from running.
func ownerlessRecord(ref datasource.EntityRef) datasource.Record {
	return datasource.Record{Ref: ref, Fields: json.RawMessage(`{}`)}
}

func (p *fakeUpdateProvider) Search(context.Context, datasource.SearchQuery) (datasource.SearchResult, error) {
	panic("fakeUpdateProvider: Search not stubbed for this test")
}

func (p *fakeUpdateProvider) ListObjects(context.Context) ([]datasource.ObjectDef, error) {
	panic("fakeUpdateProvider: ListObjects not stubbed for this test")
}

func (p *fakeUpdateProvider) ListFields(context.Context, datasource.EntityType) ([]datasource.FieldDef, error) {
	panic("fakeUpdateProvider: ListFields not stubbed for this test")
}

func (p *fakeUpdateProvider) RunReport(context.Context, datasource.ReportPlan) (datasource.ReportResult, error) {
	panic("fakeUpdateProvider: RunReport not stubbed for this test")
}

func (p *fakeUpdateProvider) StageSemantic(context.Context, ids.UUID) (string, ids.UUID, error) {
	panic("fakeUpdateProvider: StageSemantic not stubbed for this test")
}

func (p *fakeUpdateProvider) Create(context.Context, datasource.CreateInput) (datasource.EntityRef, error) {
	panic("fakeUpdateProvider: Create not stubbed for this test")
}

func (p *fakeUpdateProvider) AdvanceDeal(context.Context, datasource.AdvanceDealInput) (datasource.EntityRef, error) {
	panic("fakeUpdateProvider: AdvanceDeal not stubbed for this test")
}

func (p *fakeUpdateProvider) Archive(context.Context, datasource.EntityRef) (datasource.EntityRef, error) {
	panic("fakeUpdateProvider: Archive not stubbed for this test")
}

func (p *fakeUpdateProvider) Merge(context.Context, datasource.MergeInput) (datasource.EntityRef, error) {
	panic("fakeUpdateProvider: Merge not stubbed for this test")
}

func (p *fakeUpdateProvider) PromoteLead(context.Context, ids.UUID, string, *string) (datasource.EntityRef, bool, error) {
	panic("fakeUpdateProvider: PromoteLead not stubbed for this test")
}

func (p *fakeUpdateProvider) Freshness(context.Context, datasource.EntityRef) (datasource.FreshnessInfo, error) {
	panic("fakeUpdateProvider: Freshness not stubbed for this test")
}

var _ datasource.SystemOfRecordProvider = (*fakeUpdateProvider)(nil)

// --- notify ---

func TestApplyActionsNotifyWithNoTransportReturnsTheHonestSentinel(t *testing.T) {
	action := workflow.Action{Kind: workflow.ActionNotify, Args: json.RawMessage(`{}`)}

	_, err := ApplyActions(context.Background(), Executors{}, workflow.Effect{Actions: []workflow.Action{action}})

	if !errors.Is(err, ErrNoNotificationTransport) {
		t.Fatalf("ApplyActions err = %v, want ErrNoNotificationTransport (no Notifier wired)", err)
	}
}

func TestApplyActionsNotifyWithAWiredTransportDelivers(t *testing.T) {
	recipient := ids.NewV7()
	notifier := &fakeNotifier{}
	action := workflow.Action{
		Kind: workflow.ActionNotify,
		Args: json.RawMessage(`{"recipient":"` + recipient.String() + `","subject":"Heads up","body":"a deal moved"}`),
	}

	applied, err := ApplyActions(context.Background(), Executors{Notifier: notifier}, workflow.Effect{Actions: []workflow.Action{action}})
	if err != nil {
		t.Fatalf("ApplyActions err = %v, want nil", err)
	}
	if len(applied) != 1 {
		t.Fatalf("applied = %v, want the one notify action", applied)
	}
	if len(notifier.calls) != 1 {
		t.Fatalf("Notify called %d times, want exactly 1", len(notifier.calls))
	}
	got := notifier.calls[0]
	if got.recipient != recipient || got.subject != "Heads up" || got.body != "a deal moved" {
		t.Errorf("Notify called with %+v, want recipient=%v subject=%q body=%q", got, recipient, "Heads up", "a deal moved")
	}
}

func TestApplyActionsNotifyNeverSwallowsADeliveryFailure(t *testing.T) {
	notifyErr := errors.New("smtp: connection refused")
	notifier := &fakeNotifier{err: notifyErr}
	action := workflow.Action{Kind: workflow.ActionNotify, Args: json.RawMessage(`{}`)}

	_, err := ApplyActions(context.Background(), Executors{Notifier: notifier}, workflow.Effect{Actions: []workflow.Action{action}})

	if !errors.Is(err, notifyErr) {
		t.Fatalf("ApplyActions err = %v, want it to wrap %v", err, notifyErr)
	}
}

// --- draft_email ---

// TestApplyActionsDraftEmailCapturesTheDraftInTheAppliedRecord is the
// anti-fake-success guard: Comms.DraftEmail is pure compute and the async
// automation path has no agent to receive its text, so the composed draft
// MUST land durably on the applied action (→ workflow_run.applied), or a
// run reporting 'applied' would have silently dropped its entire effect.
// This fails against compute-and-discard: the bare planned Args carry only
// the intent, never the composed subject/body.
func TestApplyActionsDraftEmailCapturesTheDraftInTheAppliedRecord(t *testing.T) {
	comms := &fakeComms{subject: "Re: hello", body: "following up"}
	approvals := &fakeApprovals{id: ids.New[ids.ApprovalKind]()}
	anchor := ids.NewV7()
	action := workflow.Action{
		Kind:   workflow.ActionDraftEmail,
		Target: datasource.EntityRef{Type: datasource.EntityActivity, ID: anchor},
		Args:   json.RawMessage(`{"intent":"nudge toward a decision","consent_purpose":"business_correspondence"}`),
	}

	applied, err := ApplyActions(ownedFiring(),
		Executors{Comms: comms, Approvals: approvals},
		workflow.Effect{Actions: []workflow.Action{action}})
	// Drafting composes AND stages, so the firing suspends. The artifact below
	// must survive that suspension: parking a run is not a reason to lose what
	// the run produced.
	var staged *workflow.StagedApprovalError
	if !errors.As(err, &staged) {
		t.Fatalf("ApplyActions err = %v, want a StagedApprovalError — the send waits for a human", err)
	}
	if len(applied) != 1 {
		t.Fatalf("applied = %v, want the one draft_email action", applied)
	}
	if len(comms.calls) != 1 {
		t.Fatalf("DraftEmail called %d times, want exactly 1", len(comms.calls))
	}
	got := comms.calls[0]
	if got.anchor != anchor || got.intent != "nudge toward a decision" {
		t.Errorf("DraftEmail called with %+v, want anchor=%v intent=%q", got, anchor, "nudge toward a decision")
	}

	// The composed draft rides the applied action durably — this is what
	// makes 'applied' honest for a firing whose only effect is the text.
	var rec struct {
		Intent  string `json:"intent"`
		Subject string `json:"draft_subject"`
		Body    string `json:"draft_body"`
	}
	if err := json.Unmarshal(applied[0].Args, &rec); err != nil {
		t.Fatalf("applied draft_email Args do not decode: %v", err)
	}
	if rec.Subject != "Re: hello" || rec.Body != "following up" {
		t.Errorf("applied draft = (subject=%q, body=%q), want the composed (%q, %q) durably captured — a discarded draft under 'applied' is a fake success", rec.Subject, rec.Body, "Re: hello", "following up")
	}
	if rec.Intent != "nudge toward a decision" {
		t.Errorf("applied draft dropped the requested intent %q — got %q", "nudge toward a decision", rec.Intent)
	}

	// The other half: the SEND is what waits, carrying everything a release
	// needs. A staging that showed only the words would be a decision a human
	// cannot actually take — and one missing the anchor could not be sent at
	// all, because the release effect is never handed the approval's target.
	if len(approvals.calls) != 1 {
		t.Fatalf("Stage called %d times, want exactly 1", len(approvals.calls))
	}
	staging := approvals.calls[0]
	if staging.Kind != HeldDraftKind {
		t.Errorf("staged kind = %q, want %q — sharing send_email's kind would waive its version pin too", staging.Kind, HeldDraftKind)
	}
	if !staging.JoinPending {
		t.Error("staged without JoinPending: an at-least-once redelivery would add a second copy of this message to the inbox")
	}
	var proposal HeldDraftProposal
	if err := json.Unmarshal(staging.ProposedChange, &proposal); err != nil {
		t.Fatalf("staged proposal does not decode: %v", err)
	}
	if proposal.AnchorActivityID != anchor {
		t.Errorf("staged anchor = %v, want %v — the release reads it from the payload, never from the target", proposal.AnchorActivityID, anchor)
	}
	if proposal.To != replyAddressDefault {
		t.Errorf("staged to = %q, want the resolved counterparty %q", proposal.To, replyAddressDefault)
	}
	if proposal.ConsentPurpose != "business_correspondence" {
		t.Errorf("staged consent_purpose = %q, want the declared one — a send with no purpose cannot pass the gate", proposal.ConsentPurpose)
	}
	if proposal.Subject != "Re: hello" || proposal.Body != "following up" {
		t.Errorf("staged message = (%q, %q), want the composed draft", proposal.Subject, proposal.Body)
	}
}

func TestApplyActionsDraftEmailNeverSwallowsADraftFailure(t *testing.T) {
	draftErr := errors.New("activities: anchor not found")
	comms := &fakeComms{err: draftErr}
	approvals := &fakeApprovals{id: ids.New[ids.ApprovalKind]()}
	action := workflow.Action{
		Kind:   workflow.ActionDraftEmail,
		Target: datasource.EntityRef{Type: datasource.EntityActivity, ID: ids.NewV7()},
		Args:   json.RawMessage(`{"consent_purpose":"business_correspondence"}`),
	}

	_, err := ApplyActions(ownedFiring(),
		Executors{Comms: comms, Approvals: approvals},
		workflow.Effect{Actions: []workflow.Action{action}})

	if !errors.Is(err, draftErr) {
		t.Fatalf("ApplyActions err = %v, want it to wrap %v", err, draftErr)
	}
	if len(approvals.calls) != 0 {
		t.Errorf("staged %d approvals after the draft failed — a message that was never composed must not appear in anyone's inbox", len(approvals.calls))
	}
}

// A draft_email instance that names no consent purpose refuses at compose time
// rather than staging a message no human could ever release: the send gate is
// default-deny per purpose, so a purposeless draft is one that fails at the
// moment somebody tries to approve it, which is the worst place to learn.
func TestApplyActionsDraftEmailRefusesWithoutAConsentPurpose(t *testing.T) {
	comms := &fakeComms{subject: "Re: hello", body: "following up"}
	approvals := &fakeApprovals{id: ids.New[ids.ApprovalKind]()}
	action := workflow.Action{
		Kind:   workflow.ActionDraftEmail,
		Target: datasource.EntityRef{Type: datasource.EntityActivity, ID: ids.NewV7()},
		Args:   json.RawMessage(`{"intent":"nudge toward a decision"}`),
	}

	_, err := ApplyActions(ownedFiring(),
		Executors{Comms: comms, Approvals: approvals},
		workflow.Effect{Actions: []workflow.Action{action}})

	var missing *MissingConsentPurposeError
	if !errors.As(err, &missing) {
		t.Fatalf("ApplyActions err = %v, want MissingConsentPurposeError", err)
	}
	if len(approvals.calls) != 0 {
		t.Errorf("staged %d approvals for a draft that can never be released", len(approvals.calls))
	}
	if len(comms.calls) != 0 {
		t.Error("composed a draft before checking the purpose — the certain refusal must run before the expensive work")
	}
}

// The addressee is resolved at STAGING, so a thread with no counterparty fails
// the firing where an operator can see it — instead of producing an inbox item
// that dies at the moment a human presses approve.
func TestApplyActionsDraftEmailRefusesWhenTheThreadHasNoAddress(t *testing.T) {
	noAddress := errors.New("activities: no counterparty address on this thread")
	comms := &fakeComms{subject: "Re: hello", body: "following up", addressErr: noAddress}
	approvals := &fakeApprovals{id: ids.New[ids.ApprovalKind]()}
	action := workflow.Action{
		Kind:   workflow.ActionDraftEmail,
		Target: datasource.EntityRef{Type: datasource.EntityActivity, ID: ids.NewV7()},
		Args:   json.RawMessage(`{"consent_purpose":"business_correspondence"}`),
	}

	_, err := ApplyActions(ownedFiring(),
		Executors{Comms: comms, Approvals: approvals},
		workflow.Effect{Actions: []workflow.Action{action}})

	if !errors.Is(err, noAddress) {
		t.Fatalf("ApplyActions err = %v, want it to wrap %v", err, noAddress)
	}
	if len(approvals.calls) != 0 {
		t.Errorf("staged %d approvals for a message with nowhere to go", len(approvals.calls))
	}
}

// A composition that wired no staging seam refuses honestly instead of
// dereferencing nil. The draft is already composed by then, so the alternative
// to an error is a crash — or, worse, silently discarding it and reporting the
// run healthy.
func TestApplyActionsDraftEmailRefusesWithNoStagingSeam(t *testing.T) {
	comms := &fakeComms{subject: "Re: hello", body: "following up"}
	action := workflow.Action{
		Kind:   workflow.ActionDraftEmail,
		Target: datasource.EntityRef{Type: datasource.EntityActivity, ID: ids.NewV7()},
		Args:   json.RawMessage(`{"consent_purpose":"business_correspondence"}`),
	}

	_, err := ApplyActions(ownedFiring(), Executors{Comms: comms},
		workflow.Effect{Actions: []workflow.Action{action}})

	if !errors.Is(err, ErrNoApprovalStaging) {
		t.Fatalf("ApplyActions err = %v, want ErrNoApprovalStaging", err)
	}
}

// --- assign_owner's dynamic tier ---

// TestApplyAssignOwnerSingleEntityWritesThroughProviderUpdate proves the
// 🟢 branch: the honest single-entity scope every real firing carries
// today writes straight through, never touching Approvals.
func TestApplyAssignOwnerSingleEntityWritesThroughProviderUpdate(t *testing.T) {
	provider := &fakeUpdateProvider{}
	target := datasource.EntityRef{Type: datasource.EntityDeal, ID: ids.NewV7()}
	action := workflow.Action{Kind: workflow.ActionAssignOwner, Target: target, Args: json.RawMessage(`{"owner_id":"` + ids.NewV7().String() + `"}`)}

	err := applyAssignOwner(context.Background(), Executors{Provider: provider}, action, AssignOwnerScope{Bulk: false})
	if err != nil {
		t.Fatalf("applyAssignOwner err = %v, want nil", err)
	}
	if len(provider.calls) != 1 {
		t.Fatalf("provider.Update called %d times, want exactly 1", len(provider.calls))
	}
	if provider.calls[0].Ref != target {
		t.Errorf("provider.Update Ref = %v, want %v", provider.calls[0].Ref, target)
	}
}

// TestApplyAssignOwnerAtScaleStagesInsteadOfWriting proves the 🟡
// branch against a SYNTHETIC scaled scope (AUTO-T07): no shipped
// automation produces Bulk == true today (AssignOwnerScope's doc), so
// this is the resolver's escalation path exercised the only honest way
// available — a fake provider that panics on Update proves the write
// side is never reached, exactly like the 🟡 kinds' own staging tests.
func TestApplyAssignOwnerAtScaleStagesInsteadOfWriting(t *testing.T) {
	fake := &fakeApprovals{id: ids.New[ids.ApprovalKind]()}
	target := datasource.EntityRef{Type: datasource.EntityDeal, ID: ids.NewV7()}
	action := workflow.Action{Kind: workflow.ActionAssignOwner, Target: target, Args: json.RawMessage(`{"owner_id":"` + ids.NewV7().String() + `"}`)}

	err := applyAssignOwner(context.Background(), Executors{Approvals: fake}, action, AssignOwnerScope{Bulk: true})

	var staged *workflow.StagedApprovalError
	if !errors.As(err, &staged) {
		t.Fatalf("applyAssignOwner err = %v, want a *workflow.StagedApprovalError", err)
	}
	if staged.ApprovalID != fake.id {
		t.Errorf("StagedApprovalError.ApprovalID = %v, want %v", staged.ApprovalID, fake.id)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("Stage called %d times, want exactly 1", len(fake.calls))
	}
}

// An automation nobody owns refuses to draft, rather than staging a card
// nobody can decide.
//
// Releasing a held draft SENDS it from the approving human's own mailbox, so
// approvals narrows the card to the person it goes out as. A firing with no
// owner names nobody, and the three things it could do are: stage a card
// decidable by nobody, which rots in the inbox; stage one decidable by anyone,
// which is the defect that narrowing removes; or refuse where an operator can
// see it.
//
// This is not hypothetical. The seeded post_meeting_recap starter drafts email
// and catalog templates are seeded with no owner_id, so this is the shipped
// configuration until somebody assigns one.
func TestApplyActionsDraftEmailRefusesForAnAutomationWithNoOwner(t *testing.T) {
	comms := &fakeComms{subject: "Re: hello", body: "following up"}
	approvals := &fakeApprovals{id: ids.New[ids.ApprovalKind]()}
	action := workflow.Action{
		Kind:   workflow.ActionDraftEmail,
		Target: datasource.EntityRef{Type: datasource.EntityActivity, ID: ids.NewV7()},
		Args:   json.RawMessage(`{"intent":"recap the meeting","consent_purpose":"business_correspondence"}`),
	}

	// The system actor with NO owner bound — what withSendingOwner leaves for a
	// firing whose automation has no owner_id.
	ctx := principal.WithActor(context.Background(),
		principal.Principal{Type: principal.PrincipalSystem, ID: systemActor})

	_, err := ApplyActions(ctx,
		Executors{Comms: comms, Approvals: approvals},
		workflow.Effect{Actions: []workflow.Action{action}})

	var missing *MissingDraftOwnerError
	if !errors.As(err, &missing) {
		t.Fatalf("ApplyActions err = %v, want MissingDraftOwnerError", err)
	}
	if len(approvals.calls) != 0 {
		t.Errorf("staged %d approvals for a draft nobody could release", len(approvals.calls))
	}
	if len(comms.calls) != 0 {
		t.Error("composed a draft before checking the owner — the certain refusal must run before the " +
			"model call, the same order the consent-purpose check keeps")
	}
}
