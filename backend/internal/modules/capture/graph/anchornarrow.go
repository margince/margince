// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graph

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// An anchor whose window holds more than its census can count is narrowed, not
// stopped. The lower bound moves later, to a point whose tally fits, and the
// older span is recorded as unread so the owner can import it.

// anchorFitTarget is the tally a narrowed window may hold. It leaves headroom
// under the cap because a walk also counts ids that churn while it runs.
const anchorFitTarget = anchorSeenCap * 9 / 10

// anchorFitResolution is how finely the narrowed bound is placed. An hour
// costs about a dozen tallies over a three-month window.
const anchorFitResolution = time.Hour

// AnchorTooLargeError is an anchor round past the census cap, with the lower
// bound a narrower round fits from. From..Fits is the span it leaves unread.
type AnchorTooLargeError struct {
	Folder string
	From   time.Time
	Fits   time.Time
}

func (e *AnchorTooLargeError) Error() string {
	return fmt.Sprintf("graph: the %s anchor from %s holds more than %d messages; it fits from %s",
		e.Folder, e.From.UTC().Format(time.RFC3339), anchorSeenCap, e.Fits.UTC().Format(time.RFC3339))
}

func (e *AnchorTooLargeError) Unwrap() error { return ErrAnchorTooLarge }

// narrowAnchor bisects census's window on receivedDateTime for the earliest
// lower bound whose tally fits under anchorFitTarget.
func (a *httpAPI) narrowAnchor(ctx context.Context, accessToken, folder string, census *AnchorCensus) error {
	tooLarge, fits := filterInstant(census.After), census.Opened
	for fits.Sub(tooLarge) > anchorFitResolution {
		mid := filterInstant(tooLarge.Add(fits.Sub(tooLarge) / 2))
		held, err := a.countInFolder(ctx, accessToken, folder, receivedBetweenFilter(mid, census.Opened))
		if err != nil {
			return err
		}
		if held <= anchorFitTarget {
			fits = mid
		} else {
			tooLarge = mid
		}
	}
	return &AnchorTooLargeError{Folder: folder, From: census.After, Fits: fits}
}

// reanchorNarrower records the span a too-large anchor leaves unread, drops the
// oversized round, and anchors the folder again from the bound that fits.
//
// The record comes first. A round that then fails leaves the span recorded,
// and the next cycle anchors from the moved bound.
func (c *Connector) reanchorNarrower(
	ctx context.Context, access, folder string, state *cursorState, tooLarge *AnchorTooLargeError, visit PageVisitor,
) (string, error) {
	if err := connector.NarrowSyncAnchor(ctx, tooLarge.From, tooLarge.Fits); err != nil {
		return "", errors.Join(tooLarge, err)
	}
	if err := dropRound(ctx, state, folder); err != nil {
		return "", err
	}
	return c.api.DeltaInit(ctx, access, folder, tooLarge.Fits, nil, visit)
}
