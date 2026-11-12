// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graph

// The messages delta: how a mailbox is read forward. An anchor opens a filtered
// round over one well-known folder and walks it to a deltaLink; every later
// cycle resumes from the link the last one stored.
//
// The watermark only ever moves forward, so a round that fell short of its own
// window would drop what it missed for good. That is why an anchor reconciles
// its walk against the folder's own tally before the link it ends on is allowed
// to stand as a watermark.

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"time"
)

// receivedAfterFilter renders Graph's only supported delta/list filter:
// receivedDateTime ge <instant>. The window boundary is a product parameter
// measured in months, so second-grain UTC rendering loses nothing.
func receivedAfterFilter(after time.Time) string {
	return "receivedDateTime ge " + filterInstant(after).Format(time.RFC3339)
}

// filterInstant is the lower bound AS MICROSOFT WILL READ IT: second-grain,
// because that is what RFC3339 rendering leaves of it.
//
// Spelled once and shared with the reconciliation, so the two cannot drift. They
// did: the filter asked Graph for everything from the top of the second while
// the tally compared against the exact instant, so a message arriving in the
// fractional remainder counted on Microsoft's side and not on ours — a complete
// anchor refused as short, every time a mailbox happened to receive one there.
//
// Held by: TestASubsecondLowerBoundDoesNotRefuseACompleteAnchor
// (backend/internal/modules/capture/graph/client_test.go)
func filterInstant(t time.Time) time.Time { return t.UTC().Truncate(time.Second) }

// receivedBetweenFilter closes that window at the top: ge <after> and lt
// <before>.
//
// The upper bound keeps its subsecond digits (RFC3339Nano) where the lower one
// does not need them. The lower bound is a product parameter measured in
// months; this one is the instant a walk opened, and Format TRUNCATES, so
// second-grain rendering would move it back by up to a second and drop whatever
// arrived in that second out of the tally the walk is measured against. Losing
// it in that direction is losing it quietly, which is the whole failure this
// window exists to catch.
func receivedBetweenFilter(after, before time.Time) string {
	return receivedAfterFilter(after) + " and receivedDateTime lt " + before.UTC().Format(time.RFC3339Nano)
}

// The two well-known folders a standing sync follows. Named constants because
// they are Microsoft's own identifiers rather than folder names a contact chose,
// and because which folder a message came from is what attests whether the
// owner sent it.
const (
	folderInbox = "inbox"
	folderSent  = "sentitems"
)

// syncedFolders are the only folders mail capture reads, Sent Items first. The
// standing sync walks them and a history import keeps to them, so Junk Email,
// Deleted Items, Drafts and every other folder are never fetched.
var syncedFolders = [...]string{folderSent, folderInbox}

// deltaPage is one page of a messages delta. A tombstoned entry
// carries @removed instead of message fields — nothing to fetch for it.
type deltaPage struct {
	Value []struct {
		ID      string           `json:"id"`
		Removed *json.RawMessage `json:"@removed"`
		// ReceivedAt is what decides whether an entry belongs to the window a
		// filtered round asked for. A delta tracks a FOLDER, not a query, so
		// Microsoft may emit an entry for a message outside the filter when
		// something about it changed — a read/unread flip on an old message.
		// Zero where the entry does not say.
		ReceivedAt        time.Time `json:"receivedDateTime"`  //nolint:tagliatelle // Microsoft's wire format; must match to decode
		InternetMessageID string    `json:"internetMessageId"` //nolint:tagliatelle // Microsoft's wire format; must match to decode
	} `json:"value"`
	NextLink  string `json:"@odata.nextLink"`  //nolint:tagliatelle // Microsoft's wire format; must match to decode
	DeltaLink string `json:"@odata.deltaLink"` //nolint:tagliatelle // Microsoft's wire format; must match to decode
}

// anchorSeenCap bounds the distinct messages one anchor round can count. The
// set rides in the stored cursor to survive checkpoints. At 22 bytes an id,
// the cap is about 2.2 MB of cursor.
const anchorSeenCap = 100_000

