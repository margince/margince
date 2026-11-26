// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package imap

// Listing the next page of new UIDs in bounded server-side ranges. Each pull
// asks for UID ranges from the watermark up, doubling while they come back
// short. So a pull reads UIDs in proportion to its page, not to the backlog.

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"slices"

	imapv2 "github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

const (
	// firstSpanPages sizes the first range at this many pages of UIDs.
	firstSpanPages = 4
	// maxSpanPages caps a range at this many pages, so no single answer
	// carries more UIDs than that, however sparse the mailbox below it.
	maxSpanPages = 64
	// maxDoublings bounds how many ranges one pull reads. A gap wider than
	// they cover takes another pull to cross.
	maxDoublings = 16
)

// uidPage is one pull's listing: the oldest new UIDs, ascending, and the
// highest UID a range proved empty when no UID was found.
type uidPage struct {
	uids         []imapv2.UID
	emptyThrough uint32
}

// uidRanges answers the first limit UIDs the selected mailbox holds in
// [lo, hi], ascending.
type uidRanges interface {
	inRange(ctx context.Context, lo, hi uint32, limit int) ([]imapv2.UID, error)
}

// listNewUIDs lists up to want UIDs at or above from, up to the highest UID
// the mailbox holds now. That UID bounds the walk, not UIDNEXT. A server may
// omit UIDNEXT or report it wrong. A UID below an existing one is never
// assigned later. With ESEARCH, one MIN search finds the first new UID, so a
// gap costs nothing.
func listNewUIDs(ctx context.Context, client *imapclient.Client, from, messages, want uint32) (uidPage, error) {
	if messages == 0 {
		return uidPage{}, nil
	}
	caps := client.Caps()
	esearch := caps.Has(imapv2.CapESearch) || caps.Has(imapv2.CapIMAP4rev2)
	top, err := highestUID(client, esearch, messages)
	if err != nil || top < from {
		return uidPage{}, err
	}
	if !esearch {
		return pageFrom(ctx, fetchRanges{client}, from, top, want)
	}
	first, err := firstUIDFrom(client, from, top)
	if err != nil {
		return uidPage{}, err
	}
	return pageFrom(ctx, searchRanges{client}, first, top, want)
}

// pageFrom walks ranges upward from from to top. Each range is twice the last,
// up to maxSpanPages pages. It stops at want UIDs, at top, or after
// maxDoublings+1 ranges. Each range is read once. A range that came back
// empty lies below top, which exists, so no UID will ever fill it.
func pageFrom(ctx context.Context, src uidRanges, from, top, want uint32) (uidPage, error) {
	var page uidPage
	span, widest := want*firstSpanPages, want*maxSpanPages
	for lo, step := from, 0; step <= maxDoublings; step++ {
		if err := ctx.Err(); err != nil {
			return uidPage{}, err
		}
		hi := top
		if span-1 < top-lo {
			hi = lo + span - 1
		}
		found, err := src.inRange(ctx, lo, hi, int(want)-len(page.uids))
		if err != nil {
			return uidPage{}, err
		}
		page.uids = append(page.uids, found...)
		if len(page.uids) == 0 {
			page.emptyThrough = hi
		}
		if len(page.uids) >= int(want) || hi == top {
			break
		}
		lo, span = hi+1, min(span*2, widest)
	}
	if len(page.uids) == 0 && page.emptyThrough == top {
		// top holds a message, so a range ending at it is never empty; a
		// server that says otherwise is not one to skip past.
		page.emptyThrough = 0
	}
	return page, nil
}

// highestUID asks the server for the highest UID it holds now: ESEARCH
// RETURN (MAX), or the UID of the last message by sequence number. Sequence
// numbers run 1 to the message count SELECT reported, so the last one holds the
// highest UID.
func highestUID(client *imapclient.Client, esearch bool, messages uint32) (uint32, error) {
	var all imapv2.UIDSet
	all.AddRange(1, 0)
	if esearch {
		data, err := client.UIDSearch(&imapv2.SearchCriteria{UID: []imapv2.UIDSet{all}},
			&imapv2.SearchOptions{ReturnMax: true}).Wait()
		if err != nil {
			return 0, fmt.Errorf("imap: reading the newest message: %w", errors.Join(ErrUnreachable, err))
		}
		return data.Max, nil
	}
	uids, err := fetchUIDs(client, imapv2.SeqSetNum(messages))
	if err != nil {
		return 0, err
	}
	var top uint32
	for _, u := range uids {
		top = max(top, uint32(u))
	}
	return top, nil
}

