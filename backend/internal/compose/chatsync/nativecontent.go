// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func nativeContentDigest(messages []chatwire.Message) (string, error) {
	encoded, err := json.Marshal(messages)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func recheckNativeThread(ctx context.Context, reader chatwire.RootReader, channel, root, digest string) error {
	current, err := readNativeThread(ctx, reader, channel, root)
	if err != nil {
		return err
	}
	actual, err := nativeContentDigest(current)
	if err != nil {
		return err
	}
	if actual != digest {
		return apperrors.ErrConflict
	}
	return nil
}
