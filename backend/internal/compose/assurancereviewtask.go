// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Delivering a REVIEW: one task per review episode, kept once delivered.
//
// An ordinary finding re-asks after its task is done, because the condition
// being true again after a rep acted is a new question. A review is different:
// completing its task does not answer it. Only somebody's answer on the finding
// does. So the episode remembers the task it was delivered as, and while that
// episode lasts no second task is minted, done or not.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// reviewFinding is the subject's review finding, if it has one. The review
// keys the task, so the other findings on the deal join the review's task
// whatever order they sorted in.
func reviewFinding(subject subjectFindings) (assurance.Exception, bool) {
	for _, finding := range subject.findings {
		if slices.Contains(assurance.ReviewTypes(), finding.Type) {
			return finding, true
		}
	}
	return assurance.Exception{}, false
}

// reviewTaskKey keys a review's task on the finding and its episode, never on
// the run: two passes over one unchanged episode are the same delivery.
func reviewTaskKey(review assurance.Exception, episode int) string {
	return fmt.Sprintf("review:%s:%d", review.ID, episode)
}

// bundleReview delivers one review episode and files the subject's findings
// under its task. It reports whether it minted.
func bundleReview(
	ctx context.Context, deps bundleDeps, cycle ids.UUID, subject subjectFindings, review assurance.Exception,
) (bool, error) {
	episode, linked, err := readEpisodeTask(ctx, deps, review.ID)
	if err != nil {
		return false, err
	}
	if linked != nil {
		return false, fileUnderTask(ctx, deps, cycle, *linked, subject)
	}
	task, found, err := deps.bundles.OpenTaskFor(ctx, cycle, subject.kind, subject.id)
	if err != nil {
		return false, fmt.Errorf("compose: asking for the subject's task: %w", err)
	}
	if !found {
		// An open task an earlier pass left, keyed by its run: adopted, so a
		// review that began before episodes were kept is not delivered twice.
		if task, found, err = adoptOpenTask(ctx, deps, subject); err != nil {
			return false, err
		}
		if found {
			if err := alignAdoptedTask(ctx, deps, task, review); err != nil {
				return false, err
			}
		}
	}
	minted := false
	if !found {
		due, err := reviewDueAt(ctx, deps)
		if err != nil {
			return false, err
		}
		if task, err = mintBundleTask(ctx, deps, reviewTaskKey(review, episode), subject, &due); err != nil {
			return false, err
		}
		minted = true
	}
	task, err = linkEpisodeTask(ctx, deps, review.ID, episode, task)
	if err != nil {
		return minted, err
	}
	return minted, fileUnderTask(ctx, deps, cycle, task, subject)
}

func readEpisodeTask(ctx context.Context, deps bundleDeps, review ids.UUID) (int, *ids.UUID, error) {
	var episode int
	var linked *ids.UUID
	err := deps.bundles.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		episode, linked, err = deps.bundles.EpisodeTask(ctx, tx, review)
		return err
	})
	if err != nil {
		return 0, nil, fmt.Errorf("compose: reading the review's episode: %w", err)
	}
	return episode, linked, nil
}

// linkEpisodeTask records the delivered task and answers the task the episode
// keeps. A concurrent delivery that linked first wins: both minted the same
// keyed task, and the link it wrote is the one to file under.
func linkEpisodeTask(ctx context.Context, deps bundleDeps, review ids.UUID, episode int, task ids.UUID) (ids.UUID, error) {
	err := deps.bundles.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return deps.bundles.LinkEpisodeTask(ctx, tx, review, episode, task)
	})
	if !errors.Is(err, apperrors.ErrVersionSkew) {
		return task, err
	}
	_, linked, err := readEpisodeTask(ctx, deps, review)
	if err != nil {
		return ids.UUID{}, err
	}
	if linked == nil {
		return ids.UUID{}, errors.New("compose: the review's episode moved while its task was delivered; the next pass delivers it")
	}
	return *linked, nil
}

// alignAdoptedTask gives an adopted task what a minted review task carries: a
// due date and the deal owner. It only fills what is empty, so a date or an
// assignee somebody chose stays, and a concurrent edit wins over this one.
func alignAdoptedTask(ctx context.Context, deps bundleDeps, task ids.UUID, review assurance.Exception) error {
	act, err := deps.activities.GetActivity(ctx, ids.From[ids.ActivityKind](task), storekit.LiveOnly)
	if err != nil {
		return fmt.Errorf("compose: reading the adopted task: %w", err)
	}
	var in activities.UpdateActivityInput
	if act.DueAt == nil {
		due, err := reviewDueAt(ctx, deps)
		if err != nil {
			return err
		}
		in.DueAt = &due
	}
	if act.AssigneeId == nil && review.OwnerID != nil {
		owner := ids.From[ids.UserKind](*review.OwnerID)
		in.AssigneeID = &owner
	}
	if in.DueAt == nil && in.AssigneeID == nil {
		return nil
	}
	in.IfVersion = (*int64)(act.Version)
	_, err = deps.activities.UpdateActivity(ctx, ids.From[ids.ActivityKind](task), in)
	if in.AssigneeID != nil && unassignableSeat(err) {
		// The owner cannot hold work: the task keeps the date and waits in the
		// unassigned queue, as a minted one would.
		in.AssigneeID = nil
		if in.DueAt == nil {
			return nil
		}
		_, err = deps.activities.UpdateActivity(ctx, ids.From[ids.ActivityKind](task), in)
	}
	if errors.Is(err, apperrors.ErrVersionSkew) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("compose: giving the adopted task its due date and owner: %w", err)
	}
	return nil
}

// reviewDueAt is the end of the next calendar day in the installation's zone.
// It counts from delivery, not from the scan, so a late delivery never hands
// somebody a task that is already overdue.
func reviewDueAt(ctx context.Context, deps bundleDeps) (time.Time, error) {
	var due time.Time
	err := deps.bundles.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		zone, err := installationZoneTx(ctx, tx)
		if err != nil {
			return err
		}
		y, m, d := deps.now().In(zone).Date()
		due = time.Date(y, m, d+1, 23, 59, 0, 0, zone)
		return nil
	})
	return due, err
}
