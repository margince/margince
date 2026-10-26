// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The bound on how many files one message may name, and what happens to a
// caller who names one twice.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A send naming more files than a message may carry is refused BEFORE any of
// them is resolved.
//
// The contract's maxItems is documentation: nothing in this stack validates a
// request body against its schema, so this bound is the only thing between one
// request and a transaction per named id — thousands of them fit in a body well
// inside the chassis ceiling. Refused before the loop, so the cost of refusing
// does not scale with what was asked for.
func TestASendNamingMoreFilesThanAMessageMayCarryIsRefused(t *testing.T) {
	named := make([]ids.UUID, 0, maxAttachmentsPerSend+1)
	for range maxAttachmentsPerSend + 1 {
		named = append(named, ids.NewV7())
	}

	_, err := boundAttachmentIDs(named)

	var tooMany *TooManyAttachmentsError
	if !errors.As(err, &tooMany) {
		t.Fatalf("resolveAttachments → %v, want TooManyAttachmentsError", err)
	}
	if tooMany.Named != maxAttachmentsPerSend+1 || tooMany.Limit != maxAttachmentsPerSend {
		t.Errorf("the refusal reports %+v, want %d named against a limit of %d",
			tooMany, maxAttachmentsPerSend+1, maxAttachmentsPerSend)
	}
	// The caller can only fix this by sending fewer, so the reason has to say so
	// and the field has to name what to shorten.
	field, code, message := tooMany.FieldFault()
	if field != "attachment_ids" || code != "too_many_attachments" {
		t.Errorf("field fault = %q/%q, want attachment_ids/too_many_attachments", field, code)
	}
	if !strings.Contains(message, "second message") {
		t.Errorf("the refusal %q does not tell the caller what to do instead", message)
	}
}

// A file named twice counts once.
//
// Not politeness: counting the repeats would refuse a legitimate send while
// admitting the same total work under a shorter list, so the cap has to apply to
// what the message actually carries. And a message carrying one file twice is
// not something a message can mean — a recipient would see the same document in
// two parts.
func TestNamingOneFileTwiceCountsItOnce(t *testing.T) {
	one := ids.NewV7()
	named := make([]ids.UUID, 0, maxAttachmentsPerSend*3)
	for range maxAttachmentsPerSend * 3 {
		named = append(named, one)
	}

	got, err := boundAttachmentIDs(named)
	if err != nil {
		t.Fatalf("one file named %d times was refused: %v", maxAttachmentsPerSend*3, err)
	}
	if len(got) != 1 || got[0] != one {
		t.Errorf("the bounded set is %v, want the one file named once — a repeat reaching the resolve "+
			"is a second transaction and a second part in the message", got)
	}
}

// The ordinary case, asserted so the guard cannot pass by refusing everything:
// a set within the cap comes back whole and in the order it was given, because
// that order is what the recipient sees.
func TestASetWithinTheCapComesBackInOrder(t *testing.T) {
	named := []ids.UUID{ids.NewV7(), ids.NewV7(), ids.NewV7()}
	got, err := boundAttachmentIDs(named)
	if err != nil {
		t.Fatalf("a set of %d within the cap of %d was refused: %v", len(named), maxAttachmentsPerSend, err)
	}
	if len(got) != len(named) {
		t.Fatalf("the bounded set holds %d of %d files", len(got), len(named))
	}
	for i := range named {
		if got[i] != named[i] {
			t.Errorf("file %d is %v, want %v — the order the caller attached them in is the order they arrive", i, got[i], named[i])
		}
	}
}

// A scheduled read counts a file the send would refuse as unavailable. Anything
// else stops the read: a failed lookup is not a file that is gone.
func TestOnlyARefusedFileCountsAsUnavailable(t *testing.T) {
	t.Parallel()
	refused := []error{
		fmt.Errorf("resolving an attached file: %w", apperrors.ErrNotFound),
		fmt.Errorf("resolving an attached file: %w", apperrors.ErrPermissionDenied),
		&WithheldAttachmentError{Filename: "private.pdf"},
		&EmptyAttachmentError{Filename: "blank.pdf"},
	}
	for _, err := range refused {
		if !fileRefusedAtSend(err) {
			t.Errorf("%v did not count as a refused file", err)
		}
	}
	if fileRefusedAtSend(fmt.Errorf("resolving an attached file: %w", context.DeadlineExceeded)) {
		t.Error("a lookup that timed out counted as a refused file")
	}
}

// A message past waiting resolves none of its files: no edit reaches it.
func TestAMessagePastWaitingResolvesNoFiles(t *testing.T) {
	t.Parallel()
	for _, status := range []string{ScheduledStatusReleased, ScheduledStatusSent, ScheduledStatusCancelled} {
		row := ScheduledSend{Status: status, frozen: scheduledPayload{AttachmentIDs: []string{ids.NewV7().String()}}}
		files, err := (&Store{}).scheduledFilesOf(context.Background(), row)
		if err != nil || len(files.attachable) != 0 || files.unavailable != 0 {
			t.Errorf("%s message resolved its files: %+v, %v", status, files, err)
		}
	}
}

// An id the send could not carry is counted, never refused: a read answering
// with the send's own refusal would hide the message it reads.
func TestAFrozenIDTheSendCouldNotCarryIsCountedNotRefused(t *testing.T) {
	t.Parallel()
	named := make([]ids.UUID, maxAttachmentsPerSend+1)
	for i := range named {
		named[i] = ids.NewV7()
	}
	gone := named[3]
	frozen := []string{"not-an-id", named[0].String()}
	for _, id := range named {
		frozen = append(frozen, id.String())
	}
	asked := 0
	sendable := func(_ context.Context, id ids.UUID) (OutboundFile, error) {
		asked++
		if id == gone {
			return OutboundFile{}, fmt.Errorf("resolving an attached file: %w", apperrors.ErrNotFound)
		}
		return OutboundFile{AttachmentID: id, Filename: "quote.pdf"}, nil
	}

	files, err := sortFrozenFiles(context.Background(), frozen, sendable)
	if err != nil {
		t.Fatalf("a read with uncarriable ids refused: %v", err)
	}
	if asked != maxAttachmentsPerSend {
		t.Errorf("looked up %d files, want the cap of %d", asked, maxAttachmentsPerSend)
	}
	// One unparseable, one past the cap, one archived.
	if files.unavailable != 3 {
		t.Errorf("unavailable = %d, want 3", files.unavailable)
	}
	var want []ids.UUID
	for _, id := range named[:maxAttachmentsPerSend] {
		if id != gone {
			want = append(want, id)
		}
	}
	var got []ids.UUID
	for _, file := range files.attachable {
		got = append(got, file.AttachmentID)
	}
	if !slices.Equal(got, want) {
		t.Errorf("attachable = %v, want %v in frozen order", got, want)
	}
}

func TestALookupThatFailsStopsTheDetailRead(t *testing.T) {
	t.Parallel()
	sendable := func(context.Context, ids.UUID) (OutboundFile, error) {
		return OutboundFile{}, fmt.Errorf("resolving an attached file: %w", context.DeadlineExceeded)
	}
	if _, err := sortFrozenFiles(context.Background(), []string{ids.NewV7().String()}, sendable); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("a failed lookup read as %v, want it to stop the read", err)
	}
}
