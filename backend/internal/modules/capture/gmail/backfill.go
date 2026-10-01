// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The bounded backfill (ADR-0063): Gmail enumerates a mailbox backward from
// a date boundary via messages.list q=after:. The estimate is the provider's
// resultSizeEstimate — an estimate by name and by nature; the page walk uses
// the same GetRaw + capture discipline as incremental sync, so a message the
// two paths both see lands once (the capture key dedupes).

package gmail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture/capturemetrics"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// backfillPageSize bounds one BackfillPage call; the engine commits cursor
// and counters between pages, so this is also the resume granularity.
const backfillPageSize = 100

var _ connector.Backfiller = (*Connector)(nil)

// afterQuery renders Gmail's after: operator. Gmail treats the date in the
// mailbox's own timezone and the operator is inclusive-day; the window
// boundary is a product parameter measured in months, so day-grain slack is
// noise.
func afterQuery(after time.Time) string {
	return "after:" + after.Format("2006/01/02")
}

// EstimateBackfill asks the provider how many messages the window holds, and
// whether that number is the total or a floor.
func (c *Connector) EstimateBackfill(ctx context.Context, auth connector.Auth, after time.Time) (connector.BackfillEstimate, error) {
	var st authState
	if err := json.Unmarshal(auth, &st); err != nil {
		return connector.BackfillEstimate{}, fmt.Errorf("gmail: malformed auth state: %w", err)
	}
	access, err := c.oauth.AccessToken(ctx, st.RefreshToken)
	if err != nil {
		return connector.BackfillEstimate{}, err
	}
	count, floor, err := c.api.EstimateAfter(ctx, access, afterQuery(after))
	if err != nil {
		return connector.BackfillEstimate{}, err
	}
	return connector.BackfillEstimate{Messages: count, Floor: floor}, nil
}

// BackfillPage pulls one page of the window, oldest-boundary inclusive,
// through the same capture path as incremental sync.
//
// It walks the page in two passes, each fetching a few messages at once
// (backfillpool.go) and settling them one at a time in the page's own order.
// The first reads only headers and asks the Sink whether the message would be
// kept out anyway (colleague mail, an exclusion); the second downloads in full
// only what may be kept, and captures it.
func (c *Connector) BackfillPage(ctx context.Context, auth connector.Auth, after time.Time, pageToken string, sink connector.Sink) (connector.BackfillPageResult, error) {
	var st authState
	if err := json.Unmarshal(auth, &st); err != nil {
		return connector.BackfillPageResult{}, fmt.Errorf("gmail: malformed auth state: %w", err)
	}
	access, err := c.oauth.AccessToken(ctx, st.RefreshToken)
	if err != nil {
		return connector.BackfillPageResult{}, err
	}
	gate := &rateGate{}
	type listing struct {
		ids  []string
		next string
	}
	page, err := rateLimited(ctx, gate, func() (listing, error) {
		ids, next, err := c.api.ListAfter(ctx, access, afterQuery(after), pageToken, backfillPageSize)
		return listing{ids: ids, next: next}, err
	})
	if err != nil {
		return connector.BackfillPageResult{}, err
	}
	w := &pageWalk{
		c: c, ctx: ctx, access: access, owner: st.Owner, sink: sink, gate: gate,
		ids: page.ids, progress: connector.BackfillProgressFrom(ctx),
		res: connector.BackfillPageResult{NextToken: page.next, Scanned: len(page.ids)},
	}
	if err := w.walk(); err != nil {
		return connector.BackfillPageResult{}, err
	}
	return w.res, nil
}

// pageWalk is one page's walk and its tally.
type pageWalk struct {
	c        *Connector
	ctx      context.Context //nolint:containedctx // one page's walk, created and finished inside BackfillPage
	access   string
	owner    string
	sink     connector.Sink
	gate     *rateGate
	ids      []string
	progress connector.BackfillReporter
	res      connector.BackfillPageResult
	// finished counts messages settled so far, in either pass. It is what the
	// engine hears as scanned: a page that dies mid-walk is retried from the
	// committed token, and reporting the whole listing up front would show
	// progress the retry then repeats.
	finished    int
	failedInRow int
}

