// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailer

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/emailview"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
)

func TestAccountActionsUseTheirOwnLabels(t *testing.T) {
	words := mailcopy.For("de")
	message := words.AccountMessage(false, "", "", "https://example.test/#/reset")
	markup := emailview.Render(message)
	if !strings.Contains(markup, ">Neues Passwort festlegen</a>") {
		t.Fatal("reset action inherited the subject instead of its label")
	}
}
