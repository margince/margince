// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// A draft's files, reason for contact and moment, each held to the send's own rule.

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// prepareDraft readies a draft's content for the row, for the composer's save
// and an agent's alike.
//
// A file the caller cannot see, or one the send would refuse, is left out and
// counted. Refusing it would lose the words of an autosave over one file.
func (s *Store) prepareDraft(ctx context.Context, anchor MailDraftAnchor, content MailDraftContent) (MailDraftContent, resolvedFiles, error) {
	content = canonicalDraft(content)
	if content.Schedule != nil {
		schedule, err := normalizeSchedule(*content.Schedule)
		if err != nil {
			return MailDraftContent{}, resolvedFiles{}, err
		}
		content.Schedule = &schedule
	}
	if err := validateDraft(anchor, content, s.now()); err != nil {
		return MailDraftContent{}, resolvedFiles{}, err
	}
	files, err := s.draftFilesOf(ctx, content.AttachmentIDs)
	if err != nil {
		return MailDraftContent{}, resolvedFiles{}, err
	}
	content.AttachmentIDs = make([]ids.UUID, 0, len(files.attachable))
	for _, file := range files.attachable {
		content.AttachmentIDs = append(content.AttachmentIDs, file.AttachmentID)
	}
	return content, files, nil
}

// draftFilesOf is what the chosen files are to the caller now. A read runs it
// too, so a file archived or hidden since the save is left out, not a failure.
func (s *Store) draftFilesOf(ctx context.Context, chosen []ids.UUID) (resolvedFiles, error) {
	return sortChosenFiles(ctx, chosen, s.sendableFile)
}

// validateDraftSending applies the send's own rules to the same three fields.
func validateDraftSending(content MailDraftContent, now time.Time) error {
	if _, err := boundAttachmentIDs(content.AttachmentIDs); err != nil {
		return err
	}
	if content.Context != "" {
		claimed := string(content.Context)
		if _, err := sendContextFrom(&claimed, nil, nil, nil); err != nil {
			return err
		}
	}
	if content.Schedule != nil {
		return validateSchedule(*content.Schedule, now)
	}
	return nil
}

// draftScheduleColumns leaves legacy instants distinct from saved local choices.
func draftScheduleColumns(schedule *SendSchedule) (*time.Time, *string, *string) {
	if schedule == nil {
		return nil, nil, nil
	}
	at := schedule.At.UTC()
	return &at, &schedule.TZ, schedule.wallClock()
}
