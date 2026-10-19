// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package leaddraft_test

// A lead's draft cites its recipient as the lead, on both writers.

import (
	"context"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/contactdraft"
	"github.com/margince/margince/backend/internal/compose/leaddraft"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// answeringLane answers every request with one fixed reply.
type answeringLane struct{ answer string }

func (l answeringLane) Complete(context.Context, model.Request) (model.Response, error) {
	return model.Response{Text: l.answer}, nil
}

func draftWith(t *testing.T, lane contactdraft.Completer) (crmcontracts.CompanyEmailDraft, []datasource.EntityRef) {
	t.Helper()
	draft, grounding, err := leaddraft.NewService(&leadReader{lead: lead(nil)}, &correspondence{}, lane).
		Draft(humanCtx(), someLead, leaddraft.Request{})
	if err != nil {
		t.Fatalf("draft: %v", err)
	}
	return draft, grounding
}

func recipientRef(t *testing.T, draft crmcontracts.CompanyEmailDraft) *crmcontracts.CompanyBriefEvidence {
	t.Helper()
	for _, reason := range draft.Reasoning {
		if reason.Kind == crmcontracts.AccountDraftReasonKindRecipient {
			return reason.EvidenceRef
		}
	}
	t.Fatalf("the draft names no recipient: %+v", draft.Reasoning)
	return nil
}

// followsToTheLead follows the reference by its type and id, as a client does.
// It must land on the lead the draft was read from.
func followsToTheLead(t *testing.T, ref *crmcontracts.CompanyBriefEvidence, grounding []datasource.EntityRef) {
	t.Helper()
	if ref == nil {
		t.Fatal("the recipient reason carries no reference")
	}
	if ref.EntityType != crmcontracts.CompanyBriefEvidenceEntityTypeLead || ids.UUID(ref.EntityId) != someLead.UUID {
		t.Fatalf("the recipient is cited as %s %s, want lead %s", ref.EntityType, ref.EntityId, someLead.UUID)
	}
	followed := datasource.EntityRef{Type: datasource.EntityType(ref.EntityType), ID: ids.UUID(ref.EntityId)}
	if !slices.Contains(grounding, followed) {
		t.Errorf("the reference %v names no record the draft was read from: %v", followed, grounding)
	}
}

func TestTheFloorCitesTheRecipientAsTheLead(t *testing.T) {
	t.Parallel()
	draft, grounding := draftWith(t, nil)

	followsToTheLead(t, recipientRef(t, draft), grounding)
}

func TestTheModelCitesTheRecipientAsTheLead(t *testing.T) {
	t.Parallel()
	lane := answeringLane{answer: `{"subject":"Your question","body":"Hi Dung,\n\nThanks for writing.",` +
		`"reasoning":[{"kind":"recipient","label":"Dung Ly Nguyen","entity_type":"lead",` +
		`"entity_id":"` + someLead.String() + `"}]}`}

	draft, grounding := draftWith(t, lane)

	if draft.GeneratedBy != crmcontracts.WrittenByModel {
		t.Fatalf("generated_by = %s, so the model's answer was not read", draft.GeneratedBy)
	}
	followsToTheLead(t, recipientRef(t, draft), grounding)
}

// A lead's id labelled as a contact names a record that does not exist.
// The reason is dropped instead of becoming a chip that opens nothing.
func TestAModelThatCitesTheLeadAsAContactLosesThatReason(t *testing.T) {
	t.Parallel()
	lane := answeringLane{answer: `{"subject":"Your question","body":"Hi Dung,\n\nThanks for writing.",` +
		`"reasoning":[{"kind":"recipient","label":"Dung Ly Nguyen","entity_type":"contact",` +
		`"entity_id":"` + someLead.String() + `"}]}`}

	draft, _ := draftWith(t, lane)

	if draft.GeneratedBy != crmcontracts.WrittenByModel {
		t.Fatalf("generated_by = %s, so the model's answer was not read", draft.GeneratedBy)
	}
	for _, reason := range draft.Reasoning {
		if ref := reason.EvidenceRef; ref != nil && ref.EntityType == crmcontracts.CompanyBriefEvidenceEntityTypeContact {
			t.Errorf("the lead's id survived labelled as a contact: %+v", reason)
		}
	}
}
