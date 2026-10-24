// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"fmt"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/imagenorm"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// imageSource hands a photo or scan to the model through ai.PrepareImage, the
// image policy a card photo takes as well. Each refusal becomes the detail a
// rep reads on the panel.
func (d *DocumentExtractor) imageSource(
	ctx context.Context, meta crmcontracts.Attachment, mime string, bytes []byte,
) (documentSource, string, error) {
	img, err := ai.PrepareImage(ctx, d.brain.AttachmentMIMEs(),
		model.Attachment{MIME: mime, Bytes: bytes, Name: meta.Filename})
	var notCarried *ai.ImageNotCarriedError
	switch {
	case errors.As(err, &notCarried):
		return documentSource{}, fmt.Sprintf(documentWords.UnsupportedType.In(d.said.Lang()), mime), nil
	case errors.Is(err, ai.ErrImageTooLarge):
		return documentSource{}, fmt.Sprintf(documentWords.Bytes.In(d.said.Lang()), imagenorm.MaxReadImageBytes>>20), nil
	case errors.Is(err, model.ErrAttachmentMislabelled):
		return documentSource{}, documentWords.Mislabelled.In(d.said.Lang()), nil
	case err != nil:
		return documentSource{}, "", err
	}
	return documentSource{Part: img, Filename: meta.Filename, Parent: attachmentParent(meta)}, "", nil
}
