// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"time"

	"github.com/margince/margince/backend/internal/compose/profileverdicts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/integrations"
	"github.com/margince/margince/backend/internal/modules/introductions"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/platform/database"
)

// contactMergeDoor is a contacts door that merges: the HTTP handlers, the
// agent provider and the store the case seam calls.
type contactMergeDoor[T any] interface {
	WithStopCarrier(carrier contacts.StopCarrier) T
	WithSatelliteCarriers(consent contacts.ConsentSatelliteCarrier, intros contacts.IntroCarrier) T
	WithGrantCarrier(carrier contacts.GrantCarrier) T
	WithParticipantRosterReader(read contacts.ParticipantRosterReader) T
	WithProviderRunReader(read contacts.ProviderRunReader) T
	WithMergeSignalReader(read contacts.MergeSignalReader) T
	WithMergeCommitmentReader(read contacts.MergeCommitmentReader) T
	WithMergeNoticeReader(read contacts.MergeNoticeReader) T
	WithMergeContentReader(read contacts.MergeContentReader) T
	WithProfileVerdictReader(read contacts.ProfileVerdictReader) T
	WithFeedbackCarryReader(reader contacts.FeedbackCarryReader) T
}

// withContactMergeCarriers wires every edge a contact merge carries or
// previews over. Each merging door takes them here, so a door cannot merge
// with a carrier the others have; an unwired carrier refuses the merge.
func withContactMergeCarriers[T contactMergeDoor[T]](db *database.DB, door T) T {
	pool := db.Pool()
	return door.
		WithStopCarrier(consent.NewStore(db)).
		WithSatelliteCarriers(consent.NewStore(db), introductions.NewStore(db, time.Now)).
		WithGrantCarrier(identity.NewServiceFor(db)).
		WithParticipantRosterReader(activities.EnsureParticipantRosterVisibleTx).
		WithProviderRunReader(integrations.ReadRunForContactTx).
		WithMergeSignalReader(signals.PreviewSignalCarry(signalSummaryWords)).
		WithMergeCommitmentReader(weeklyPlanStore(pool).PreviewCommitmentCarryTx).
		WithMergeNoticeReader(notices.PreviewNoticeCarryTx).
		WithMergeContentReader(activities.NewStore(db)).
		WithProfileVerdictReader(profileverdicts.MergeReader(db)).
		WithFeedbackCarryReader(ai.NewFeedbackStore(db))
}
