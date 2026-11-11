// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// mailEnsure is what the in-transaction ensure left for after the commit.
type mailEnsure struct {
	outcome EnsureOutcome
	fault   error
}

// ensureCounterpartyTx files a freshly captured mail under its counterparty
// inside the capture transaction. The mail is never visible unlinked, so its
// link does not move the Home fence (home_read_fence_first_link). A fault
// costs the derivation and not the mail: the savepoint rolls it back, and the
// link_reconcile sweep links the link-less mail later.
func (s *Sink) ensureCounterpartyTx(ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord, landed activityLanding) (mailEnsure, error) {
	decision := landed.decision
	if !landed.created || landed.internal || !decision.ensuresMail() {
		return mailEnsure{}, nil
	}
	sp, err := tx.Begin(ctx)
	if err != nil {
		return mailEnsure{}, fmt.Errorf("capture: opening the counterparty-ensure savepoint: %w", err)
	}
	outcome, ensureErr := s.ensurer.EnsureCounterpartyTx(ctx, sp, s.mailEnsureRequest(rec, landed.ref, decision))
	if ensureErr != nil {
		if rbErr := sp.Rollback(ctx); rbErr != nil {
			return mailEnsure{}, errors.Join(ensureErr, rbErr)
		}
		if storekit.IsLockCycle(ensureErr) {
			return mailEnsure{}, ensureErr
		}
		return mailEnsure{fault: ensureErr}, nil
	}
	if err := sp.Commit(ctx); err != nil {
		return mailEnsure{}, fmt.Errorf("capture: committing the counterparty ensure: %w", err)
	}
	return mailEnsure{outcome: outcome}, nil
}

// ensuresMail says the mail ensure later in the capture transaction will run.
func (d counterpartyDecision) ensuresMail() bool { return d.create && !d.channel }

// lockMailReachTx locks every contact this mail is about to be filed under,
// with what they reach, in the order every writer shares
// (storekit.LockLastActivityTargets). Otherwise two mails naming one pair in
// swapped roles lock the pair in opposite orders: one through the participant
// filing, the other through the ensure. It answers the participants' records,
// which the filing reads. A suppressed mail is filed under nobody.
//
// The sender's address lock comes first, as the ensure and the verdict path
// take it before the contact. A sender with no contact yet gets one the ensure
// mints, a row no other transaction can see or lock.
func lockMailReachTx(ctx context.Context, tx pgx.Tx, activity ids.ActivityID, decision counterpartyDecision) ([]participantRecord, error) {
	var named []participantRecord
	var targets storekit.LastActivityTargets
	if decision.traceOutcome != TraceSuppressed {
		var err error
		if named, err = participantRecords(ctx, tx, activity); err != nil {
			return nil, err
		}
		for _, record := range named {
			if record.entity == datasource.EntityContact {
				targets.Contacts = append(targets.Contacts, record.id)
			}
		}
	}
	if decision.ensuresMail() && decision.subject.Email != "" {
		holders, err := storekit.LockIdentityHolders(ctx, tx, storekit.IdentityKeys{Emails: []string{normalizeEmail(decision.subject.Email)}})
		if err != nil {
			return nil, fmt.Errorf("capture: locking the sender's address: %w", err)
		}
		for _, contact := range holders.Contacts {
			targets.Contacts = append(targets.Contacts, contact.UUID)
		}
	}
	if err := storekit.LockLastActivityTargets(ctx, tx, targets); err != nil {
		return nil, fmt.Errorf("capture: %w", err)
	}
	return named, nil
}

func (s *Sink) mailEnsureRequest(rec connector.NormalizedRecord, ref datasource.EntityRef, decision counterpartyDecision) EnsureRequest {
	// The party the ladder judged (counterpartyDecision.subject).
	cp := decision.subject
	// No HoldAuthority: admitWriteTx took the member's authority before any
	// row lock, and no other transaction can see, let alone lock, this new mail.
	return EnsureRequest{
		Admission:       s.counterpartyAdmission(rec, ref, cp),
		Email:           cp.Email,
		DisplayName:     cp.DisplayName,
		Domain:          cp.Domain,
		OwnerID:         decision.owner,
		ActivityID:      ref.ID,
		Source:          captureSource(rec),
		CapturedBy:      decision.capturedBy,
		SuppressCompany: decision.suppressCompany,
		Replied:         decision.replied,
	}
}

// settleCounterparty finishes the counterparty step once the capture
// committed. A channel record still ensures here, in its own transaction.
func (s *Sink) settleCounterparty(ctx context.Context, rec connector.NormalizedRecord, ref datasource.EntityRef, decision counterpartyDecision, ensured mailEnsure) {
	if !decision.create {
		return
	}
	if decision.channel {
		s.ensureChannelCounterparty(ctx, rec, ref, decision)
		return
	}
	if ensured.fault != nil {
		s.logEnsureFault(ctx, rec, ensured.fault)
		return
	}
	ensured.outcome.afterCommit(ctx)
	// Nil unless a backfill page is running: incremental sync creates
	// counterparties too, and they belong to no run.
	pageProgressFrom(ctx).counted(ctx, ensured.outcome)
}
