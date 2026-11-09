// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package imap

// Listing the next page of new UIDs in bounded server-side ranges. Each pull
// asks for UID ranges from the watermark up, doubling while they come back
// short. So a pull reads UIDs in proportion to its page, not to the backlog.

import (
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
	// maxDoublings bounds how far one pull widens its range. A gap wider than
	// the widest range takes another pull to cross.
	maxDoublings = 16
)

// uidPage is one pull's listing: the oldest new UIDs, ascending, and the
// highest UID a range proved empty below UIDNEXT when no UID was found.
type uidPage struct {
	uids         []imapv2.UID
	emptyThrough uint32
}

// uidRanges answers which UIDs the selected mailbox holds in [lo, hi].
type uidRanges interface {
	inRange(ctx context.Context, lo, hi uint32) ([]imapv2.UID, error)
}

// listNewUIDs lists up to want UIDs at or above from. With ESEARCH, one MIN
// search finds the first new UID, so a gap costs nothing. Without it the
// ranges walk the gap and report how far it was empty.
func listNewUIDs(ctx context.Context, client *imapclient.Client, from, uidNext, want uint32) (uidPage, error) {
	caps := client.Caps()
	if !caps.Has(imapv2.CapESearch) && !caps.Has(imapv2.CapIMAP4rev2) {
		return pageFrom(ctx, fetchRanges{client}, from, uidNext, want)
	}
	first, ok, err := firstUIDFrom(client, from)
	if err != nil {
		return uidPage{}, err
	}
	if !ok {
		// Nothing at or above from exists now, and every UID below UIDNEXT
		// is taken or gone for good.
		var page uidPage
		if uidNext > from {
			page.emptyThrough = uidNext - 1
		}
		return page, nil
	}
	return pageFrom(ctx, searchRanges{client}, first, uidNext, want)
}

// pageFrom walks ranges [lo, lo+span-1] upward from from, doubling span after
// each range, until it holds want UIDs, reaches UIDNEXT, or has doubled
// maxDoublings times. Each range is read once.
func pageFrom(ctx context.Context, src uidRanges, from, uidNext, want uint32) (uidPage, error) {
	var page uidPage
	span := want * firstSpanPages
	lo := from
	for step := 0; step <= maxDoublings; step++ {
		if err := ctx.Err(); err != nil {
			return uidPage{}, err
		}
		hi := uint32(math.MaxUint32)
		if span-1 <= math.MaxUint32-lo {
			hi = lo + span - 1
		}
		found, err := src.inRange(ctx, lo, hi)
		if err != nil {
			return uidPage{}, err
		}
		page.uids = append(page.uids, found...)
		belowNext := uidNext > 0 && hi < uidNext
		if len(page.uids) == 0 && belowNext {
			page.emptyThrough = hi
		}
		if len(page.uids) >= int(want) || !belowNext {
			break
		}
		lo = hi + 1
		if span <= math.MaxUint32/2 {
			span *= 2
		}
	}
	if len(page.uids) > int(want) {
		page.uids = page.uids[:want]
	}
	return page, nil
}

// firstUIDFrom asks the server for the lowest UID at or above from. A range
// from:* always matches the highest message, so a minimum below from means
// nothing new.
func firstUIDFrom(client *imapclient.Client, from uint32) (uint32, bool, error) {
	var set imapv2.UIDSet
	set.AddRange(imapv2.UID(from), 0)
	data, err := client.UIDSearch(&imapv2.SearchCriteria{UID: []imapv2.UIDSet{set}},
		&imapv2.SearchOptions{ReturnMin: true}).Wait()
	if err != nil {
		return 0, false, fmt.Errorf("imap: searching for new mail: %w", errors.Join(ErrUnreachable, err))
	}
	return data.Min, data.Min >= from, nil
}

// searchRanges lists a range with `UID SEARCH RETURN (ALL COUNT)`, which the
// server answers as one compact set.
type searchRanges struct{ client *imapclient.Client }

func (s searchRanges) inRange(_ context.Context, lo, hi uint32) ([]imapv2.UID, error) {
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
	uids, ok := all.Nums()
	if !ok {
		return nil, fmt.Errorf("imap: listing new mail: the server answered with an open-ended set: %w", ErrUnreachable)
	}
	slices.Sort(uids)
	return inBounds(uids, lo, hi), nil
}

// fetchRanges lists a range with UID FETCH lo:hi (UID), for a server without
// ESEARCH. The range is closed, so the answer is at most hi-lo+1 UIDs.
type fetchRanges struct{ client *imapclient.Client }

func (f fetchRanges) inRange(_ context.Context, lo, hi uint32) ([]imapv2.UID, error) {
	var set imapv2.UIDSet
	set.AddRange(imapv2.UID(lo), imapv2.UID(hi))
	cmd := f.client.Fetch(set, &imapv2.FetchOptions{UID: true})
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
	return inBounds(uids, lo, hi), nil
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
