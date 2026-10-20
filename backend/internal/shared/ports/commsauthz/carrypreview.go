// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package commsauthz

import (
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"time"
)

type CarriedStop struct {
	ID           ids.UUID
	Kind         string
	Authority    string
	PurposeID    *ids.UUID
	PurposeLabel string
}

type CarriedConsentLinks struct {
	WithdrawalCredentials []CarriedWithdrawalLink
	CommunicationBases    []CarriedCommunicationBasis
	Confirmations         []CarriedConfirmation
	DoubleOptIns          []CarriedDoubleOptIn
	QualifyingEvents      []CarriedQualifyingEvent
}

type CarriedWithdrawalLink struct {
	ID           ids.UUID
	Address      string
	Scope        string
	PurposeLabel *string
	IssuedAt     time.Time
	ExpiresAt    time.Time
	RevokedAt    *time.Time
}

type CarriedCommunicationBasis struct {
	ID               ids.UUID
	Kind             string
	SourceActivityID *ids.UUID
	ValidFrom        time.Time
	ValidUntil       *time.Time
	RevokedAt        *time.Time
}

type CarriedOverride struct {
	ID        ids.UUID
	Category  string
	Reason    string
	Authority string
}

type CarriedPreferenceLink struct {
	ID            ids.UUID
	EmailID       *ids.UUID
	CreatedAt     time.Time
	ExpiresAt     time.Time
	RevokedAt     *time.Time
	RevokedReason *string
	WillRetire    bool
}

type CarriedConfirmation struct {
	ID         ids.UUID
	Address    string
	IssuedAt   time.Time
	ExpiresAt  time.Time
	OpenedAt   *time.Time
	ConsumedAt *time.Time
}

type CarriedDoubleOptIn struct {
	ID           ids.UUID
	PurposeID    ids.UUID
	PurposeLabel string
	IssuedAt     time.Time
	ExpiresAt    time.Time
	ConsumedAt   *time.Time
}

type CarriedQualifyingEvent struct {
	ID         ids.UUID
	Kind       string
	SourceType *string
	SourceID   *ids.UUID
	Note       *string
	OccurredAt time.Time
	Collapsed  bool
}
