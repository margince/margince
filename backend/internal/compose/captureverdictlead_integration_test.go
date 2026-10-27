// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A live lead is never offered as a new contact. An offer accepted after the
// lead appeared makes no contact either. The lead is their record.

import (
	"context"
	"log/slog"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func seedReviewLead(t *testing.T, e *integration.Env, address string) {
	t.Helper()
	if _, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{
		FullName: new("Dana Lead"), Email: new(address), Source: "manual",
	}); err != nil {
		t.Fatalf("creating the lead: %v", err)
	}
}

func TestALeadsAddressIsNotOfferedAsANewContact(t *testing.T) {
	e := integration.Setup(t)
	const address = "dana@leadco.example"
	seedReviewLead(t, e, address)
	activityID := seedCapturedMail(t, e, address, "about your services")
	dispositionID := seedPendingDisposition(t, e, address, "leadco.example", activityID)
	retireToUnsure(t, e, dispositionID)

	engine := NewCounterpartyVerdictEngine(e.Pool, &scriptedVerdictBrain{}, CaptureConfig{}, slog.Default())
	if err := engine.StageReviewsWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("staging reviews: %v", err)
	}
	if n := counterpartyProposals(t, e, dispositionID); n != 0 {
		t.Errorf("%d contact proposals for a live lead's address, want none", n)
	}
	if got := dispositionStatus(t, e, dispositionID); got != capture.PendingStatusReal {
		t.Errorf("the lead's question is %q, want it closed as real: left unsure it returns on every pass", got)
	}
}

func TestAnOfferAcceptedAfterTheSenderBecameALeadMakesNoContact(t *testing.T) {
	e := integration.Setup(t)
	const address = "rui@leadco.example"
	activityID := seedCapturedMail(t, e, address, "about your services")
	dispositionID := seedPendingDisposition(t, e, address, "leadco.example", activityID)
	retireToUnsure(t, e, dispositionID)
	svc := approvalsServiceWithEffects(e.Pool)
	engine := NewCounterpartyVerdictEngine(e.Pool, &scriptedVerdictBrain{}, CaptureConfig{}, slog.Default())
	if err := engine.StageReviewsWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("staging reviews: %v", err)
	}
	approvalID := stagedProposalID(t, e, dispositionID)
	seedReviewLead(t, e, address)

	decider := e.As(e.Rep1, nil, integration.AdminPerms)
	if _, err := svc.Decide(decider, ids.From[ids.ApprovalKind](approvalID), true, nil); err != nil {
		t.Fatalf("approving the counterparty proposal: %v", err)
	}
	if held := contactsHolding(t, e, address); len(held) != 0 {
		t.Errorf("contacts %v after the accept, want none: the sender is a lead", held)
	}
	if got := dispositionStatus(t, e, dispositionID); got != capture.PendingStatusReal {
		t.Errorf("disposition status after accept = %q, want real", got)
	}
}
