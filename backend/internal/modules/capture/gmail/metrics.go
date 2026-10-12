// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gmail

// Every Gmail REST call is counted and timed here, around the three methods
// that reach the network, and so is every message a backfill walks, so a slow
// import can be read as Google's latency or ours.

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture/capturemetrics"
	"github.com/margince/margince/backend/internal/modules/capture/googleconn"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/modules/capture/oauthflow"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// get performs an authorized GET through getOnce and records the call.
//
//craft:ignore naked-any out is the caller-supplied JSON decode target — its concrete type varies per endpoint
func (a *httpAPI) get(ctx context.Context, accessToken, path string, q url.Values, out any, maxBytes int64) (int, error) {
	start := time.Now()
	status, err := a.getOnce(ctx, accessToken, path, q, out, maxBytes)
	capturemetrics.ObserveRequest(connectorName, getOp(path, q), status, err, time.Since(start))
	return status, err
}

// postJSON performs an authorized POST through postJSONOnce and records the
// call.
//
//craft:ignore naked-any payload/out are the caller-supplied JSON encode/decode values — the concrete type varies per endpoint
func (a *httpAPI) postJSON(ctx context.Context, accessToken, path string, payload, out any) error {
	start := time.Now()
	err := a.postJSONOnce(ctx, accessToken, path, payload, out)
	capturemetrics.ObserveRequest(connectorName, capturemetrics.OpOther, 0, err, time.Since(start))
	return err
}

// Watch registers the mailbox's users.watch through watchOnce and records the
// call.
func (a *httpAPI) Watch(ctx context.Context, accessToken, topic string) (string, time.Time, error) {
	start := time.Now()
	historyID, expires, err := a.watchOnce(ctx, accessToken, topic)
	capturemetrics.ObserveRequest(connectorName, capturemetrics.OpOther, 0, err, time.Since(start))
	return historyID, expires, err
}

// timedAuthorizer counts every token-endpoint round trip as a provider call:
// each one is a refresh at Google, and a backfill makes one per page.
type timedAuthorizer struct{ googleconn.Authorizer }

func (a timedAuthorizer) AccessToken(ctx context.Context, refreshToken string) (string, error) {
	start := time.Now()
	access, err := a.Authorizer.AccessToken(ctx, refreshToken)
	capturemetrics.ObserveRequest(connectorName, capturemetrics.OpToken, 0, err, time.Since(start))
	return access, err
}

func (a timedAuthorizer) Exchange(ctx context.Context, code, redirectURI string) (oauthflow.TokenGrant, error) {
	start := time.Now()
	grant, err := a.Authorizer.Exchange(ctx, code, redirectURI)
	capturemetrics.ObserveRequest(connectorName, capturemetrics.OpToken, 0, err, time.Since(start))
	return grant, err
}

// getRaw downloads one message in full, timed as a backfill's fetch stage.
func (w *pageWalk) getRaw(ctx context.Context, id string) (Message, error) {
	start := time.Now()
	msg, err := w.c.api.GetRaw(ctx, w.access, id)
	capturemetrics.ObserveStage(ctx, capturemetrics.StageFetch, time.Since(start))
	return msg, err
}

// getHeaders reads one message's headers, timed as a backfill's
// fetch_headers stage.
func (w *pageWalk) getHeaders(ctx context.Context, headers HeaderFetcher, id string) (Message, error) {
	start := time.Now()
	msg, err := headers.GetHeaders(ctx, w.access, id)
	capturemetrics.ObserveStage(ctx, capturemetrics.StageFetchHeaders, time.Since(start))
	return msg, err
}

// observeGatePause records how much longer a rate limit closed the page's
// gate, beside the Retry-After the provider asked for with it. Concurrent
// refusals that fall inside a pause already set add nothing.
func observeGatePause(ctx context.Context, extended time.Duration, cause error) {
	if extended <= 0 {
		return
	}
	var asked time.Duration
	if limited, ok := errors.AsType[*connector.RateLimitedError](cause); ok {
		asked = limited.RetryAfter
	}
	capturemetrics.ObserveInPageWait(ctx, extended, asked)
}

// settleMessage settles one message through settle and counts it under the
// outcome it came to: refused when the page walked past its failure, failed
// when the failure ended the page.
func (w *pageWalk) settleMessage(message *capturemetrics.Message, id string, captured bool, err error) error {
	settled := w.settle(id, captured, err)
	if err != nil && settled == nil {
		message.Refuse()
	} else {
		message.End(captured, settled)
	}
	return settled
}

// parseTimed parses one fetched message, timed as a backfill's parse stage.
func parseTimed(ctx context.Context, raw []byte, owner string) (mailmap.Message, error) {
	start := time.Now()
	msg, err := mailmap.Parse(raw, owner)
	capturemetrics.ObserveStage(ctx, capturemetrics.StageParse, time.Since(start))
	return msg, err
}

// getOp names the call a GET makes: the id listing, a message's headers or its
// full RAW download, the history delta, or another.
func getOp(path string, q url.Values) string {
	switch {
	case path == "/messages":
		return capturemetrics.OpList
	case strings.HasPrefix(path, "/messages/") && q.Get("format") == "metadata":
		return capturemetrics.OpGetMetadata
	case strings.HasPrefix(path, "/messages/"):
		return capturemetrics.OpGetRaw
	case path == "/history":
		return capturemetrics.OpHistory
	default:
		return capturemetrics.OpOther
	}
}
