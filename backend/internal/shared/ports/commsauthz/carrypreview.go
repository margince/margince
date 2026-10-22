// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package commsauthz

import (
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// CarriedStop preserves communication restrictions when their subject changes.
type CarriedStop struct {
	ID           ids.UUID
	Kind         string
	Authority    string
	PurposeID    *ids.UUID
	PurposeLabel string
}

// CarriedConsentLinks keeps consent evidence attached to the surviving subject.
type CarriedConsentLinks struct {
	WithdrawalCredentials []CarriedWithdrawalLink
	CommunicationBases    []CarriedCommunicationBasis
	Confirmations         []CarriedConfirmation
	DoubleOptIns          []CarriedDoubleOptIn
	QualifyingEvents      []CarriedQualifyingEvent
}

// CarriedWithdrawalLink exposes withdrawal-link status without its redeemable credential.
type CarriedWithdrawalLink struct {
	ID           ids.UUID
	Address      string
	Scope        string
	PurposeLabel *string
	IssuedAt     time.Time
	ExpiresAt    time.Time
	RevokedAt    *time.Time
}

// CarriedCommunicationBasis preserves the evidence and validity period behind permission to communicate.
type CarriedCommunicationBasis struct {
	ID               ids.UUID
	Kind             string
	SourceActivityID *ids.UUID
	ValidFrom        time.Time
	ValidUntil       *time.Time
	RevokedAt        *time.Time
}

// CarriedOverride preserves the authority behind an exception when its subject changes.
type CarriedOverride struct {
	ID        ids.UUID
	Category  string
	Reason    string
	Authority string
}

// CarriedPreferenceLink makes link retirement visible before addresses move.
type CarriedPreferenceLink struct {
	ID            ids.UUID
	EmailID       *ids.UUID
	CreatedAt     time.Time
	ExpiresAt     time.Time
	RevokedAt     *time.Time
	RevokedReason *string
	WillRetire    bool
}

// CarriedConfirmation exposes confirmation status without its redeemable credential.
type CarriedConfirmation struct {
	ID         ids.UUID
	Address    string
	IssuedAt   time.Time
	ExpiresAt  time.Time
	OpenedAt   *time.Time
	ConsumedAt *time.Time
}

// CarriedDoubleOptIn keeps purpose and consumption status available for merge review.
type CarriedDoubleOptIn struct {
	ID           ids.UUID
	PurposeID    ids.UUID
	PurposeLabel string
	IssuedAt     time.Time
	ExpiresAt    time.Time
	ConsumedAt   *time.Time
}

// CarriedQualifyingEvent shows whether consent evidence survives or collapses into an existing event.
type CarriedQualifyingEvent struct {
	ID         ids.UUID
	Kind       string
	SourceType *string
	SourceID   *ids.UUID
	Note       *string
	OccurredAt time.Time
	Collapsed  bool
}