// ErrAnchorTooLarge is an anchor round whose window holds more messages than
// its census can hold. The round stops rather than certify a count it cannot
// keep exact.
var ErrAnchorTooLarge = errors.New("graph: the anchor window holds more messages than one certified round can count")

// ErrAnchorShort is a census that closed below the folder's tally. It is
// unreachable, so the cycle retries, but the round it walked is not resumed.
var ErrAnchorShort = fmt.Errorf("graph: the anchor census closed short: %w", ErrUnreachable)

// DeltaPage is one page of a delta round, handed over before the next page is
// read, with what a checkpoint stores to continue after it.
type DeltaPage struct {
	Messages []MessageRef
	Removed  []string
	Resume   DeltaResume
}

// DeltaResume is where a round continues after a page: the link read next, and
// for an anchor what its census has counted. NextLink is empty after the
// round's last page.
type DeltaResume struct {
	NextLink string        `json:"next_link"`
	Anchor   *AnchorCensus `json:"anchor,omitempty"`
}

// PageVisitor captures one page. An error stops the round where it is.
type PageVisitor func(context.Context, DeltaPage) error

// AnchorCensus counts the distinct messages an anchor round walked inside the
// window it reconciles, [After, Opened). Each id is kept as a 16-byte SHA-256
// prefix: an exact set, not a filter that answers "maybe".
type AnchorCensus struct {
	After  time.Time
	Opened time.Time
	seen   map[string]struct{}
}

func newAnchorCensus(after, opened time.Time) *AnchorCensus {
	return &AnchorCensus{After: after, Opened: opened, seen: map[string]struct{}{}}
}

// count adds one entry. A tombstone, a move out of the folder included, takes
// its id back out: the tally counts only what the folder still holds. An entry
// outside the window or without an instant does not count; see countedInWindow.
func (c *AnchorCensus) count(id string, receivedAt time.Time, removed bool) error {
	digest := idDigest(id)
	if removed {
		delete(c.seen, digest)
		return nil
	}
	if !countedInWindow(receivedAt, c.After, c.Opened) {
		return nil
	}
	if _, ok := c.seen[digest]; ok {
		return nil
	}
	if len(c.seen) >= anchorSeenCap {
		return fmt.Errorf("%w: more than %d messages since %s; run the history import for this mailbox, then re-anchor it",
			ErrAnchorTooLarge, anchorSeenCap, c.After.UTC().Format(time.RFC3339))
	}
	c.seen[digest] = struct{}{}
	return nil
}

func idDigest(id string) string {
	sum := sha256.Sum256([]byte(id))
	return base64.RawURLEncoding.EncodeToString(sum[:16])
}

type anchorCensusJSON struct {
	After  time.Time `json:"after"`
	Opened time.Time `json:"opened"`
	Seen   []string  `json:"seen"`
}

// MarshalJSON stores the set sorted, so one census always reads the same.
func (c *AnchorCensus) MarshalJSON() ([]byte, error) {
	seen := make([]string, 0, len(c.seen))
	for digest := range c.seen {
		seen = append(seen, digest)
	}
	slices.Sort(seen)
	return json.Marshal(anchorCensusJSON{After: c.After, Opened: c.Opened, Seen: seen})
}

// UnmarshalJSON refuses a stored set past the cap rather than resuming a
// census it could no longer keep exact.
func (c *AnchorCensus) UnmarshalJSON(b []byte) error {
	var stored anchorCensusJSON
	if err := json.Unmarshal(b, &stored); err != nil {
		return err
	}
	if len(stored.Seen) > anchorSeenCap {
		return fmt.Errorf("graph: a stored anchor census holds %d ids, over the cap of %d", len(stored.Seen), anchorSeenCap)
	}
	c.After, c.Opened = stored.After, stored.Opened
	c.seen = make(map[string]struct{}, len(stored.Seen))
	for _, digest := range stored.Seen {
		c.seen[digest] = struct{}{}
	}
	return nil
}

