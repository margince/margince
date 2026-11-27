// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"errors"
	"strings"

	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/modules/chatlinks/dispact"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
)

// createChannel makes the record's channel under the record's name. Deals
// often share a name, so a name the chat refuses is asked once more in a form
// derived from the record. A blank name starts in that form.
func createChannel(ctx context.Context, chat Chat, kind string, id ids.UUID, name string) (chatwire.Channel, error) {
	name = channelNameOf(name)
	if name == "" {
		name = kind + "-" + idSuffix(id)
	}
	ch, err := chat.CreateChannel(ctx, externalKey(kind, id), name)
	if errors.Is(err, dispact.ErrNameRefused) {
		ch, err = chat.CreateChannel(ctx, externalKey(kind, id), fallbackName(kind, id, name, err))
	}
	return ch, ChatFailure(err)
}

// fallbackName tells a taken name apart by the end of the record's id. A name
// the chat's rules refuse is replaced by the record's kind and that id.
func fallbackName(kind string, id ids.UUID, name string, refusal error) string {
	suffix := idSuffix(id)
	var coded *dispact.CodeError
	if errors.As(refusal, &coded) && coded.Code == dispact.CodeNameTaken {
		if kept := strings.TrimSpace(textcut.Runes(name, channelNameLimit-len(suffix)-1)); kept != "" {
			return kept + " " + suffix
		}
	}
	return kind + "-" + suffix
}

// idSuffix is the end of a record's id, its random part.
func idSuffix(id ids.UUID) string {
	full := id.String()
	return full[len(full)-6:]
}

// externalKey names the channel Margince made for a record, so a retried
// create finds it instead of making a second one.
func externalKey(kind string, id ids.UUID) string {
	return "margince:" + kind + ":" + id.String()
}

// channelNameLimit is the longest channel name the chat takes, in characters.
const channelNameLimit = 80

// channelNameOf keeps a record's name within the chat's limit.
func channelNameOf(name string) string {
	return textcut.Runes(strings.TrimSpace(name), channelNameLimit)
}
