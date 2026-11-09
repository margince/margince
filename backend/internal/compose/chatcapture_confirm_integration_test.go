// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"strings"
	"sync"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func strangerChatReply() string {
	return reply(
		statedField(chatFieldText, chatScreenshotText, "Lena Hoffmann: Are we still on for Thursday?"),
		statedField(chatFieldName, "Jonas Reiter", "Jonas Reiter"),
		notStated(chatFieldPhone))
}

func (f chatFixture) fileConfirmed(t *testing.T, confirm bool) (ids.UUID, error) {
	t.Helper()
	body := chatScreenshotText
	out, err := NewChatCaptureFile(f.env.Pool).File(f.env.Admin(), f.attachment, crmcontracts.FileChatCaptureRequest{
		ExtractionId: openapi_types.UUID(f.reading), ChannelProvider: "whatsapp", Body: &body,
		ConfirmOtherCounterparty: &confirm,
	})
	return ids.UUID(out.ActivityId), err
}

func TestAChatShowingSomeoneElseFilesOnlyOnceConfirmed(t *testing.T) {
	f := readChatScreenshot(t, strangerChatReply())
	read, err := f.store.GetExtractionRead(f.env.Admin(), f.attachment, f.reading)
	if err != nil {
		t.Fatal(err)
	}
	if shows, err := f.store.OtherChatCounterparty(f.env.Admin(), read); err != nil || shows != "Jonas Reiter" {
		t.Fatalf("the reading does not say the chat shows someone else: %q %v", shows, err)
	}
	if _, err := f.fileConfirmed(t, false); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("a chat showing someone else filed without a confirm: %v", err)
	}
	if n := f.chatActivities(t); n != 0 {
		t.Fatalf("the refused confirm wrote %d activities", n)
	}
	if _, err := f.fileConfirmed(t, true); err != nil {
		t.Fatalf("a confirmed chat was refused: %v", err)
	}
}

func TestAChatShowingTheContactNeedsNoConfirm(t *testing.T) {
	f := readChatScreenshot(t, chatReply())
	read, err := f.store.GetExtractionRead(f.env.Admin(), f.attachment, f.reading)
	if err != nil {
		t.Fatal(err)
	}
	if shows, err := f.store.OtherChatCounterparty(f.env.Admin(), read); err != nil || shows != "" {
		t.Fatalf("a chat with the contact's own name is reported as someone else: %q %v", shows, err)
	}
	if _, err := f.fileConfirmed(t, false); err != nil {
		t.Fatalf("a chat showing the contact asked for a confirm: %v", err)
	}
}

func TestAChatBodyIsHeldToItsLimitInCharacters(t *testing.T) {
	f := readChatScreenshot(t, chatReply())
	over := strings.Repeat("ü", activities.MaxChatText+1)
	_, err := f.file(t, &over)
	var refused *activities.ChatCaptureError
	if !errors.As(err, &refused) || refused.Field != "body" {
		t.Fatalf("a body one character over the limit was not refused by field: %v", err)
	}
	atLimit := strings.Repeat("ü", activities.MaxChatText)
	if _, err := f.file(t, &atLimit); err != nil {
		t.Fatalf("a body at the limit was refused: %v", err)
	}
}

// Two confirms of one reading race. The attachment lock makes the second wait
// and then answer the first one's activity.
func TestTwoConcurrentConfirmsFileOneActivity(t *testing.T) {
	f := readChatScreenshot(t, chatReply())
	var wg sync.WaitGroup
	results := make([]ids.UUID, 2)
	errs := make([]error, 2)
	for i := range 2 {
		wg.Go(func() { results[i], errs[i] = f.fileConfirmed(t, false) })
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("confirm %d failed: %v", i, err)
		}
	}
	if results[0] != results[1] || f.chatActivities(t) != 1 {
		t.Fatalf("two confirms filed %d activities (%s, %s)", f.chatActivities(t), results[0], results[1])
	}
}
