// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The edge from identity's seat writers to contacts, which owns the anchor
// company whose existence decides whether a seat may be added at all.

import (
	"context"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
)

// installationDescribed answers from the anchor's STANDING, not its profile: an
// admin delegated user_admin may hold no company grant, and this is a yes/no.
func installationDescribed(store *contacts.Store) identity.InstallationDescribed {
	return func(ctx context.Context) (bool, error) {
		exists, _, err := store.AnchorProfileStanding(ctx)
		return exists, err
	}
}
