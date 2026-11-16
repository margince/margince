// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"fmt"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

// disclosureLayout is what decides the words and the order of one message's
// disclosures, beyond which ones are owed.
type disclosureLayout struct {
	// marketing is whether the message is advertising.
	marketing bool
	// language is the message's language; baseLanguage the language the
	// installation's default text is written in.
	language, baseLanguage string
	// packLanguage is a recipient pack's DisclosureLanguage, which advertising
	// particulars must be stated in.
	packLanguage string
	// refusalLast closes advertising with the refusal section, the
	// advertiser's block directly before it.
	refusalLast bool
}

// layoutDisclosures renders each owed disclosure in the words the layout asks
// for, in the order it asks for. A line whose particular is not stated in the
// required language, or lacks a means its disclosure names, is Missing.
func layoutDisclosures(owed []messagingrules.Disclosure, p ControllerParticulars, l disclosureLayout) []DisclosureLine {
	out := make([]DisclosureLine, 0, len(owed))
	for _, d := range owed {
		// MarketingOnly is the pack's own narrowing. The objection route is
		// owed on advertising and not on an invoice, where it would read as an
		// unsubscribe line under a payment reminder.
		if d.MarketingOnly && !l.marketing {
			continue
		}
		text := particularFor(d.Kind, l.particularsFor(d.Kind, p))
		if len(meansMissing(text, d.Means)) > 0 {
			text = ""
		}
		out = append(out, DisclosureLine{Kind: string(d.Kind), Text: text})
	}
	if !l.marketing || !l.refusalLast {
		return out
	}
	return l.refusalSectionLast(out, l.advertisingParticulars(p))
}

// particularsFor answers the version of p a disclosure of kind prints.
//
// An advertising particular owed in a pack language is that version
// or nothing: the default text counts only when it is written in that
// language. Every other particular prints the message's language where one is
// stated, else the default text.
func (l disclosureLayout) particularsFor(kind messagingrules.DisclosureKind, p ControllerParticulars) ControllerParticulars {
	if !l.marketing || l.packLanguage == "" || dataProtectionDisclosure(kind) {
		return p.In(l.language)
	}
	return l.advertisingParticulars(p)
}

// advertisingParticulars is p as advertising in the pack language prints it. The
// name and address fall back to the default text; the objection route and
// the advertiser contact do not.
func (l disclosureLayout) advertisingParticulars(p ControllerParticulars) ControllerParticulars {
	if l.packLanguage == "" {
		return p.In(l.language)
	}
	out := p.In(l.packLanguage)
	if l.baseLanguage == l.packLanguage {
		return out
	}
	stated, _ := p.translation(l.packLanguage)
	out.ObjectionRoute, out.AdvertiserContact = stated.ObjectionRoute, stated.AdvertiserContact
	return out
}

// refusalSectionLast moves the advertiser's block and the refusal section to
// the end, in that order.
//
// The advertiser's block carries the name and address with the contact. So a
// controller identity line would repeat them, and is folded into the block.
// The refusal states the right to refuse, the route, and the data choice.
func (l disclosureLayout) refusalSectionLast(lines []DisclosureLine, advertising ControllerParticulars) []DisclosureLine {
	language := l.packLanguage
	if language == "" {
		language = l.language
	}
	words := mailcopy.For(language)
	hasAdvertiser := slices.ContainsFunc(lines, func(line DisclosureLine) bool {
		return line.Kind == string(messagingrules.AdvertiserContact)
	})
	var closing []DisclosureLine
	out := make([]DisclosureLine, 0, len(lines))
	for _, line := range lines {
		switch messagingrules.DisclosureKind(line.Kind) {
		case messagingrules.ControllerIdentity:
			if !hasAdvertiser {
				out = append(out, line)
			}
		case messagingrules.AdvertiserContact:
			if line.Text != "" {
				line.Text = bothParticulars(advertising.LegalName, advertising.PostalAddress, line.Text)
			}
			closing = append([]DisclosureLine{line}, closing...)
		case messagingrules.ObjectionRoute:
			name := strings.TrimSpace(advertising.LegalName)
			if line.Text == "" || name == "" {
				line.Text = ""
			} else {
				line.Text = fmt.Sprintf(words.AdvertisingRefusal, mailcopy.OneLine(name)) + "\n" +
					line.Text + "\n" + words.AdvertisingDataChoice
			}
			closing = append(closing, line)
		default:
			out = append(out, line)
		}
	}
	return append(out, closing...)
}
