// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/emailview"
)

// A mail that speaks as Margince must carry the mark its markup points at,
// or the client shows a broken image.
func TestAMailSpeakingAsMarginceCarriesTheMarkItDraws(t *testing.T) {
	head, inline := mailLetterhead{}.Letterhead(context.Background(), "en")
	if head.MarkContentID != emailview.BrandMarkContentID {
		t.Fatalf("the letterhead names mark %q, want %q", head.MarkContentID, emailview.BrandMarkContentID)
	}
	if len(inline) != 1 || inline[0].ContentID != head.MarkContentID || inline[0].ContentType != "image/png" || len(inline[0].Body) == 0 {
		t.Fatalf("the send attaches %+v, want one PNG named %q", inline, head.MarkContentID)
	}
	markup := emailview.Render(emailview.Message{Title: "Your day", Text: "Two tasks are due."}, head)
	if !strings.Contains(markup, `src="cid:`+inline[0].ContentID+`"`) {
		t.Fatal("the markup does not point at the attached mark")
	}
}
