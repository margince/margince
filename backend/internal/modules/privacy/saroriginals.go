// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/originalstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func subjectExternalOriginalsTx(ctx context.Context, tx pgx.Tx, contact ids.ContactID, emails []string) ([]ids.UUID, error) {
	channels, err := contactChannelIdentities(ctx, tx, contact)
	if err != nil {
		return nil, err
	}
	matched, err := matchingExternalOriginalsTx(ctx, tx, rawSubjectScope{contact, emails, channels})
	return matched.ids, err
}

// The SQL disclosure gate runs before hydration; withheld originals never read
// their object and storage metadata never leaves the export boundary.
func resolveSAROriginals(ctx context.Context, rows []map[string]any) error {
	for _, row := range rows {
		value := row["payload"]
		if value == nil {
			continue
		}
		wrapped, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("encode original export value: %w", err)
		}
		payload, err := originalstore.Decode(ctx, wrapped)
		if err != nil {
			row["payload"] = nil
			row["content_disclosed"] = false
			slog.WarnContext(ctx, "subject original could not be restored; content withheld")
			continue
		}
		if err := json.Unmarshal(payload, &value); err != nil {
			return fmt.Errorf("decode original export value: %w", err)
		}
		row["payload"] = value
	}
	return nil
}

func sarOriginalArgs(contact ids.ContactID, external []ids.UUID) ([]any, string) {
	args := []any{contact}
	args = append(args, external)
	return args, fmt.Sprintf("$%d::uuid[]", len(args))
}
