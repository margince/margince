// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package openchannel

// The outbound half's transport: one signed POST to an address a member
// registered, and the bounds it is held to on the way out.
//
// THE SIGNATURE USES THE SAME HEADERS, MATERIAL AND ALGORITHM THE INBOUND EDGE
// VERIFIES, BUT A DIFFERENT SCOPE — extension.ScopeOutbound here, against
// extension.ScopeInbound at the edge that admits arrivals. That difference is
// deliberate and load-bearing, not an inconsistency to close: a message this
// connector SENDS must not verify as a valid ARRIVAL at its own edge, because
// this connector signs with the same member secret it uses to admit requests.
// Same-scope material would let any party we transmit to relay our own bytes
// back and be authenticated as the sender — the cheapest forgery available,
// requiring no secret at all. So two installations of this connector do NOT
// simply verify one another; each one only verifies what a member's own system
// (holding no secret of ours) sends it. The material itself is built by the
// published SignedPayload and never re-spelled here: a second spelling is one
// that will one day differ from the sender's, and the failure looks like a wrong
// secret rather than a rule that moved.
//
// THE ADDRESS IS MEMBER-SUPPLIED, which makes it an SSRF vector and this file the
// place that guard belongs. It is applied on the CONCRETE address the resolver
// returned rather than on the name, because a DNS answer pointing at this
// deployment's own network is exactly what checking the name cannot prevent.

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/margince/margince/backend/pkg/extension"
)

// The refusal classes a send acts on. CONSTANTS rather than errors.New values,
// which is the tier's rule rather than a style: a unit's root package may hold no
// package-level initializer that CALLS anything, because an initializer runs at
// import — before the declaration has been validated. A string-kinded error type
// is comparable, so errors.Is answers about these exactly as about a sentinel.
//
// A blocked dial is not a third member of this vocabulary: the egress guard is
// published, not written here, and it publishes its own sentinel —
// extension.ErrEgressRefused — for exactly the same reason this unit publishes
// none of ITS classes to a caller: the distinction that matters downstream
// belongs to whoever produces it.
const (
	// errRefused is an answer the receiver actually sent, saying no. It is a
	// DEFINITE answer, so nothing was delivered and the product's ladder may
	// retry it.
	errRefused sendError = "openchannel: the registered address refused this message"

	// errUnanswered marks a request that WENT OUT and whose outcome never came
	// back: the connection failed mid-flight, the deadline expired, the answer
	// could not be read. The message may be at the recipient and may not, and
	// this connector has no prior-send lookup that could ever find out — so it is
	// the one refusal that must not be reported as a failure, because the
	// product's ladder would send the rep's message a second time with nothing
	// able to detect it.
	errUnanswered sendError = "openchannel: the registered address never reported the outcome"
)

// sendError is one of this unit's own outbound refusal classes.
type sendError string

func (e sendError) Error() string { return string(e) }

// deliveryNonce derives the signature nonce from the product's own delivery id.
//
// IT CANNOT BE THE DELIVERY ID ITSELF. That id is a UUID and carries hyphens,
// while InboundHeaderNonce documents the value as hex-encoded and the core's
// own edge enforces it (extension.ValidInboundNonce): a connector receiving its
// own scheme back would refuse the very nonce it sent. Deriving it rather than
// generating it fresh per attempt is the other half of the requirement — a
// retried attempt must present the SAME nonce, because that is what lets a
// receiver recognise a re-post as the message it already saw rather than land
// it as a second one. A fixed-size hex digest is also comfortably inside
// MaxInboundNonce regardless of what the product's id format ever becomes.
func deliveryNonce(idempotencyKey string) string {
	sum := sha256.Sum256([]byte(idempotencyKey))
	return hex.EncodeToString(sum[:])
}

// sendTimeout bounds ONE outbound POST, and the unit brings it because the core
// imposes none: the only ceiling above it is the delivery job's five-minute wall
// clock, which is a budget for the whole delivery rather than for one request.
//
// Twenty seconds, matching the tree's other connector so two numbers a reader
// compares are comparable. It is chosen from what the far end IS: an https
// endpoint a member registered, which accepts a small document and acknowledges
// it. Twenty seconds covers a slow handshake, a cold serverless start and a
// congested link with room over; past that the far end is not answering, and
// waiting longer only holds a worker while the delivery's own wall clock runs
// down. The cost of being wrong is bounded in the safe direction — a timeout is
// the unknown-outcome class, which STOPS the delivery rather than re-sending it.
const sendTimeout = 20 * time.Second

