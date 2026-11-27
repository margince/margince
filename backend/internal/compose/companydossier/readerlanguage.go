// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companydossier

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/identity"
)

// readerLanguage is the language the dossier, the growth fit and their
// receipts are written in: the reader's own. Each is cached per reader, and
// its fingerprint names the language.
func readerLanguage(ctx context.Context, pool *pgxpool.Pool) string {
	return identity.ReaderLanguageForPrompt(ctx, pool)
}
