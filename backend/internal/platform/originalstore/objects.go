// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package originalstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/blobstore"
)

// ErrEvidenceUnavailable distinguishes lost evidence from retryable transport failures.
var ErrEvidenceUnavailable = errors.New("stored original evidence is unavailable")

// Read checks length and digest before any consumer interprets stored evidence.
func Read(ctx context.Context, ref Reference) (payload []byte, err error) {
	s, err := serviceOf(ctx)
	if err != nil {
		return nil, err
	}
	ctx, err = s.workspaceContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := validate(ctx, ref); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, objectTimeout)
	defer cancel()
	body, object, err := s.blob.Get(ctx, ref.Key)
	if errors.Is(err, blobstore.ErrNotFound) {
		return nil, errors.Join(ErrEvidenceUnavailable, err)
	}
	if err != nil {
		return nil, fmt.Errorf("read provider original: %w", err)
	}
	defer func() {
		if closeErr := body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close provider original: %w", closeErr)
		}
	}()
	if object.Size != ref.Bytes {
		return nil, fmt.Errorf("stored original size differs from its reference: %w", ErrEvidenceUnavailable)
	}
	payload, err = io.ReadAll(io.LimitReader(body, ref.Bytes+1))
	if err != nil {
		return nil, fmt.Errorf("read provider original: %w", err)
	}
	sum := sha256.Sum256(payload)
	if int64(len(payload)) != ref.Bytes || hex.EncodeToString(sum[:]) != ref.SHA256 {
		return nil, fmt.Errorf("stored original integrity check failed: %w", ErrEvidenceUnavailable)
	}
	return payload, nil
}

// CountReturned counts the rows a DELETE ... RETURNING payload_object removed,
// and the objects among them. It deletes no object. The raw_capture trigger
// queues each key on the caller's transaction, and the stored-object reap
// deletes the bytes after commit. So a rolled-back delete keeps them.
func CountReturned(rows pgx.Rows) (records, objects int64, err error) {
	defer rows.Close()
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return 0, 0, err
		}
		records++
		if len(metadata) > 0 {
			objects++
		}
	}
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}
	return records, objects, nil
}

// RepairPayload lets a repair record unreadable evidence without blocking its
// cursor forever. Configuration and transport failures remain retryable errors.
func RepairPayload(ctx context.Context, wrapper []byte) ([]byte, error) {
	payload, err := Decode(ctx, wrapper)
	if errors.Is(err, ErrEvidenceUnavailable) {
		slog.WarnContext(ctx, "provider original unavailable; repair leaves its record unchanged")
		return nil, nil
	}
	return payload, err
}
