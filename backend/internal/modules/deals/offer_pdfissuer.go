// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The issuer block every offer PDF prints under its title. That is the legal
// entity's address, VAT ID, register entry and bank details, and its logo
// above the title. It is printed always, whatever the template's footer says.

import (
	"bytes"
	"image"
	"image/draw"
	"image/png"

	"github.com/go-pdf/fpdf"
)

// pdfLogoWidth is the logo's printed width in millimetres; its height follows
// the image.
const pdfLogoWidth = 40

// PDFIssuer is the issuer block with the logo's bytes the caller read for it.
type PDFIssuer struct {
	OfferIssuer
	// Logo is the stored PNG behind OfferIssuer.LogoKey, nil for none.
	Logo []byte
}

// writeOfferPDFIssuerBlock prints the lines under the issuer's name. A field
// the entity has no value for prints nothing.
func writeOfferPDFIssuerBlock(pdf *fpdf.Fpdf, tr pdfTranslator, issuer OfferIssuer, labels pdfLabels) {
	lines := append([]string{}, issuer.Address...)
	if issuer.VatID != "" {
		lines = append(lines, labels.vatID+": "+issuer.VatID)
	}
	if register := joinNonEmpty(" ", issuer.RegisterCourt, issuer.RegisterNumber); register != "" {
		lines = append(lines, labels.register+": "+register)
	}
	if bank := joinNonEmpty(" · ", issuer.BankAccountHolder, issuer.IBAN, issuer.BIC); bank != "" {
		lines = append(lines, labels.bank+": "+bank)
	}
	pdf.SetFont("Helvetica", "", 9)
	for _, line := range lines {
		pdf.Cell(0, 4.5, tr(line))
		pdf.Ln(4.5)
	}
	pdf.SetFont("Helvetica", "", 11)
}

// writeOfferPDFLogo embeds the issuer's logo at the top of the first page. A
// logo that does not decode as PNG is left out, so a damaged stored object
// never fails the whole document.
func writeOfferPDFLogo(pdf *fpdf.Fpdf, logo []byte) {
	flat, ok := flatPNG(logo)
	if !ok {
		return
	}
	options := fpdf.ImageOptions{ImageType: "PNG", ReadDpi: false}
	info := pdf.RegisterImageOptionsReader("issuer-logo", options, bytes.NewReader(flat))
	if info == nil || pdf.Err() {
		pdf.ClearError()
		return
	}
	left, top, _, _ := pdf.GetMargins()
	height := pdfLogoWidth * info.Height() / info.Width()
	pdf.ImageOptions("issuer-logo", left, top, pdfLogoWidth, height, false, options, 0, "")
	pdf.SetY(top + height + 4)
}

// flatPNG re-encodes a logo as 8-bit RGBA PNG, the one PNG shape fpdf embeds
// whatever bit depth or interlacing the stored file used.
func flatPNG(logo []byte) ([]byte, bool) {
	if len(logo) == 0 {
		return nil, false
	}
	decoded, err := png.Decode(bytes.NewReader(logo))
	if err != nil {
		return nil, false
	}
	flat := image.NewNRGBA(decoded.Bounds())
	draw.Draw(flat, flat.Bounds(), decoded, decoded.Bounds().Min, draw.Src)
	var out bytes.Buffer
	if err := png.Encode(&out, flat); err != nil {
		return nil, false
	}
	return out.Bytes(), true
}
