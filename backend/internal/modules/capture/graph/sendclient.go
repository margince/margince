// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graph

// The two Graph calls the outbound half needs: submitting a MIME message, and
// finding the sent copy of one by the identity it was asked to carry. Beside
// send.go rather than in client.go because that file is the READ surface, and
// a reader auditing what this connector can do to a mailbox should not have to
// separate the two by eye.

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// sendMIMEPath is Microsoft's submit endpoint. It takes the whole RFC822
// message base64-encoded, under a `text/plain` content type — the vendor's own
// convention for "the body IS the MIME", not a mislabelling on our part.
const sendMIMEPath = "/me/sendMail"

// maxSubmitBytes is Microsoft's 4 MB write-request ceiling, read as the size of
// the BASE64 body actually sent — which is what the limit applies to and what
// a 413 is measured against.
//
// Stated in decimal MB rather than MiB deliberately: Microsoft documents "4 MB"
// without an exact byte value, so the smaller reading is the one that cannot be
// wrong.
const maxSubmitBytes = 4_000_000

// SendMIME transmits one complete RFC822 message as the signed-in user.
//
// Microsoft answers 202 Accepted with NO BODY: it names no message id at
// submission, which is why the caller resolves the sent copy afterwards by the
// identity the message carries rather than by an id this call could return.
func (a *httpAPI) SendMIME(ctx context.Context, accessToken string, rfc822 []byte) error {
	encoded := base64.StdEncoding.EncodeToString(rfc822)
	if len(encoded) > maxSubmitBytes {
		// Refused HERE and not only by Carriage, because Carriage bounds ONE
		// FILE and this bounds the whole request: several attachments each
		// inside the per-file cap still add up past Microsoft's limit, and
		// that combination is invisible to every gate above this one.
		//
		// ErrFilesNotCarried is the sentinel the dispatcher PARKS on. A message
		// this size will be exactly this size on every retry, so the retry
		// ladder can only exhaust itself and park under "the ladder ran out",
		// which names no cause. The byte count and the bound go to the operator
		// in the log the dispatcher writes; the sender gets a reason they can
		// act on.
		return fmt.Errorf("graph: the message is %d encoded bytes against Microsoft's %d-byte sendMail limit: %w",
			len(encoded), maxSubmitBytes, connector.ErrFilesNotCarried)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.base+sendMIMEPath, strings.NewReader(encoded))
	if err != nil {
		return fmt.Errorf("graph: building send request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	// text/plain is what Microsoft's MIME submit path expects; application/json
	// would make it read the base64 as a message resource and refuse it.
	req.Header.Set("Content-Type", "text/plain")
	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("graph: %s: %w", sendMIMEPath, ErrUnreachable)
	}
	//craft:ignore swallowed-errors best-effort close of the response body — the status is what matters
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyLen))
	return classifyStatus(resp, sendMIMEPath, body)
}

// maxErrorBodyLen bounds how much of a refusal Microsoft's OData envelope is
// read for its machine code. A successful send answers with nothing at all, so
// this budget only ever pays for a failure.
const maxErrorBodyLen = 64 << 10

// FindSentByMessageID resolves the sent copy of a message by the RFC822
// identity it was asked to carry, so an at-least-once retry can recognise a
// transmission that already happened.
//
// It filters Sent Items on `internetMessageId`, which is a real Graph property
// rather than a full-text index — so a match is the message's own field, not a
// search that may not have caught up. The identity is BRACKETED on the wire
// here even though this system stores it unbracketed, because that is the form
// the property holds.
//
// An identity Exchange rewrote on submission matches nothing, and that reads as
// "not sent" — the caller treats a miss as a miss rather than as a fault, and
// send.go states what that costs.
func (a *httpAPI) FindSentByMessageID(ctx context.Context, accessToken, unbracketedMessageID string) (string, bool, error) {
	if unbracketedMessageID == "" {
		return "", false, nil
	}
	q := url.Values{
		// The identity is a caller-controlled string reaching an OData filter,
		// so it is escaped for OData's own quoting rule (a literal quote is
		// doubled) before url.Values escapes it for the query. Without the
		// first step an identity carrying a quote would end the literal and the
		// rest of it would be read as filter syntax.
		paramFilter: {"internetMessageId eq '" + odataQuote("<"+unbracketedMessageID+">") + "'"},
		paramSelect: {"id"},
		paramTop:    {"1"},
	}
	var out struct {
		Value []struct {
			ID string `json:"id"`
		} `json:"value"`
	}
	if _, err := a.get(ctx, accessToken, a.base+"/me/mailFolders/sentitems/messages?"+q.Encode(), nil, &out); err != nil {
		return "", false, err
	}
	if len(out.Value) == 0 || out.Value[0].ID == "" {
		return "", false, nil
	}
	return out.Value[0].ID, true, nil
}

// odataQuote escapes a value for an OData single-quoted string literal, where
// a literal quote is written twice. It is the ONE place this escaping happens,
// so a second filter cannot be written without it.
func odataQuote(v string) string { return strings.ReplaceAll(v, "'", "''") }
