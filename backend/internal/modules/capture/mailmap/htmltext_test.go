// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture/htmltext"
)

// bodyText is where the choice between the two MIME parts is made, and the
// plain part must keep winning: it is what the sender's own client composed.
func TestBodyTextPrefersThePlainPart(t *testing.T) {
	t.Parallel()
	got, _ := bodyText("Anbei das Angebot.", "<p>Anbei das <b>Angebot</b>.</p>")
	if got != "Anbei das Angebot." {
		t.Errorf("bodyText() = %q, want the plain part verbatim", got)
	}
}

func TestBodyTextRendersHTMLWhenThereIsNoPlainPart(t *testing.T) {
	t.Parallel()
	got, _ := bodyText("   ", "<p>Anbei das Angebot.</p><p>Bis Dienstag.</p>")
	if got != "Anbei das Angebot.\n\nBis Dienstag." {
		t.Errorf("bodyText() = %q, want the rendered HTML", got)
	}
}

func TestBodyTextIsEmptyWhenTheMessageCarriedNoText(t *testing.T) {
	t.Parallel()
	if got, _ := bodyText("", ""); got != "" {
		t.Errorf("bodyText() = %q, want empty", got)
	}
}

// The render cap is derived from the stored length, so the two must not drift.
func TestTheRenderCapCoversTheStoredBody(t *testing.T) {
	t.Parallel()
	if htmltext.StoredLen != maxBodyLen {
		t.Errorf("htmltext.StoredLen = %d, want maxBodyLen %d", htmltext.StoredLen, maxBodyLen)
	}
}
