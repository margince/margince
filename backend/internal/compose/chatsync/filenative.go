// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
)

const (
	messageDeleteType = "messageDelete"
)

const (
	activityKind = "activity"
	messageKind  = "message"
)

func (f *Filer) fileNativeMessage(ctx context.Context, session chatlinks.Session, link chatlinks.Link, reader chatwire.RootReader, reference string, wholeThread bool) (crmcontracts.Activity, error) {
	location, err := locateNativeMessage(ctx, session.Chat, link, reference)
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	original, err := readNativeThread(ctx, reader, link.ChannelSlug, location.Root)
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	digest, err := nativeContentDigest(original)
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	messages, err := chosenNativeMessages(original, location, wholeThread)
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	reply, err := f.summariseChosen(ctx, link, messages)
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	if err := recheckNativeThread(ctx, reader, link.ChannelSlug, location.Root, digest); err != nil {
		return crmcontracts.Activity{}, err
	}
	selected := location.Message
	if f.Reauthorize != nil {
		ctx, err = f.Reauthorize(ctx)
		if err != nil {
			return crmcontracts.Activity{}, err
		}
	}
	mode := messageKind
	if wholeThread {
		selected = location.Root
		mode = "thread"
	}
	// A one-off selection stays separate from the recurring recap, so background
	// edits cannot replace the scope the member chose to save.
	hash := sha256.Sum256([]byte(strings.Join([]string{link.ConnectionID.String(), link.ChannelSlug, selected, mode}, "\x00")))
	said := talkOf("chosen:"+hex.EncodeToString(hash[:]), chatlinks.ProviderChannelURL(link.Provider, session.TeamID, session.Address, link.ChannelSlug), messages)
	said.rootID = location.Root
	return fileNote(ctx, filing{reconfirm: f.Reconfirm, pool: f.Pool, store: f.Store, acts: f.Activities, link: link}, said, reply)
}

func chosenNativeMessages(messages []chatwire.Message, location chatwire.MessageLocation, wholeThread bool) ([]chatwire.Message, error) {
	found := false
	for _, message := range messages {
		if message.TS == location.Message && !message.Deleted && judged(message) {
			found = true
		}
	}
	if !found {
		return nil, chatlinks.ErrMessageNotFound
	}
	out := []chatwire.Message{}
	for _, message := range messages {
		if !message.Deleted && judged(message) && (wholeThread || message.TS == location.Message) {
			out = append(out, message)
		}
	}
	if len(out) == 0 {
		return nil, chatlinks.ErrMessageNotFound
	}
	return out, nil
}

func locateNativeMessage(ctx context.Context, chat chatlinks.Chat, link chatlinks.Link, reference string) (chatwire.MessageLocation, error) {
	locator, ok := chat.(chatwire.MessageLocator)
	if !ok {
		return chatwire.MessageLocation{}, messaging.ErrUnsupported
	}
	location, err := locator.LocateMessage(ctx, link.ChannelSlug, reference)
	if errors.Is(err, messaging.ErrInvalid) {
		return chatwire.MessageLocation{}, chatlinks.ErrMessageNotFound
	}
	if err != nil {
		return chatwire.MessageLocation{}, chatlinks.ChatFailure(err)
	}
	if location.Message == "" || location.Root == "" || link.RootID != "" && link.RootID != location.Root {
		return chatwire.MessageLocation{}, chatlinks.ErrMessageNotFound
	}
	return location, nil
}
