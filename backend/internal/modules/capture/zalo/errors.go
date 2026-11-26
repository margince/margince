// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package zalo

import (
	"errors"
	"fmt"
	"time"
)

// Client errors. A caller branches on these with errors.Is and never on the
// message.
var (
	// ErrReauthRequired reports an access token or refresh token Zalo no longer
	// accepts; only the OA admin granting access again repairs it.
	ErrReauthRequired = errors.New("zalo: the access grant is expired or invalid")
	// ErrRateLimited reports a throttle; the same call succeeds later.
	ErrRateLimited = errors.New("zalo: rate limited")
	// ErrOutsideReplyWindow reports a customer the OA may not message now,
	// because they have not interacted recently or do not follow the OA.
	ErrOutsideReplyWindow = errors.New("zalo: the customer is outside the reply window")
	// ErrInvalidRecipient reports a customer id Zalo does not know.
	ErrInvalidRecipient = errors.New("zalo: the recipient is not a known customer")
	// ErrRejected reports any other refusal by Zalo.
	ErrRejected = errors.New("zalo: the request was refused")
	// ErrUnreachable reports a call whose outcome is unknown: transport failure,
	// a 5xx or an unreadable answer. A send that ends here may have been
	// delivered, so it is not safe to repeat blindly.
	ErrUnreachable = errors.New("zalo: no usable answer")
	// ErrMediaTooLarge reports an attachment over the caller's size limit.
	ErrMediaTooLarge = errors.New("zalo: the attachment exceeds the size limit")
	// ErrMediaChecksum reports downloaded bytes that differ from the checksum
	// the webhook stated.
	ErrMediaChecksum = errors.New("zalo: the attachment checksum does not match")
)

// APIError carries Zalo's own error code and message beside the sentinel it
// maps to. The message is Zalo's text about the request, never request
// content.
type APIError struct {
	Code    int
	Message string
	kind    error
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%v (zalo error %d: %s)", e.kind, e.Code, e.Message)
}

// Unwrap lets errors.Is match the sentinel.
func (e *APIError) Unwrap() error { return e.kind }

// RetryAfter is how long to wait before repeating a call that ended in
// ErrRateLimited. Zalo names no interval, so it is a fixed pause.
const RetryAfter = time.Minute

// Zalo's numeric codes, grouped by what the caller does about them.
var (
	reauthCodes    = map[int]bool{-124: true, -216: true, -14019: true}
	rateLimitCodes = map[int]bool{-32: true, -36: true, -233: true}
	windowCodes    = map[int]bool{-213: true, -230: true, -232: true}
	recipientCodes = map[int]bool{-204: true, -201: true}
)

// kindOf maps a non-zero Zalo code to its sentinel.
func kindOf(code int) error {
	switch {
	case reauthCodes[code]:
		return ErrReauthRequired
	case rateLimitCodes[code]:
		return ErrRateLimited
	case windowCodes[code]:
		return ErrOutsideReplyWindow
	case recipientCodes[code]:
		return ErrInvalidRecipient
	default:
		return ErrRejected
	}
}
