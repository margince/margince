// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companydossier

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A Vietnamese reader gets the dossier in Vietnamese, whatever language the
// installation runs in. The language is the reader's choice, so no read of
// the installation is needed.
func TestTheDossierIsWrittenInTheReadersLanguage(t *testing.T) {
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "user:" + ids.NewV7().String(), Locale: "vi",
	})
	if got := readerLanguage(ctx, nil); got != "vi" {
		t.Errorf("a Vietnamese reader's dossier language is %q, want vi", got)
	}
}