// walk runs both passes.
func (w *pageWalk) walk() error {
	keep := w.ids
	if judge, ok := w.sink.(connector.PreStoreJudge); ok {
		if headers, ok := w.c.api.(HeaderFetcher); ok {
			var err error
			if keep, err = w.judgeHeaders(headers, judge); err != nil {
				return err
			}
		}
	}
	return inOrder(w.ctx, len(keep),
		func(ctx context.Context, i int) (Message, error) {
			return rateLimited(ctx, w.gate, func() (Message, error) { return w.getRaw(ctx, keep[i]) })
		},
		func(i int, msg Message, err error) error {
			msgCtx, message := capturemetrics.BeginMessage(w.ctx)
			if errors.Is(err, ErrMessageGone) {
				// Deleted or moved since this page was listed — nothing to fetch.
				// A routine 404 across a months-long window must not stall the run.
				return w.settleMessage(message, keep[i], false, nil)
			}
			if err != nil {
				return w.settleMessage(message, keep[i], false, err)
			}
			captured, err := captureOne(msgCtx, msg, w.sink, w.c.bounces, w.owner)
			return w.settleMessage(message, keep[i], captured, err)
		})
}

// judgeHeaders reads every message's headers and answers the ids that still
// need a full download. A message whose headers settle it — gone, a draft,
// spam, or kept out by the Sink's own gates — is counted here and not fetched.
// Anything the headers cannot settle the same way the full message would is
// downloaded in full and judged there, as before.
func (w *pageWalk) judgeHeaders(headers HeaderFetcher, judge connector.PreStoreJudge) ([]string, error) {
	keep := make([]string, 0, len(w.ids))
	err := inOrder(w.ctx, len(w.ids),
		func(ctx context.Context, i int) (Message, error) {
			return rateLimited(ctx, w.gate, func() (Message, error) { return w.getHeaders(ctx, headers, w.ids[i]) })
		},
		func(i int, msg Message, err error) error {
			id := w.ids[i]
			msgCtx, message := capturemetrics.BeginMessage(w.ctx)
			if errors.Is(err, ErrMessageGone) {
				return w.settleMessage(message, id, false, nil)
			}
			if err != nil {
				if endsPage(w.ctx, err) {
					return w.settleMessage(message, id, false, err)
				}
				keep = append(keep, id)
				return nil
			}
			drop, err := dropFromHeaders(msgCtx, msg, judge, w.owner)
			if w.ctx.Err() != nil {
				return w.ctx.Err()
			}
			if err != nil || !drop {
				// Not settled by its headers: the full download judges it, and a
				// fault here costs only the download this pass would have saved.
				keep = append(keep, id)
				return nil //nolint:nilerr // the full path decides and reports for itself
			}
			return w.settleMessage(message, id, false, nil)
		})
	return keep, err
}

// dropFromHeaders answers whether a message can be skipped on its headers
// alone. It applies exactly the checks captureOne applies before storing, in
// the same order, and drops only where those checks drop without needing the
// body: a draft or spam label, or the Sink's pre-store gates. A message the
// skip rules would drop is NOT dropped here, because that path also records a
// bounce from the full message; it is downloaded and judged in full.
func dropFromHeaders(ctx context.Context, fetched Message, judge connector.PreStoreJudge, owner string) (bool, error) {
	if hasDraftLabel(fetched.Labels) || hasRejectedLabel(fetched.Labels) {
		return true, nil
	}
	msg, err := mailmap.Parse(fetched.RFC822, owner)
	if err != nil {
		return false, nil //nolint:nilerr // the full download parses it again and decides
	}
	if _, drop := msg.SkipReason(); drop {
		return false, nil
	}
	rec := msg.AttestSentByOwner(fetched.FiledAsSent).ToRecord(connectorName, nil)
	rec.Containers = labelContainers(fetched.Labels)
	return judge.DropBeforeStore(ctx, rec)
}

