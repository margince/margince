// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gmail

// Reading a message's headers without its body.
//
// About half of a typical mailbox is mail between colleagues, which capture
// drops without storing. Deciding that needs only the headers, and Gmail serves
// them in a small metadata read, so a backfill asks for them first and
// downloads the full message only when it may be kept.

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/retryafter"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// HeaderFetcher is the OPTIONAL API seam for a headers-only read. An API
// without it is read in full, exactly as before.
type HeaderFetcher interface {
	// GetHeaders fetches one message's headers (format=metadata) and answers
	// them as a header-only RFC822 block in Message.RFC822, with the same
	// labels and SENT filing GetRaw reads. ErrMessageGone on a 404.
	GetHeaders(ctx context.Context, accessToken, msgID string) (Message, error)
}

var _ HeaderFetcher = (*httpAPI)(nil)

// GetHeaders reads one message's headers and labels.
func (a *httpAPI) GetHeaders(ctx context.Context, accessToken, msgID string) (Message, error) {
	var out struct {
		LabelIDs []string `json:"labelIds"` //nolint:tagliatelle // Google's wire format (camelCase); must match to decode
		Payload  struct {
			Headers []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			} `json:"headers"`
		} `json:"payload"`
	}
	q := url.Values{"format": {"metadata"}}
	status, err := a.get(ctx, accessToken, "/messages/"+url.PathEscape(msgID), q, &out, maxJSONResponseBytes)
	if status == http.StatusNotFound {
		return Message{}, ErrMessageGone
	}
	if err != nil {
		return Message{}, err
	}
	var b strings.Builder
	for _, h := range out.Payload.Headers {
		b.WriteString(headerLine(h.Name, h.Value))
	}
	b.WriteString("\r\n")
	return Message{RFC822: []byte(b.String()), FiledAsSent: hasSentLabel(out.LabelIDs), Labels: out.LabelIDs}, nil
}

// headerLine renders one header as an RFC822 line. Gmail answers each header
// unfolded; a stray line break inside a value is flattened so one header can
// never become two.
func headerLine(name, value string) string {
	name = strings.TrimSpace(name)
	if name == "" || strings.ContainsAny(name, ":\r\n") {
		return ""
	}
	value = strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ").Replace(value)
	return name + ": " + value + "\r\n"
}

// ErrPageTokenRejected marks a backfill page token Gmail no longer accepts. It
// wraps connector.ErrCursorGone, the class the engine answers by starting the
// window's walk again from the top; captured messages dedupe, so nothing is
// stored twice.
var ErrPageTokenRejected = fmt.Errorf("gmail: the backfill page token was rejected: %w", connector.ErrCursorGone)

// pageTokenRejected reports whether a messages.list failure means the page
// token itself was refused. Gmail answers a stale or garbled token with a 400.
func pageTokenRejected(status int, pageToken string, err error) bool {
	return err != nil && pageToken != "" && status == http.StatusBadRequest
}

// rateLimitWait is how long Google asked us to wait: the Retry-After header
// when it sent one, else the time its error message names. Zero when it said
// neither, and the caller's own ladder decides.
func rateLimitWait(resp *http.Response, body []byte) time.Duration {
	if wait := retryafter.Of(resp); wait > 0 {
		return wait
	}
	return retryAfterInBody(body, time.Now())
}

// retryAfterInBody reads the wait out of a Google rate-limit error message
// ("User-rate limit exceeded. Retry after 2026-10-01T05:00:00.000Z"), which
// Gmail sends instead of a Retry-After header. Zero when the body names no
// time or the time has passed.
func retryAfterInBody(body []byte, now time.Time) time.Duration {
	m := retryAfterPattern.FindSubmatch(body)
	if m == nil {
		return 0
	}
	at, err := time.Parse(time.RFC3339Nano, string(m[1]))
	if err != nil {
		return 0
	}
	if wait := at.Sub(now); wait > 0 {
		return wait
	}
	return 0
}

var retryAfterPattern = regexp.MustCompile(`(?i)retry after (\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z)`)
