// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
)

// SendObserver hears what one Send through the two email doors came to. It gets
// the outcome when the message went or was scheduled, and the error otherwise.
// Compose counts it, because classifying a refusal needs the consent module's types.
type SendObserver func(ctx context.Context, out SendOutcome, err error)

// WithSendObserver binds the observer both email doors report to. Nil reports
// to nobody.
func (h Handlers) WithSendObserver(observe SendObserver) Handlers {
	h.sendObserver = observe
	return h
}

func (h Handlers) observeSend(ctx context.Context, out SendOutcome, err error) {
	if h.sendObserver != nil {
		h.sendObserver(ctx, out, err)
	}
}

// repeatedSendError is a refusal of a message this seat already holds from an
// earlier refused Send. The held row is the identity of one Send intent, so a
// second refusal of it is a repeat press and never a first one.
type repeatedSendError struct{ refusal error }

func (e *repeatedSendError) Error() string { return e.refusal.Error() }

func (e *repeatedSendError) Unwrap() error { return e.refusal }

// IsRepeatedSend reports whether err refused a message an earlier Send of the
// same seat already held.
func IsRepeatedSend(err error) bool {
	_, repeated := errors.AsType[*repeatedSendError](err)
	return repeated
}
