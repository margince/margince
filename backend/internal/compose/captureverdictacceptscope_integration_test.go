// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Accepting a capture verdict card files the sender's mail onto the contact
// their address already belongs to. That contact must be one the decider can
// see, or accepting would file mail onto a colleague's private record.

import (
	"context"
	"errors"
	"maps"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const privateSender = "jonas@brandt.example"

// stagedSenderCard is a counterparty card for privateSender, whose address
// already belongs to a contact private to privateOwner.
type stagedSenderCard struct {
	*integration.Env
	svc      *approvals.Service
	approval ids.ApprovalID
	contact  ids.UUID
	mail     ids.UUID
}

func stageSenderCard(t *testing.T, privateOwner func(*integration.Env) ids.UUID) stagedSenderCard {
	t.Helper()
	e := integration.Setup(t)
	contact := seedCaptureOnlyContact(t, e, privateSender, privateOwner(e))
	mail := seedCapturedMail(t, e, privateSender, "Angebot")
	disposition := seedPendingDisposition(t, e, privateSender, "brandt.example", mail)
	// A card is offered for a sender the verdict pass could not settle.
	e.WsExec(t, `UPDATE capture_pending_counterparty SET status = 'unsure' WHERE id = $1`, disposition)
	svc := approvalsServiceWithEffects(e.Pool)
	staged, err := stageCounterpartyReview(e.Admin(), svc, approvalSummaryByLang["en"], capture.PendingCounterparty{
		ID: disposition, Email: privateSender, Domain: "brandt.example", ActivityID: mail, OwnerID: e.Rep1,
	})
	if err != nil {
		t.Fatalf("staging the card: %v", err)
	}
	return stagedSenderCard{Env: e, svc: svc, approval: ids.From[ids.ApprovalKind](staged), contact: contact, mail: mail}
}

// decider is Rep1, a rep in Team1 who may accept the card.
func (s stagedSenderCard) decider() context.Context {
	perms := integration.RepPerms
	perms.Objects = maps.Clone(integration.RepPerms.Objects)
	for _, object := range []string{"approval", "activity", "company"} {
		perms.Objects[object] = principal.ObjectGrant{Create: true, Read: true, Update: true}
	}
	return s.As(s.Rep1, []ids.UUID{s.Team1}, perms)
}

func (s stagedSenderCard) filedOntoContact(t *testing.T) int {
	t.Helper()
	return countIn(t, s.Env, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND contact_id = $2`,
		s.mail, s.contact)
}

// The address belongs to a contact private to a rep in another team. The
// accept is refused, no mail is filed onto it, and the card still waits.
func TestAcceptingASenderWhoseContactTheDeciderCannotSeeIsRefused(t *testing.T) {
	s := stageSenderCard(t, func(e *integration.Env) ids.UUID { return e.Rep3 })

	if _, err := s.svc.Decide(s.decider(), s.approval, true, nil); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("accepting a sender on a contact the decider cannot see: err = %v, want not found", err)
	}
	if n := s.filedOntoContact(t); n != 0 {
		t.Errorf("a refused accept filed the mail onto the private contact (%d link)", n)
	}
	if status := s.WsScalar(t, `SELECT status FROM approval WHERE id = $1`, s.approval.UUID); status != "pending" {
		t.Errorf("the refused card is %q, want it still pending", status)
	}
}

// The write asks again for a contact hidden after the precheck answered: run
// with no precheck in front of it, the effect alone files nothing.
func TestTheAcceptedSenderWriteRefusesAContactTheDeciderCannotSee(t *testing.T) {
	s := stageSenderCard(t, func(e *integration.Env) ids.UUID { return e.Rep3 })
	writeOnly := approvals.NewService(s.DB())
	writeOnly.WithEffect(counterpartyProposalKind, counterpartyAcceptEffect(writeOnly, newCounterpartyStore(s.Pool),
		newConnectorTagFiler(s.Pool), capture.NewPendingStore(InstallationDB(s.Pool)), nil))

	if _, err := writeOnly.Decide(s.decider(), s.approval, true, nil); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("writing a sender on a contact the decider cannot see: err = %v, want not found", err)
	}
	if n := s.filedOntoContact(t); n != 0 {
		t.Errorf("a refused write filed the mail onto the private contact (%d link)", n)
	}
}

// The contact is the decider's own, so the accept files the mail onto it.
func TestAcceptingASenderWhoseContactTheDeciderCanSeeFilesTheMail(t *testing.T) {
	s := stageSenderCard(t, func(e *integration.Env) ids.UUID { return e.Rep1 })

	if _, err := s.svc.Decide(s.decider(), s.approval, true, nil); err != nil {
		t.Fatalf("accepting: %v", err)
	}
	if n := s.filedOntoContact(t); n != 1 {
		t.Errorf("the accept filed the mail onto the decider's contact %d time(s), want once", n)
	}
}

// A read-only share shows the decider the contact. It gives no right to file
// mail onto it or publish it, which is what accepting would do.
func TestAcceptingASenderWhoseContactIsSharedReadOnlyIsRefused(t *testing.T) {
	s := stageSenderCard(t, func(e *integration.Env) ids.UUID { return e.Rep3 })
	s.WsExec(t, `INSERT INTO record_grant (record_type, record_id, subject_type, subject_id, access, granted_by)
		VALUES ('contact', $1, 'user', $2, 'read', $3)`, s.contact, s.Rep1, s.Rep3)

	if _, err := s.svc.Decide(s.decider(), s.approval, true, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("accepting a sender on a read-only share: err = %v, want permission denied", err)
	}
	if n := s.filedOntoContact(t); n != 0 {
		t.Errorf("a refused accept filed the mail onto the shared contact (%d link)", n)
	}
}

// The card is about one sender. An edit that names another address is
// refused before the decision, so the card still waits.
func TestAnEditThatRetargetsTheSenderIsRefused(t *testing.T) {
	s := stageSenderCard(t, func(e *integration.Env) ids.UUID { return e.Rep1 })
	edited := []byte(`{"email":"someone@else.example","domain":"else.example","display_name":"Someone"}`)

	var invalid *approvals.InvalidEditError
	if _, err := s.svc.DecideEdited(s.decider(), s.approval, edited); !errors.As(err, &invalid) {
		t.Fatalf("an edit naming another sender: err = %v, want an invalid edit", err)
	}
	if status := s.WsScalar(t, `SELECT status FROM approval WHERE id = $1`, s.approval.UUID); status != "pending" {
		t.Errorf("the refused card is %q, want it still pending", status)
	}
}
