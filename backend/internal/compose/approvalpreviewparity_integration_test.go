// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The approval card, read_approval and the approval email are three readings
// of one preview. Each test reads one approval through all three as the member
// who decides it, and holds that each says the same will happen.

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// previewSurfaces is one approval as each surface shows it to its decider.
type previewSurfaces struct {
	card crmcontracts.ApprovalPreview
	tool agents.StagedApproval
	mail string
}

// readOnEverySurface reads the approval as its requested decider. The card is
// GetWire, and read_approval runs through the composed registry as their
// agent. The email is the notice it would send them.
func readOnEverySurface(t *testing.T, e *integration.Env, id ids.ApprovalID) previewSurfaces {
	t.Helper()
	staged, err := NewApprovalsService(e.DB()).GetWire(e.Admin(), id)
	if err != nil || staged.RequestedDeciderId == nil {
		t.Fatalf("reading who decides the approval: %+v, %v", staged.RequestedDeciderId, err)
	}
	decider := ids.UUID(*staged.RequestedDeciderId)
	member := storedMemberSeat(t, e, decider)
	card, err := NewApprovalsService(e.DB()).GetWire(member, id)
	if err != nil || card.Preview == nil {
		t.Fatalf("reading the card: %+v, %v", card.Preview, err)
	}

	seat, _ := principal.Actor(member)
	agent := principal.WithActor(member, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:preview-parity", SeatType: seat.SeatType,
		OnBehalfOf: decider, UserID: decider, Scopes: principal.NewScopeSet(principal.ScopeRead), Permissions: seat.Permissions,
	})
	raw, err := composedRegistry(e).Invoke(agent, "read_approval", fmt.Appendf(nil, `{"staged_action_id":%q}`, id.UUID))
	if err != nil {
		t.Fatalf("read_approval: %v", err)
	}
	var envelope agents.Envelope
	var tool agents.StagedApproval
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(envelope.Data, &tool); err != nil {
		t.Fatal(err)
	}

	attempt := notices.EmailAttempt{
		ApprovalID: &id, Notice: notices.Notice{Kind: notices.KindApprovalPending}, Recipient: ids.From[ids.UserKind](decider),
	}
	_, facts, send, err := refreshApprovalNotice(member, e.Pool, attempt, clocktest.Now(t), forSingleEmail)
	if err != nil || !send {
		t.Fatalf("the approval email: send=%v, %v", send, err)
	}
	return previewSurfaces{card: *card.Preview, tool: tool, mail: factsText(facts)}
}

// requireSameOutcome holds read_approval and the email to the card's state,
// changes, operands and effects.
func requireSameOutcome(t *testing.T, s previewSurfaces) {
	t.Helper()
	if s.tool.PreviewState != s.card.State {
		t.Errorf("read_approval says the preview is %q, the card %q", s.tool.PreviewState, s.card.State)
	}
	if len(s.tool.Changes) != len(s.card.Changes) {
		t.Errorf("read_approval names %d changes, the card %d", len(s.tool.Changes), len(s.card.Changes))
	}
	for i, change := range s.card.Changes {
		if i < len(s.tool.Changes) && (s.tool.Changes[i].Field != change.Field || !reflect.DeepEqual(s.tool.Changes[i].After, change.After)) {
			t.Errorf("read_approval change %d is %+v, the card's %+v", i, s.tool.Changes[i], change)
		}
	}
	var cardDetails []crmcontracts.ApprovalPreviewDetail
	if s.card.Details != nil {
		cardDetails = *s.card.Details
	}
	toolDetails := make([]crmcontracts.ApprovalPreviewDetail, 0, len(s.tool.Details))
	for _, detail := range s.tool.Details {
		toolDetails = append(toolDetails, crmcontracts.ApprovalPreviewDetail{Field: detail.Field, Value: detail.Value})
	}
	if len(toolDetails)+len(cardDetails) > 0 && !reflect.DeepEqual(toolDetails, cardDetails) {
		t.Errorf("read_approval lists the operands %+v, the card %+v", toolDetails, cardDetails)
	}
	shown := 0
	for _, detail := range cardDetails {
		if detail.Value.Kind == previewValueUnavailable {
			continue
		}
		if shown++; shown <= mailChangeLimit && !strings.Contains(s.mail, "\n"+agents.FieldLabel(textlang.English, detail.Field)+": ") {
			t.Errorf("the email does not list the operand %s:\n%s", detail.Field, s.mail)
		}
	}
	var cardEffects []string
	if s.card.Effects != nil {
		cardEffects = *s.card.Effects
	}
	if len(s.tool.Effects) != len(cardEffects) {
		t.Errorf("read_approval names the effects %+v, the card %v", s.tool.Effects, cardEffects)
	}
	words := mailcopy.For("en")
	for i, effect := range cardEffects {
		sentence := agents.EffectSentence(textlang.English, effect)
		if i < len(s.tool.Effects) && (s.tool.Effects[i].Effect != effect || s.tool.Effects[i].Label != sentence) {
			t.Errorf("read_approval effect %d is %+v, the card says %s: %q", i, s.tool.Effects[i], effect, sentence)
		}
		if !strings.Contains(s.mail, words.MailAlsoChanges+": "+sentence) {
			t.Errorf("the email does not say the effect %s:\n%s", effect, s.mail)
		}
	}
}

