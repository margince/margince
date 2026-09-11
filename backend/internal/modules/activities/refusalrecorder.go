// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// What an installation keeps when a send is refused.
//
// The refusal itself is answered by the caller either way. This is the optional
// second half: a composition that keeps records gets the chance to keep this
// one, at the one moment it can be kept.

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// RefusalRecorder turns a refusal the staging call carried out of its
// transaction into whatever durable record the installation keeps of it.
//
// A SECOND, OPTIONAL SEAM rather than a wider StageTx, because it runs at a
// different MOMENT and that is the whole point: staging runs inside the
// caller's transaction, and this runs after that transaction has unwound and
// returned its pool connection. A recorder called from inside would either be
// rolled back with the refusal it records, or hold a second connection while
// the first is still checked out — and enough concurrent refusals would then
// wait on a connection none of them can release.
//
// A stager that does not implement it is a composition that keeps no record,
// which is what every fixture is and what the product was before this existed.
//
// intentID names the held scheduled_send carrying the refused message, so the
// review binds to something a human can resume. A zero id means the message was
// not held — the caller could not freeze it, or the transport has nothing to
// freeze — and the review is recorded without one, which is what every review
// looked like before holdforreview.go existed.
type RefusalRecorder interface {
	RecordPendingReview(ctx context.Context, err error, intentID ids.UUID) error
}

// recordRefusal gives a stager that keeps records the chance to keep this one,
// once the transaction is done with. It answers the error either way, so a
// composition with no recorder refuses exactly as it did before.
func recordRefusal(ctx context.Context, stager DeliveryStager, err error, intentID ids.UUID) error {
	recorder, ok := stager.(RefusalRecorder)
	if !ok {
		return err
	}
	return recordRefusalOn(ctx, recorder, err, intentID)
}

// recordChannelRefusal is the same offer to the channel stager, which is a
// different interface carrying the same optional seam.
//
// It never holds an intent. scheduled_send freezes a MAIL message — its payload
// is addressed, subjected and attached like one, and the fire path rebuilds a
// mail send from it. A channel reply frozen into that shape would be a message
// no fire could send, so a refused channel reply keeps the review it always had
// and gains nothing to resume. Giving channels their own resumable intent is a
// slice of its own.
func recordChannelRefusal(ctx context.Context, stager ChannelDeliveryStager, err error) error {
	recorder, ok := stager.(RefusalRecorder)
	if !ok {
		return err
	}
	return recordRefusalOn(ctx, recorder, err, ids.UUID{})
}

// recordRefusalOn is the shared body both transports call once they hold a
// recorder. A refusal recorded on mail and one recorded on a channel answer
// the same rule because they run the same code.
func recordRefusalOn(ctx context.Context, recorder RefusalRecorder, err error, intentID ids.UUID) error {
	if err == nil {
		return nil
	}
	return recorder.RecordPendingReview(ctx, err, intentID)
}
