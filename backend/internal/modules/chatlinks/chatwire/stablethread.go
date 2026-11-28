// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatwire

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"
	"unicode/utf8"
)

// StableThread requires stable canonical source state before recap construction.
func StableThread(ctx context.Context, read func(context.Context) (ThreadPage, error)) (ThreadPage, error) {
	first, err := read(ctx)
	if err != nil {
		return ThreadPage{}, err
	}
	revision, err := threadRevision(first)
	if err != nil {
		return ThreadPage{}, err
	}
	confirmed, err := read(ctx)
	if err != nil {
		return ThreadPage{}, err
	}
	latest, err := threadRevision(confirmed)
	if err != nil {
		return ThreadPage{}, err
	}
	if latest != revision {
		return ThreadPage{}, ErrUnreachable
	}
	first.Revision = revision
	return first, nil
}

func threadRevision(page ThreadPage) (string, error) {
	if page.Next != "" || len(page.Messages) > 1000 || !page.Deleted && len(page.Messages) == 0 {
		return "", ErrThreadTooLong
	}
	seen := map[string]bool{}
	runes := 0
	for _, message := range page.Messages {
		if message.TS == "" || seen[message.TS] {
			return "", ErrUnreachable
		}
		seen[message.TS] = true
		runes += utf8.RuneCountInString(message.Text)
		if runes > 120000 {
			return "", ErrThreadTooLong
		}
	}
	slices.SortFunc(page.Messages, func(a, b Message) int { return strings.Compare(a.TS, b.TS) })
	encoded, err := json.Marshal(page)
	if err != nil {
		return "", ErrUnreachable
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}
