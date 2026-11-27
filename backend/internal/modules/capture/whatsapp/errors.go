// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package whatsapp

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// The Graph API outcomes a caller must tell apart, because each has a different
// answer: re-authorize, wait, stop sending to this customer, or retry later.

// ErrAuth marks a token Meta refused (expired, revoked or invalid). The
// connection needs re-authorization; retrying does not help.
var ErrAuth = errors.New("whatsapp: the access token was rejected")

// ErrOutsideWindow marks a free-form message to a customer who has not written
// in the last 24 hours (Graph code 131047). Only a template message can reach
// them; retrying the same text never works.
var ErrOutsideWindow = errors.New("whatsapp: outside the 24-hour customer-service window")

// ErrRecipientInvalid marks a recipient WhatsApp will not deliver to: not a
// WhatsApp number, malformed, or not allowed. Retrying never works.
var ErrRecipientInvalid = errors.New("whatsapp: the recipient cannot receive this message")

// ErrRejected marks any other refusal Meta stated on its own terms.
var ErrRejected = errors.New("whatsapp: the request was rejected")

// ErrUnreachable marks a failure with no provider verdict (transport fault,
// 5xx, unreadable answer). The request may be retried.
var ErrUnreachable = errors.New("whatsapp: the Graph API was unreachable")

// ErrMediaTooLarge marks media over the caller's size limit.
var ErrMediaTooLarge = errors.New("whatsapp: media exceeds the size limit")

// ErrMediaChecksum marks downloaded bytes whose SHA-256 differs from the one
// Meta stated.
var ErrMediaChecksum = errors.New("whatsapp: media checksum mismatch")

// GraphError is what Meta's error envelope said. It rides alongside the
// sentinel so a log shows the code; the message text never reaches a client.
type GraphError struct {
	Status  int
	Code    int
	Subcode int
	Message string
}

func (e *GraphError) Error() string {
	return fmt.Sprintf("graph status %d code %d subcode %d", e.Status, e.Code, e.Subcode)
}

// Graph error codes, from Meta's Cloud API error reference.
const (
	codeOutsideWindow = 131047
	maxRetryAfter     = 5 * time.Minute
)

var (
	authCodes = map[int]bool{
		190: true, 102: true, 463: true, 467: true, 10: true, 200: true,
	}
	rateLimitCodes = map[int]bool{
		4: true, 17: true, 613: true, 80007: true, 130429: true, 131056: true, 133016: true,
	}
	recipientCodes = map[int]bool{
		131026: true, 131021: true, 131030: true, 131009: true,
	}
)

// classify maps one non-2xx Graph answer to its sentinel, joined with the
// GraphError (and, for a throttle, the shared RateLimitedError carrying the
// wait).
func classify(status int, g GraphError, retryAfter time.Duration) error {
	g.Status = status
	var kind error
	switch {
	case status == http.StatusUnauthorized, authCodes[g.Code]:
		kind = ErrAuth
	case g.Code == codeOutsideWindow:
		kind = ErrOutsideWindow
	case status == http.StatusTooManyRequests, rateLimitCodes[g.Code]:
		return errors.Join(
			fmt.Errorf("whatsapp: %w", ErrRejected), &g,
			&connector.RateLimitedError{RetryAfter: min(retryAfter, maxRetryAfter), Status: status},
		)
	case recipientCodes[g.Code]:
		kind = ErrRecipientInvalid
	case status >= http.StatusInternalServerError:
		kind = ErrUnreachable
	default:
		kind = ErrRejected
	}
	return errors.Join(kind, &g)
}

func isRateLimited(err error) bool { return errors.Is(err, connector.ErrRateLimited) }