func TestEverySurfaceSaysWhatALeadPromotionWillDo(t *testing.T) {
	s := newPromotionSight(t, "rep", approverIsAdmin, "admin")
	rep1 := ids.From[ids.UserKind](s.e.Rep1)
	lead := s.lead(contacts.CreateLeadInput{
		FullName: new("Pia Parity"), Email: new("pia.parity@gmail.com"), CompanyName: new("Parity Works"), Source: "manual", OwnerID: &rep1,
	})
	id := s.stage(lead, fmt.Sprintf(`{"lead_id":%q,"trigger":"human_qualify"}`, lead.UUID))

	surfaces := readOnEverySurface(t, s.e, id)
	if surfaces.card.State != previewReadyState || surfaces.card.Effects == nil || !slices.Contains(*surfaces.card.Effects, "promote_lead_archive_source") {
		t.Fatalf("the card = %s %v, want a ready promotion that archives the lead", surfaces.card.State, surfaces.card.Effects)
	}
	if surfaces.card.Details == nil || len(*surfaces.card.Details) == 0 {
		t.Fatal("the card lists no operands, so the comparison proves nothing")
	}
	requireSameOutcome(t, surfaces)
}

func TestEverySurfaceSaysWhatAFieldChangeWillDo(t *testing.T) {
	e := integration.Setup(t)
	e.GrantRole(t, e.AdminUser, "admin")
	pipeline, open, _ := integration.DealFixture(t, e)
	deal := e.SeedDeal(t, "Parity renewal", pipeline, open, &e.AdminUser)
	amount, currency := int64(250000), "EUR"
	if _, err := e.Deals.UpdateDeal(e.Admin(), ids.From[ids.DealKind](deal), deals.UpdateDealInput{AmountMinor: &amount, Currency: &currency}); err != nil {
		t.Fatal(err)
	}
	raw, hash, err := diffhash.Canonical(fmt.Appendf(nil, `{"record_type":"deal","id":%q,"fields":{"amount_minor":300000}}`, deal))
	if err != nil {
		t.Fatal(err)
	}
	id, err := NewApprovalsService(e.DB()).Stage(e.Admin(), approvals.StageInput{
		Kind: "update_record", TargetType: "deal", TargetID: deal, ProposedChange: raw, DiffHash: hash,
	})
	if err != nil {
		t.Fatal(err)
	}

	surfaces := readOnEverySurface(t, e, id)
	if surfaces.card.State != previewReadyState || len(surfaces.card.Changes) != 1 {
		t.Fatalf("the card = %s %+v, want one ready change", surfaces.card.State, surfaces.card.Changes)
	}
	requireSameOutcome(t, surfaces)
	if !strings.Contains(surfaces.mail, "Value: 2,500.00 EUR → 3,000.00 EUR") {
		t.Errorf("the email does not say the change:\n%s", surfaces.mail)
	}
}