// countedInWindow reports whether an entry belongs to [after, before), the
// window a tally covers, with the lower bound read the way Microsoft reads it.
// The round asks for the instant, so an entry without one does not count: its
// zero time precedes every window. A short census is retried next cycle.
func countedInWindow(receivedAt, after, before time.Time) bool {
	return !receivedAt.Before(filterInstant(after)) && receivedAt.Before(before)
}

// DeltaInit starts a fresh anchor round, or resumes one from resume, handing
// each page to visit. The round's deltaLink is returned only after its census
// reaches the folder's own tally.
func (a *httpAPI) DeltaInit(ctx context.Context, accessToken, folder string, after time.Time, resume *DeltaResume, visit PageVisitor) (string, error) {
	census, start, err := a.anchorStart(folder, after, resume)
	if err != nil {
		return "", err
	}
	deltaLink, err := a.deltaWalk(ctx, accessToken, start, census, visit)
	if err != nil {
		return "", err
	}
	// A filtered delta that closes early also ends with a deltaLink. The
	// watermark only moves forward, so the census must reach the folder's
	// tally before it may stand.
	held, err := a.countInFolder(ctx, accessToken, folder, receivedBetweenFilter(census.After, census.Opened))
	if err != nil {
		return "", err
	}
	// The tally runs after the walk, over a window closed when the walk opened.
	// Mail that arrives or is deleted meanwhile can only raise the census side.
	// The census counts only in-window entries, so changed older mail cannot pad it.
	if walked := len(census.seen); walked < held {
		return "", fmt.Errorf(
			"graph: the %s anchor closed holding %d message(s) from the window but the folder holds %d since %s: %w",
			folder, walked, held, census.After.UTC().Format(time.RFC3339), ErrAnchorShort)
	}
	return deltaLink, nil
}

// anchorStart returns the census and first URL of an anchor round. It opens a
// fresh round now, or resumes the one a checkpoint stored.
func (a *httpAPI) anchorStart(folder string, after time.Time, resume *DeltaResume) (*AnchorCensus, string, error) {
	if resume != nil {
		if resume.Anchor == nil || resume.Anchor.seen == nil || resume.NextLink == "" {
			return nil, "", fmt.Errorf("graph: a stored %s anchor round names no census or link: %w", folder, ErrUnreachable)
		}
		if err := a.sameMailbox(resume.NextLink); err != nil {
			return nil, "", err
		}
		return resume.Anchor, resume.NextLink, nil
	}
	// $select asks for receivedDateTime, which Graph does not promise by
	// default. It rides every nextLink and the deltaLink. Exchange stamps it on
	// sent mail too, so it bounds both folders.
	q := url.Values{
		paramFilter: {receivedAfterFilter(after)},
		paramSelect: {"id,receivedDateTime"},
	}
	return newAnchorCensus(after, time.Now().UTC()),
		a.root() + "/mailFolders/" + url.PathEscape(folder) + "/messages/delta?" + q.Encode(), nil
}

// countInFolder reports how many messages one folder holds for a filter, so a
// walk over that folder can be measured against the provider's own tally
// rather than trusted because its last round closed.
func (a *httpAPI) countInFolder(ctx context.Context, accessToken, folder, filter string) (int, error) {
	var out struct {
		// A POINTER, so "Microsoft said none" is distinguishable from "Microsoft
		// did not answer the question". Read as an int, a 2xx that omits
		// @odata.count — the shape a request missing ConsistencyLevel can take —
		// would arrive as a tally of zero, and a tally of zero is one no walk can
		// ever fall short of. The reconciliation would pass every round while
		// measuring nothing, which is the failure it exists to catch wearing the
		// costume of a green check.
		Count *int `json:"@odata.count"` //nolint:tagliatelle // Microsoft's wire format; must match to decode
	}
	q := url.Values{
		paramFilter: {filter},
		paramCount:  {"true"},
		paramSelect: {"id"},
		paramTop:    {"1"},
	}
	// $count=true needs the eventual-consistency header on Graph.
	hdr := http.Header{"ConsistencyLevel": {"eventual"}}
	u := a.root() + "/mailFolders/" + url.PathEscape(folder) + "/messages?" + q.Encode()
	if _, err := a.get(ctx, accessToken, u, hdr, &out); err != nil {
		return 0, err
	}
	if out.Count == nil {
		return 0, fmt.Errorf(
			"graph: the %s tally came back without a count: %w", folder, ErrUnreachable)
	}
	return *out.Count, nil
}