// firstUIDFrom asks the server for the lowest UID in [from, top]. top exists
// and is at least from, so the answer is never empty.
func firstUIDFrom(client *imapclient.Client, from, top uint32) (uint32, error) {
	var set imapv2.UIDSet
	set.AddRange(imapv2.UID(from), imapv2.UID(top))
	data, err := client.UIDSearch(&imapv2.SearchCriteria{UID: []imapv2.UIDSet{set}},
		&imapv2.SearchOptions{ReturnMin: true}).Wait()
	if err != nil {
		return 0, fmt.Errorf("imap: searching for new mail: %w", errors.Join(ErrUnreachable, err))
	}
	return max(data.Min, from), nil
}

// searchRanges lists a range with `UID SEARCH RETURN (ALL COUNT)`, which the
// server answers as one compact set.
type searchRanges struct{ client *imapclient.Client }

func (s searchRanges) inRange(_ context.Context, lo, hi uint32, limit int) ([]imapv2.UID, error) {
	var set imapv2.UIDSet
	set.AddRange(imapv2.UID(lo), imapv2.UID(hi))
	data, err := s.client.UIDSearch(&imapv2.SearchCriteria{UID: []imapv2.UIDSet{set}},
		&imapv2.SearchOptions{ReturnAll: true, ReturnCount: true}).Wait()
	if err != nil {
		return nil, fmt.Errorf("imap: listing new mail: %w", errors.Join(ErrUnreachable, err))
	}
	if data.Count == 0 {
		return nil, nil
	}
	all, ok := data.All.(imapv2.UIDSet)
	if !ok {
		return nil, fmt.Errorf("imap: listing new mail: the server answered a UID search without UIDs: %w", ErrUnreachable)
	}
	return firstInSet(all, lo, hi, limit), nil
}

// firstInSet reads the first limit UIDs of a compact set that lie in [lo, hi],
// ascending, range by range. It never expands the whole set, which can span
// the widest range a pull asks about.
func firstInSet(set imapv2.UIDSet, lo, hi uint32, limit int) []imapv2.UID {
	ranges := slices.Clone(set)
	for i, r := range ranges {
		if r.Start > r.Stop {
			ranges[i] = imapv2.UIDRange{Start: r.Stop, Stop: r.Start}
		}
	}
	slices.SortFunc(ranges, func(a, b imapv2.UIDRange) int { return cmp.Compare(a.Start, b.Start) })
	var uids []imapv2.UID
	for _, r := range ranges {
		start, stop := max(uint32(r.Start), lo), min(uint32(r.Stop), hi)
		for u := start; u <= stop && len(uids) < limit; u++ {
			if len(uids) == 0 || uint32(uids[len(uids)-1]) < u {
				uids = append(uids, imapv2.UID(u))
			}
			if u == math.MaxUint32 {
				break
			}
		}
	}
	return uids
}

// fetchRanges lists a range with UID FETCH lo:hi (UID), for a server without
// ESEARCH. The range is closed, so the answer is at most hi-lo+1 UIDs.
type fetchRanges struct{ client *imapclient.Client }

func (f fetchRanges) inRange(_ context.Context, lo, hi uint32, limit int) ([]imapv2.UID, error) {
	var set imapv2.UIDSet
	set.AddRange(imapv2.UID(lo), imapv2.UID(hi))
	uids, err := fetchUIDs(f.client, set)
	if err != nil {
		return nil, err
	}
	uids = inBounds(uids, lo, hi)
	if len(uids) > limit {
		uids = uids[:limit]
	}
	return uids, nil
}

// fetchUIDs runs FETCH (UID) over set and returns the UIDs, ascending.
func fetchUIDs(client *imapclient.Client, set imapv2.NumSet) ([]imapv2.UID, error) {
	cmd := client.Fetch(set, &imapv2.FetchOptions{UID: true})
	var uids []imapv2.UID
	for msg := cmd.Next(); msg != nil; msg = cmd.Next() {
		for item := msg.Next(); item != nil; item = msg.Next() {
			if it, ok := item.(imapclient.FetchItemDataUID); ok {
				uids = append(uids, it.UID)
			}
		}
	}
	if err := cmd.Close(); err != nil {
		return nil, fmt.Errorf("imap: listing new mail: %w", errors.Join(ErrUnreachable, err))
	}
	slices.Sort(uids)
	return uids, nil
}

// inBounds keeps the UIDs in [lo, hi]. A server may answer outside the range
// it was asked about, and a UID below the watermark is never read again.
func inBounds(uids []imapv2.UID, lo, hi uint32) []imapv2.UID {
	kept := uids[:0]
	for _, u := range uids {
		if uint32(u) >= lo && uint32(u) <= hi {
			kept = append(kept, u)
		}
	}
	return kept
}
