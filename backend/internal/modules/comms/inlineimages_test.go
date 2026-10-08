// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type fixedLogo struct{ calls int }

func (f *fixedLogo) SignatureLogo(context.Context) (connector.InlineImage, bool, error) {
	f.calls++
	return connector.InlineImage{ContentID: connector.SignatureLogoContentID, ContentType: "image/png", Body: []byte("png")}, true, nil
}

// The logo is read only for markup that shows it.
func TestTheLogoRidesOnlyWithMarkupThatShowsIt(t *testing.T) {
	t.Parallel()
	logo := &fixedLogo{}
	d := (&Dispatcher{}).WithInlineImages(logo)
	plain, err := d.inlineFor(context.Background(), "<p>Hello</p>")
	if err != nil || len(plain) != 0 || logo.calls != 0 {
		t.Fatalf("markup with no logo read %d image(s), %d call(s), err %v", len(plain), logo.calls, err)
	}
	signed, err := d.inlineFor(context.Background(), `<img src="cid:`+connector.SignatureLogoContentID+`">`)
	if err != nil || len(signed) != 1 || signed[0].ContentID != connector.SignatureLogoContentID {
		t.Fatalf("markup showing the logo got %+v, err %v", signed, err)
	}
}
