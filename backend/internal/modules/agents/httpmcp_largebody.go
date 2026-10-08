// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The bound on one MCP request, and the few slots a request past the ordinary
// body bound may hold while it is read and decoded.

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// MaxMCPRequestBytes bounds one MCP request. An agent attaches a file inline as
// base64, so the largest file it can attach is maxInlineFileBytes.
const MaxMCPRequestBytes = 8 << 20

// maxLargeMCPBodiesInFlight bounds the requests over httperr.MaxBodyBytes one
// process holds at once: each is resident several times over while it is decoded.
const maxLargeMCPBodiesInFlight = 4

// largeBodyReadDeadline bounds reading a large body once it holds a slot. 8 MiB in
// 10 s is 7 Mbit/s; the server's 30 s ReadTimeout would let a stalled sender keep it.
const largeBodyReadDeadline = 10 * time.Second

// errLargeBodiesBusy refuses a large request while every large slot is taken.
var errLargeBodiesBusy = errors.New("every slot for a large MCP request is taken")

// errLargeBodyHeld refuses a principal's second large request while its first
// runs, so one passport cannot take every slot from every other workspace.
var errLargeBodyHeld = errors.New("this principal already holds a large MCP request")

// errLargeBodyUnbounded means the read deadline could not be set: the handler
// chain lost Unwrap(), and a slot without a deadline is one a stalled sender keeps.
var errLargeBodyUnbounded = errors.New("the read deadline for a large MCP request cannot be set")

// readBody reads the request, holding a large slot for one over the ordinary
// body bound; release gives the slot back after the call is answered. A
// declared length takes its slot before any byte is read, so a refusal reaches
// a client that is still sending; an undeclared one takes it once it grows past.
func (h *httpMCPHandler) readBody(w http.ResponseWriter, r *http.Request) (body []byte, release func(), err error) {
	if r.ContentLength > MaxMCPRequestBytes {
		return nil, nil, &http.MaxBytesError{Limit: MaxMCPRequestBytes}
	}
	capped := http.MaxBytesReader(w, r.Body, MaxMCPRequestBytes)
	if r.ContentLength <= httperr.MaxBodyBytes {
		body, err = io.ReadAll(io.LimitReader(capped, httperr.MaxBodyBytes+1))
		if err != nil || len(body) <= httperr.MaxBodyBytes {
			return body, func() {}, err
		}
	}
	release, err = h.takeLargeSlot(w, r)
	if err != nil {
		return nil, nil, err
	}
	buf := bytes.NewBuffer(body)
	if _, err := buf.ReadFrom(capped); err != nil {
		release()
		return nil, nil, err
	}
	return buf.Bytes(), release, nil
}

// takeLargeSlot holds a slot for the request's principal and bounds the rest
// of the body's read by the handler's read budget.
func (h *httpMCPHandler) takeLargeSlot(w http.ResponseWriter, r *http.Request) (release func(), err error) {
	actor, _ := principal.Actor(r.Context())
	h.largeMu.Lock()
	defer h.largeMu.Unlock()
	if _, held := h.largeHolders[actor.ID]; held {
		return nil, errLargeBodyHeld
	}
	select {
	case h.largeBodies <- struct{}{}:
	default:
		return nil, errLargeBodiesBusy
	}
	h.largeHolders[actor.ID] = struct{}{}
	release = func() {
		h.largeMu.Lock()
		delete(h.largeHolders, actor.ID)
		h.largeMu.Unlock()
		<-h.largeBodies
	}
	if err := http.NewResponseController(w).SetReadDeadline(time.Now().Add(h.largeBodyReadBudget)); err != nil {
		release()
		return nil, fmt.Errorf("%w: %w", errLargeBodyUnbounded, err)
	}
	return release, nil
}

// writeBodyRefusal answers a request whose body readBody could not hand over.
func (h *httpMCPHandler) writeBodyRefusal(w http.ResponseWriter, r *http.Request, err error) {
	// The limit is read off the error because the chassis may have bound the
	// body tighter than this handler does, and the refusal names the one that fired.
	var tooLarge *http.MaxBytesError
	switch {
	case errors.Is(err, errLargeBodyHeld):
		w.Header().Set("Connection", "close")
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusTooManyRequests,
			Code:   "rate_limited",
			Detail: "This agent already has a large MCP request running. Send the next one when it is answered.",
		})
	case errors.Is(err, errLargeBodiesBusy):
		// Connection: close stops the server draining the unread body before it
		// answers, so a sender that paused still hears this.
		w.Header().Set("Connection", "close")
		w.Header().Set("Retry-After", "1")
		httperr.ServiceUnavailable(w, r, "This server is already handling as many large MCP requests "+
			"as it can hold. Wait a moment and send the call again.")
	case errors.Is(err, errLargeBodyUnbounded):
		h.server.log.Error("mcp: a large request could not be bounded", "err", err)
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusInternalServerError,
			Code:   "deadline_not_extendable",
			Detail: "This server chain cannot bound how long a large request takes to arrive.",
		})
	case errors.As(err, &tooLarge):
		w.Header().Set("Connection", "close")
		httperr.Write(w, r, httperr.BodyTooLargeRefusal(fmt.Sprintf(
			"This request exceeds the %s limit for one MCP call. Upload a larger file in the Margince app.",
			httperr.Megabytes(tooLarge.Limit))))
	case errors.Is(err, os.ErrDeadlineExceeded):
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusRequestTimeout,
			Code:   "request_timeout",
			Detail: fmt.Sprintf("This request's body did not arrive within %s. Send the call again.",
				h.largeBodyReadBudget),
		})
	default:
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusBadRequest,
			Code:   "unreadable_body",
			Detail: "This request's body could not be read to the end.",
		})
	}
}
