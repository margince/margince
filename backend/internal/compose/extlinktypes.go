// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
	"github.com/margince/margince/backend/pkg/extension"
)

// registerLinkTypes fills the registry the recordlinks module validates
// against, the one the record kinds already live in.
func registerLinkTypes(exts []extension.Extension) {
	for _, e := range exts {
		recordkinds.RegisterLinkTypes(e.Name, e.LinkTypes)
	}
}