// sender posts to one member's registered address.
type sender struct {
	url  string
	http *http.Client
}

// senderFactory is how a send reaches a member's registered address.
//
// It is a parameter rather than a call to newSender inside the send path because
// that address is this unit's ONE true boundary: the production constructor wraps
// the transport in the egress guard, which refuses a loopback address by design
// and therefore refuses a test's own listener. Everything above it — resolving
// the endpoint, signing, classifying the answer, recording the attempt — is this
// unit's own logic and is driven end to end through this seam.
type senderFactory func(url string) (*sender, error)

// newSender builds the client a send uses, with the egress guard attached.
//
// THE GUARD IS THE POINT OF THIS CONSTRUCTOR. The address is text a member typed:
// without a control hook, a host that resolves to a link-local address makes this
// installation's own worker post a signed request to its cloud metadata endpoint.
// The address grammar was already checked where the member could read the refusal
// (url.go); what is left is the part only the resolver knows.
//
// The guard itself is extension.OutboundTransport's, not this unit's own: the
// dialer, its Control hook and the published denylist behind it live once, in
// the core, so a range added there reaches this connector without this file
// changing. What this unit still owns is what OutboundTransport deliberately
// leaves to the caller — the client-wide Timeout and the redirect policy below.
func newSender(address string) (*sender, error) {
	// Re-checked here rather than trusted from the row. The row holds what was
	// legal when it was written, and this is the last moment before a packet
	// leaves — a stored address that predates a tightened rule must not be
	// dialable because it was stored before the rule.
	dialable, err := registrableURL(address)
	if err != nil {
		return nil, err
	}
	return &sender{
		url: dialable,
		http: &http.Client{
			Timeout:   sendTimeout,
			Transport: extension.OutboundTransport(),
			// A redirect is another address, chosen by the receiver rather than
			// by the member — and following one would carry a signed body to a
			// host the guard never judged. Refusing is cheap: a receiver that
			// answers a redirect here is one this connector does not understand.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

// post transmits one signed document and classifies whatever comes back.
//
// nonce is the receiver's replay key and at is what it checks for freshness; both
// are covered by the signature, so neither can be edited in flight.
func (s *sender) post(ctx context.Context, secret []byte, nonce string, at time.Time, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%w: building the request: %s", errRefused, err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(extension.InboundHeaderTimestamp, strconv.FormatInt(at.Unix(), 10))
	req.Header.Set(extension.InboundHeaderNonce, nonce)
	req.Header.Set(extension.InboundHeaderSignature, signatureOver(secret, nonce, at, body))
	resp, err := s.http.Do(req)
	if err != nil {
		// The guard's refusal FIRST, and it is not a variety of "no answer".
		// Control runs on the resolved address before a connection exists, so
		// nothing was transmitted and the outcome is certain — where a genuine
		// unanswered POST may be at the recipient and may not. Reporting the
		// two alike would park a delivery that certainly never left, under a
		// sentence blaming a system this installation declined to call. The
		// sentinel comes from the guard itself, not from a class this unit
		// invented for it: extension.ErrEgressRefused survives the http.Client
		// and net.OpError wrapping between the dial and here.
		if errors.Is(err, extension.ErrEgressRefused) {
			return fmt.Errorf("%w: %s", extension.ErrEgressRefused, err.Error())
		}
		// The request went out and this side never learned what was decided.
		// No answer exists, so the send path treats it as unknown rather than
		// as a failure.
		return fmt.Errorf("%w: %s", errUnanswered, err.Error())
	}
	// The answer's BODY is never read. What the far end says about itself is a
	// remote party's prose, and this connector stores and renders none of it; the
	// status is the whole of what it acts on.
	//
	//craft:ignore swallowed-errors best-effort close on an unread body: a close error says nothing about whether the receiver accepted the message, which is this call's only result
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("%w: it answered %d", errRefused, resp.StatusCode)
	}
	return nil
}

// signatureOver is the header value a receiver verifies, and it is built from
// the PUBLISHED material rather than from a concatenation spelled here.
//
// Under ScopeOutbound, which is the whole point: this connector signs with the
// same member secret it verifies arrivals with, so a payload spelled the same
// in both directions would make every message it SENDS a valid message to
// itself. The party we send to is trusted to receive, not to speak as the
// sender — and relaying our own bytes back at our own edge is the cheapest
// forgery there is. Different scope, different message.
func signatureOver(secret []byte, nonce string, at time.Time, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(extension.SigningPayload(extension.ScopeOutbound, "", "", at, nonce, body))
	return signaturePrefix + hex.EncodeToString(mac.Sum(nil))
}
