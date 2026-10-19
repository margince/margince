// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

func (s *Store) controllerView(ctx context.Context, tx pgx.Tx, rendered Rendered, category commsauthz.Category) (Rendered, error) {
	lines, err := s.DisclosuresFor(ctx, tx, category)
	if err != nil {
		return Rendered{}, err
	}
	for _, line := range lines {
		if line.Missing() {
			return Rendered{}, &ValidationError{Field: "controller_particulars", Reason: "complete the required company and privacy contact details before sending"}
		}
		rendered.Body += "\n\n" + strings.TrimSpace(line.Text)
	}
	return rendered, nil
}