// settle counts one finished message. A failure that ends the page is
// returned; a failure of this message alone is counted as failed and walked
// past, unless too many fail in a row.
func (w *pageWalk) settle(id string, captured bool, err error) error {
	switch {
	case err == nil && captured:
		w.res.Captured++
		w.failedInRow = 0
	case err == nil:
		w.res.Skipped++
		w.failedInRow = 0
	case endsPage(w.ctx, err):
		return err
	default:
		w.failedInRow++
		if w.failedInRow >= failedInARowEndsPage {
			return err
		}
		w.res.Failed++
		slog.WarnContext(w.ctx, "gmail: backfill walked past a message it could not capture",
			"message", id, "error_class", "internal", "err", err)
	}
	w.finished++
	// The live tally has no failed column: a failed message shows as skipped
	// until the page commits, and the commit files it under failed.
	w.progress.Observed(w.ctx, w.finished, w.res.Captured, w.res.Skipped+w.res.Failed)
	return nil
}

// paramMaxResults is Gmail's page-size query parameter.
const paramMaxResults = "maxResults"

const (
	// estimatePageSize is Gmail's max ids per messages.list page.
	estimatePageSize = 500
	// estimateMaxPages bounds the count so a very large mailbox cannot turn a
	// preview into a long scan: up to 500 × 40 = 20,000 messages are counted
	// exactly; beyond that the returned number is a FLOOR.
	//
	// Honest-but-low is a stated property now rather than an apology: the cap
	// is reported alongside the count, so a surface says "at least 20,000"
	// instead of a precise-looking number that is short by multiples. Raising
	// the cap was the other way out and buys the wrong thing — two hundred
	// provider calls inside a preview a human is waiting on, worst on exactly
	// the busy mailboxes where it binds — while what the consent argument
	// needs is knowing WHAT will be read, not counting it.
	estimateMaxPages = 40
)

// EstimateAfter counts the messages matching the query by paging their ids —
// metadata only, no bodies — up to a page cap. Gmail's own resultSizeEstimate
// is notoriously unreliable (off by multiples: a 1,300-message window can read
// as ~200), which is exactly the "made-up number" a user distrusts; an exact
// id count is a few cheap calls and honest. The count also feeds the spend
// preview, so its accuracy is a consent property, not just cosmetics.
func (a *httpAPI) EstimateAfter(ctx context.Context, accessToken, query string) (count int, floor bool, err error) {
	total, pageToken := 0, ""
	for range estimateMaxPages {
		ids, next, err := a.ListAfter(ctx, accessToken, query, pageToken, estimatePageSize)
		if err != nil {
			return 0, false, err
		}
		total += len(ids)
		if next == "" {
			return total, false, nil
		}
		pageToken = next
	}
	// The cap bound on a very large mailbox: the window holds at least this
	// many and the rest was not counted. Said rather than left to look exact —
	// the live meter is the source of truth once the import runs.
	return total, true, nil
}

// ListAfter returns one page of message ids matching the query.
func (a *httpAPI) ListAfter(ctx context.Context, accessToken, query, pageToken string, pageSize int) ([]string, string, error) {
	var out struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
		NextPageToken string `json:"nextPageToken"` //nolint:tagliatelle // Google names this field
	}
	q := url.Values{"q": {query}, paramMaxResults: {strconv.Itoa(pageSize)}, includeSpamTrash: {"false"}}
	if pageToken != "" {
		q.Set("pageToken", pageToken)
	}
	if status, err := a.get(ctx, accessToken, "/messages", q, &out, maxJSONResponseBytes); err != nil {
		if pageTokenRejected(status, pageToken, err) {
			return nil, "", ErrPageTokenRejected
		}
		return nil, "", err
	}
	ids := make([]string, 0, len(out.Messages))
	for _, m := range out.Messages {
		ids = append(ids, m.ID)
	}
	return ids, out.NextPageToken, nil
}
