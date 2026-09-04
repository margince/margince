// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package leaddraft_test

// What the service refuses, and what it spends before refusing.

import (
	"context"
	"errors"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/leaddraft"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// leadReader answers one lead and records what it was asked.
type leadReader struct {
	lead     crmcontracts.Lead
	err      error
	archived storekit.ArchivedFilter
	asked    bool
}

func (r *leadReader) GetLead(
	_ context.Context, _ ids.LeadID, archived storekit.ArchivedFilter,
) (crmcontracts.Lead, error) {
	r.asked = true
	r.archived = archived
	return r.lead, r.err
}

// correspondence records whether the timeline was read at all, which is how a
// test can say a refusal happened BEFORE the reads it would have cost.
type correspondence struct {
	rows  []crmcontracts.Activity
	asked bool
}

func (c *correspondence) ForLead(context.Context, ids.LeadID) ([]crmcontracts.Activity, error) {
	c.asked = true
	return c.rows, nil
}

func humanCtx() context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:u-1",
	})
}

func agentCtx() context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:test",
	})
}

var someLead = ids.From[ids.LeadKind](ids.MustParse("019fe7ae-0000-7000-8000-000000000001"))

// Drafting spends the workspace's model budget on prose for a person to send
// under their own name, so an agent is refused — and refused before the lead is
// read, not after.
func TestAnAgentIsRefusedBeforeAnythingIsRead(t *testing.T) {
	t.Parallel()
	leads := &leadReader{lead: lead(nil)}
	acts := &correspondence{}

	_, err := leaddraft.NewService(leads, acts, nil).Draft(agentCtx(), someLead, leaddraft.Request{})

	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("an agent got %v, want ErrPermissionDenied", err)
	}
	if leads.asked {
		t.Error("the lead was read for a caller who may not draft")
	}
}

// A draft addressed to nobody is not a message. The refusal lands before the
// correspondence read and before any model call, so a lead with no address
// costs nothing to refuse.
func TestALeadWithNoAddressIsRefusedBeforeTheModel(t *testing.T) {
	t.Parallel()
	leads := &leadReader{lead: lead(func(l *crmcontracts.Lead) { l.Email = nil })}
	acts := &correspondence{}

	_, err := leaddraft.NewService(leads, acts, nil).Draft(humanCtx(), someLead, leaddraft.Request{})

	if err == nil {
		t.Fatal("a lead with no address drafted anyway")
	}
	if acts.asked {
		t.Error("the correspondence was read for a lead there is nobody to write to")
	}
}

// An address recorded as the empty string is the same fact as none. A record
// that carries the column and nothing in it is not a reachable lead, and
// treating it as one would draft a message the send gate refuses later.
func TestAnEmptyAddressIsTheSameAsNone(t *testing.T) {
	t.Parallel()
	blank := openapi_types.Email("")
	leads := &leadReader{lead: lead(func(l *crmcontracts.Lead) { l.Email = &blank })}

	_, err := leaddraft.NewService(leads, &correspondence{}, nil).
		Draft(humanCtx(), someLead, leaddraft.Request{})

	if err == nil {
		t.Fatal("a lead whose address is the empty string drafted anyway")
	}
}

// A terminal lead is not a record to open a new conversation from. Both
// closures archive the row — disqualified, and promoted to a contact — and the
// promoted one's correspondence belongs to the person it became.
func TestATerminalLeadIsNotDraftedTo(t *testing.T) {
	t.Parallel()
	leads := &leadReader{lead: lead(nil)}

	if _, err := leaddraft.NewService(leads, &correspondence{}, nil).
		Draft(humanCtx(), someLead, leaddraft.Request{}); err != nil {
		t.Fatalf("a live lead was refused: %v", err)
	}
	if leads.archived != storekit.LiveOnly {
		t.Errorf("the lead was read with filter %v, want LiveOnly — an archived lead "+
			"would otherwise be drafted to", leads.archived)
	}
}

// The lead's read failure is the caller's answer, unchanged. A lead this caller
// may not see must refuse as not-found rather than degrade to a draft written
// from nothing.
func TestALeadTheCallerCannotSeeRefuses(t *testing.T) {
	t.Parallel()
	leads := &leadReader{err: apperrors.ErrNotFound}
	acts := &correspondence{}

	_, err := leaddraft.NewService(leads, acts, nil).Draft(humanCtx(), someLead, leaddraft.Request{})

	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("got %v, want the store's own refusal", err)
	}
	if acts.asked {
		t.Error("the correspondence was read for a lead the caller cannot see")
	}
}

// With no model lane the draft still arrives, from persondraft's deterministic
// floor. A rep who pressed the button on a deployment running no model gets a
// short opener to edit rather than a refusal.
func TestWithNoModelLaneTheFloorWrites(t *testing.T) {
	t.Parallel()
	leads := &leadReader{lead: lead(nil)}

	draft, err := leaddraft.NewService(leads, &correspondence{}, nil).
		Draft(humanCtx(), someLead, leaddraft.Request{})
	if err != nil {
		t.Fatalf("the floor refused: %v", err)
	}
	if draft.Body == "" {
		t.Error("the floor wrote no body")
	}
	if draft.GeneratedBy == crmcontracts.Model {
		t.Error("a draft with no lane claims a model wrote it")
	}
	// The Art. 50 disclosure rides only a model-written draft. A deterministic
	// one carries none, because there is nothing to disclose.
	if draft.AiDisclosure != nil {
		t.Errorf("a deterministic draft carried the AI disclosure %q", *draft.AiDisclosure)
	}
}
