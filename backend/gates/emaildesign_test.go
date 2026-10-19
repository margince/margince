// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H1

package gates

import (
	"os"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/emailview"
)

func TestEmailColorsFollowTheProductTokens(t *testing.T) {
	t.Parallel()
	tokens, err := os.ReadFile("../frontend/src/design-system/tokens.css")
	if err != nil {
		t.Fatal(err)
	}
	for token, value := range map[string]string{"bgPage": emailview.Paper, "bgElevated": emailview.Surface, "accent": emailview.Accent, "textPrimary": emailview.Ink} {
		if !strings.Contains(string(tokens), "--"+token+": "+value+";") {
			t.Errorf("email %s = %s disagrees with product tokens", token, value)
		}
	}
	for token, value := range map[string]string{"bgPage": emailview.DarkPaper, "bgElevated": emailview.DarkSurface, "accent": emailview.DarkAccent, "textPrimary": emailview.DarkInk, "borderSubtle": emailview.DarkBorder} {
		if !strings.Contains(string(tokens), "--"+token+": "+value+";") {
			t.Errorf("dark email %s disagrees with tokens", token)
		}
	}
}
