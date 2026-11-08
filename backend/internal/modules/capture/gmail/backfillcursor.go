// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gmail

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	historyCursorPrefix   = "margince-gmail-v1:"
	checkpointEvery       = 10
	maxHistoryCursorBytes = 128 << 10
)

type historyCursor struct {
	IDs  []string `json:"ids"`
	Next *string  `json:"next"`
}

func parseHistoryCursor(token string) (historyCursor, bool, error) {
	if !strings.HasPrefix(token, historyCursorPrefix) {
		return historyCursor{}, false, nil
	}
	if len(token) > maxHistoryCursorBytes {
		return historyCursor{}, true, errors.New("gmail: history cursor exceeds its size limit")
	}
	encoded, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(token, historyCursorPrefix))
	if err != nil {
		return historyCursor{}, true, fmt.Errorf("gmail: invalid history cursor: %w", err)
	}
	var cursor historyCursor
	if err := json.Unmarshal(encoded, &cursor); err != nil {
		return cursor, true, fmt.Errorf("gmail: invalid history cursor: %w", err)
	}
	if err := cursor.validate(); err != nil {
		return cursor, true, err
	}
	return cursor, true, nil
}

func (cursor historyCursor) validate() error {
	if cursor.Next == nil || cursor.IDs == nil || len(cursor.IDs) > backfillPageSize {
		return errors.New("gmail: incomplete or oversized history cursor")
	}
	seen := make(map[string]bool, len(cursor.IDs))
	for _, id := range cursor.IDs {
		if id == "" || seen[id] {
			return errors.New("gmail: invalid message in history cursor")
		}
		seen[id] = true
	}
	return nil
}

func (cursor historyCursor) encode() (string, error) {
	encoded, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	token := historyCursorPrefix + base64.RawURLEncoding.EncodeToString(encoded)
	if len(token) > maxHistoryCursorBytes {
		return "", errors.New("gmail: history cursor exceeds its size limit")
	}
	return token, nil
}

func (w *pageWalk) saveCheckpoint(ctx context.Context) error {
	cursor := historyCursor{Next: &w.res.NextToken, IDs: make([]string, 0, len(w.ids)-len(w.settled))}
	for _, id := range w.ids {
		if !w.settled[id] {
			cursor.IDs = append(cursor.IDs, id)
		}
	}
	token, err := cursor.encode()
	if err != nil {
		return err
	}
	tally := w.res
	tally.Scanned = w.finished
	_, err = w.progress.Checkpoint(ctx, token, tally)
	if err == nil {
		w.savedAt = w.finished
	}
	return err
}

func (c *Connector) listHistoryPage(ctx context.Context, access, query, token string, gate *rateGate) (historyCursor, error) {
	cursor, prepared, err := parseHistoryCursor(token)
	if err != nil || prepared {
		return cursor, err
	}
	page, err := rateLimited(ctx, gate, func() (historyCursor, error) {
		ids, next, err := c.api.ListAfter(ctx, access, query, token, backfillPageSize)
		return historyCursor{IDs: ids, Next: &next}, err
	})
	if err != nil {
		return historyCursor{}, err
	}
	if page.IDs == nil {
		page.IDs = []string{}
	}
	return page, page.validate()
}
