// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package employerchange asks a contact's owner whether the contact moved to
// the company their newest mail points at, and writes the move they accept.
package employerchange

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// Kind is the approval kind of the question.
const Kind = approvalsubject.KindEmployerChange

const (
	actor       = "agent:" + Kind
	summaryCode = "approval.employerChangeAsked"
	targetType  = "contact"
)

// staleReason tells the decider why an accept wrote nothing.
const staleReason = "This contact's employer changed since the question was asked, so nothing was written."

// NewInbox builds the approvals service a question is staged on. Compose binds
// it at start-up. That service is built from a contacts store, so it cannot be
// handed to one while the store is built.
var NewInbox func(*database.DB) *approvals.Service

// Proposer stages the question for the contact's owner.
type Proposer struct{ DB *database.DB }

// ProposeEmployerChangeTx stages one question per contact and company. A
// repeat joins the open one, and a pair the owner declined is not asked again.
// An owner who can no longer be asked gets nothing, and nobody else is asked.
func (p Proposer) ProposeEmployerChangeTx(ctx context.Context, tx pgx.Tx, change approvalsubject.EmployerChange) error {
	body, err := json.Marshal(change)
	if err != nil {
		return fmt.Errorf("employerchange: encoding the question: %w", err)
	}
	identity, err := json.Marshal(map[string]string{
		"contact_id": change.ContactID.String(), "to_company_id": change.ToCompanyID.String(),
	})
	if err != nil {
		return fmt.Errorf("employerchange: encoding the identity: %w", err)
	}
	// The hash leaves the evidence out, so the next mail joins the open
	// question instead of replacing it. A different employment to end is a
	// new question.
	digest := sha256.Sum256(append(identity, change.FromRelationshipID.String()...))
	_, _, err = NewInbox(p.DB).StageUnlessDeclinedTx(principal.SystemActing(ctx, actor), tx, approvals.StageInput{
		Kind:           Kind,
		ProposedChange: body,
		DiffHash:       hex.EncodeToString(digest[:]),
		TargetType:     targetType,
		TargetID:       change.ContactID,
		Summary: fmt.Sprintf(systemtext.Copy(summaryCode, textlang.English),
			change.ContactName, change.FromCompanyName, change.ToCompanyName),
		SystemText: &systemtext.Message{Version: 1, Subject: systemtext.Sentence(summaryCode,
			systemtext.Text(change.ContactName), systemtext.Text(change.FromCompanyName), systemtext.Text(change.ToCompanyName))},
		JoinPending: true,
		Identity:    identity,
		Evidence:    []approvals.Evidence{evidence(change)},
	})
	if errors.Is(err, approvals.ErrNoDecider) {
		return nil
	}
	return err
}

// evidence is what the decider judges the question on: the signature line, or
// the mail domain.
func evidence(change approvalsubject.EmployerChange) approvals.Evidence {
	snippet := change.Signature
	if snippet == "" {
		snippet = change.Domain
	}
	return approvals.Evidence{
		Snippet: textcut.Bytes(snippet, approvals.MaxEvidenceSnippet), SourceType: "activity", SourceID: change.ActivityID,
	}
}

// Effect writes the accepted move as the deciding owner.
func Effect(svc *approvals.Service, store *contacts.Store) approvals.ApprovedEffect {
	return func(ctx context.Context, approvalID ids.ApprovalID, proposedChange json.RawMessage, diffHash string) error {
		var change approvalsubject.EmployerChange
		if err := json.Unmarshal(proposedChange, &change); err != nil {
			return fmt.Errorf("employerchange: decoding the question: %w", err)
		}
		applied := false
		err := svc.RedeemAndApply(ctx, approvalID, Kind, diffHash, func(tx pgx.Tx) error {
			var err error
			applied, err = store.ApplyEmployerChangeTx(ctx, tx, change)
			return err
		})
		if err == nil && !applied {
			return &approvals.ObsoleteError{Reason: staleReason}
		}
		return err
	}
}

// Precheck refuses an edited answer. The question is answered yes or no: the
// day it carries becomes the old employment's end date, and nobody typed it.
func Precheck(_ context.Context, _, edited json.RawMessage) error {
	if len(edited) > 0 {
		return &approvals.InvalidEditError{Cause: errors.New("an employer change is accepted or declined as it stands")}
	}
	return nil
}