// Delta resumes a folder's round from its stored deltaLink, or from the page a
// checkpoint stored, handing each page to visit.
func (a *httpAPI) Delta(ctx context.Context, accessToken, deltaLink string, resume *DeltaResume, visit PageVisitor) (string, error) {
	start := deltaLink
	if resume != nil {
		start = resume.NextLink
	}
	if err := a.sameMailbox(start); err != nil {
		return "", err
	}
	// Folder tombstones include archive moves, which change even immutable ids.
	// They cannot authorize irreversible deletion, so a page reports none.
	return a.deltaWalk(ctx, accessToken, start, nil, visit)
}

// deltaWalk follows a round from startURL through every nextLink to the
// closing deltaLink, handing visit one page at a time. An anchor round counts
// each page into census first. A 410 anywhere returns ErrDeltaGone.
func (a *httpAPI) deltaWalk(ctx context.Context, accessToken, startURL string, census *AnchorCensus, visit PageVisitor) (string, error) {
	next := startURL
	for {
		var page deltaPage
		status, err := a.get(ctx, accessToken, next, nil, &page)
		if err != nil {
			if status == http.StatusGone {
				return "", ErrDeltaGone
			}
			return "", err
		}
		messages, err := pageMessages(page, census)
		if err != nil {
			return "", err
		}
		if page.NextLink == "" {
			if page.DeltaLink == "" {
				// A round with neither link has lost its cursor. It is
				// unreachable, never a success without a watermark.
				return "", ErrUnreachable
			}
			// The deltaLink is stored, so it gets the origin check too. An
			// off-origin one would fail every later cycle with no 410 to
			// re-anchor on.
			if err := a.sameMailbox(page.DeltaLink); err != nil {
				return "", err
			}
			if err := visit(ctx, DeltaPage{Messages: messages, Resume: DeltaResume{Anchor: census}}); err != nil {
				return "", err
			}
			return page.DeltaLink, nil
		}
		if err := a.sameMailbox(page.NextLink); err != nil {
			return "", err
		}
		if err := visit(ctx, DeltaPage{Messages: messages, Resume: DeltaResume{NextLink: page.NextLink, Anchor: census}}); err != nil {
			return "", err
		}
		next = page.NextLink
	}
}

// pageMessages is a page's fetchable messages, counted into census first when
// the round is an anchor. A tombstone carries nothing to fetch.
func pageMessages(page deltaPage, census *AnchorCensus) ([]MessageRef, error) {
	var messages []MessageRef
	for _, m := range page.Value {
		if m.ID == "" {
			continue
		}
		receivedAt := m.ReceivedAt.UTC()
		removed := m.Removed != nil
		if census != nil {
			if err := census.count(m.ID, receivedAt, removed); err != nil {
				return nil, err
			}
		}
		if !removed {
			messages = append(messages, MessageRef{ID: m.ID, ReceivedAt: receivedAt, InternetMessageID: m.InternetMessageID})
		}
	}
	return messages, nil
}

// sameAPIOrigin refuses any continuation URL that does not live under the
// configured Graph base. nextLink/deltaLink are server-supplied URLs the
// client follows bearing the access token — and the deltaLink round-trips
// through the stored sync cursor — so an off-origin link (tampered cursor,
// broken provider) must never be fetched.
