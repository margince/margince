// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// storedLogos holds logos by object key, as the blob store does.
type storedLogos struct{ asked []string }

func (s *storedLogos) SignatureLogo(_ context.Context, key string) (connector.InlineImage, bool, error) {
	s.asked = append(s.asked, key)
	if key != "logos/a" {
		return connector.InlineImage{}, false, nil
	}
	return connector.InlineImage{ContentID: connector.SignatureLogoContentID, ContentType: "image/png", Body: []byte("logo a")}, true, nil
}

// A delivery embeds the logo staged with it, and reads none when it staged none.
func TestADeliveryEmbedsTheLogoStagedWithIt(t *testing.T) {
	t.Parallel()
	logos := &storedLogos{}
	d := (&Dispatcher{}).WithInlineImages(logos)
	plain, err := d.inlineFor(context.Background(), Delivery{HTMLBody: "<p>Hello</p>"})
	if err != nil || len(plain) != 0 || len(logos.asked) != 0 {
		t.Fatalf("a delivery with no staged logo read %d image(s), asked %v, err %v", len(plain), logos.asked, err)
	}
	signed, err := d.inlineFor(context.Background(), Delivery{InlineLogoKey: "logos/a"})
	if err != nil || len(signed) != 1 || string(signed[0].Body) != "logo a" {
		t.Fatalf("the staged logo came back as %+v, err %v", signed, err)
	}
}

// A logo the workspace deleted after staging sends the message without it.
func TestADeletedStagedLogoSendsWithoutTheImage(t *testing.T) {
	t.Parallel()
	d := (&Dispatcher{}).WithInlineImages(&storedLogos{})
	images, err := d.inlineFor(context.Background(), Delivery{InlineLogoKey: "logos/replaced"})
	if err != nil || len(images) != 0 {
		t.Fatalf("a deleted logo gave %d image(s), err %v", len(images), err)
	}
}
