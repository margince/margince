// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package openchannel

// The outbound half's transport: one signed POST to an address a member
// registered, and the bounds it is held to on the way out.
//
// THE SIGNATURE IS THE SAME SCHEME THE INBOUND EDGE ACCEPTS, and that is the
// point of this connector rather than an economy. A receiver verifies what
// leaves here with the code that verifies what arrives there — the same three
// headers, the same material, the same algorithm prefix — so "point two of these
// installations at each other" is a thing that works. The material is built by
// the published SignedPayload and never re-spelled here: a second spelling is one
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
	"fmt"
	"net"
	"net/http"
	"strconv"
	"syscall"
	"time"

	"github.com/margince/margince/backend/pkg/extension"
)

// The refusal classes a send acts on. CONSTANTS rather than errors.New values,
// which is the tier's rule rather than a style: a unit's root package may hold no
// package-level initializer that CALLS anything, because an initializer runs at
// import — before the declaration has been validated. A string-kinded error type
// is comparable, so errors.Is answers about these exactly as about a sentinel.
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
func newSender(address string) (*sender, error) {
	// Re-checked here rather than trusted from the row. The row holds what was
	// legal when it was written, and this is the last moment before a packet
	// leaves — a stored address that predates a tightened rule must not be
	// dialable because it was stored before the rule.
	dialable, err := registrableURL(address)
	if err != nil {
		return nil, err
	}
	dialer := &net.Dialer{Timeout: sendTimeout, Control: refusePrivate}
	return &sender{
		url: dialable,
		http: &http.Client{
			Timeout:   sendTimeout,
			Transport: &http.Transport{DialContext: dialer.DialContext},
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

// refusePrivate refuses to dial anything that is not a globally routable unicast
// address, on the CONCRETE address the resolver returned.
//
// The reserved ranges are read from the published surface rather than restated
// here. A hand-copy drifts, and a range the core refuses while this unit admits
// it is a member-supplied host reaching an internal address.
func refusePrivate(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("openchannel: the address %q is not host:port", address)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("openchannel: %q did not resolve to an address", host)
	}
	if !publicIP(ip) {
		return fmt.Errorf("openchannel: refusing to post to %s — it is not a public address, and an address a member registered must not become a probe of this deployment's own network", ip)
	}
	return nil
}

// publicIP reports whether ip is a globally routable unicast address: what the
// stdlib's own predicates already answer, plus the core's published denylist for
// the ranges they miss.
func publicIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	for _, reserved := range extension.ReservedNets() {
		if reserved.Contains(ip) {
			return false
		}
	}
	return true
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
		// The request went out and this side never learned what was decided —
		// including the guard's own refusal, which transmitted nothing at all.
		// The two are told apart by the guard running BEFORE anything leaves, so
		// its message names the address it would not dial; what they share is
		// that no answer exists, and the send path treats an unanswered POST as
		// unknown rather than as a failure.
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

// signatureOver is the header value a receiver verifies, and it is built from the
// PUBLISHED material rather than from a concatenation spelled here.
//
// The same function the inbound edge's verifier uses, over the same value: what
// this connector sends is what this connector would accept.
func signatureOver(secret []byte, nonce string, at time.Time, body []byte) string {
	signed := extension.InboundRequest{Timestamp: at, Nonce: nonce, Body: body}
	mac := hmac.New(sha256.New, secret)
	mac.Write(signed.SignedPayload())
	return signaturePrefix + hex.EncodeToString(mac.Sum(nil))
}
