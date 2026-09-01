// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Which of OUR channel kinds a card's TYPE parameter means.
//
// Its own file because it is its own question, and the only part of the reader
// that is about this product rather than about vCard: everything beside it
// parses what the card says, while this decides what we call it. A card may
// carry anything in that position — INTERNET, PREF, an X- extension somebody
// invented — and the mapping is what keeps a guess out of the record.

import "strings"

// The vCard type names this reader recognizes. A card may carry anything here
// (INTERNET, PREF, X-custom); what it does NOT carry is this product's own
// vocabulary, so an unrecognized type folds onto `other` rather than being
// guessed at work.
const (
	vcardTypeHome    = "home"
	vcardTypeWork    = "work"
	vcardTypeCell    = "cell"
	vcardTypeMobile  = "mobile"
	channelKindOther = "other"
)

func emailKindFrom(params []string) string {
	switch typeParam(params) {
	case vcardTypeHome:
		return "personal"
	case vcardTypeWork, "":
		// A business card states a working address unless it says otherwise.
		return emailTypeWork
	default:
		return channelKindOther
	}
}

func phoneKindFrom(params []string) string {
	switch typeParam(params) {
	case vcardTypeCell, vcardTypeMobile:
		return vcardTypeMobile
	case vcardTypeHome:
		return vcardTypeHome
	case vcardTypeWork, "":
		return phoneTypeWork
	default:
		return channelKindOther
	}
}

// typeParam reads the value that NAMES A KIND, lowercased, from anywhere in the
// property's parameters.
//
// EVERY parameter and every value in it, in order, rather than the first value
// of the first TYPE. A card may spell one property's types as a value list
// (`TEL;TYPE=VOICE,WORK`) or as repeated parameters (`TEL;TYPE=voice;TYPE=work`)
// — RFC 2426 allows both — and reading only the first hands back `voice` for a
// number the card plainly calls a work number, which then folds onto `other`.
//
// Auxiliary values are read PAST. `pref` says which number is primary; `voice`
// and `fax` say what happens when you dial it; `internet` says how an address is
// delivered. None of them says WHERE it is, which is the only question here —
// and `EMAIL;TYPE=INTERNET,HOME` is the ordinary spelling of a private address
// in every vCard 2.1 exporter there is. They are remembered rather than discarded,
// so a property whose types are auxiliary all the way down still answers with
// one — `TEL;TYPE=FAX` is a fax and not a work phone — and only a property with
// nothing but `pref` answers with absence, which is what lets the caller apply
// its own default.
//
// An UNRECOGNIZED value is an answer too, and it is returned as it stands. It
// names a kind this product does not have, so the caller folds it onto `other`;
// dropping it here made it indistinguishable from a card that named no type at
// all, and `TEL;X-custom` was then filed as a work number.
func typeParam(params []string) string {
	auxiliary := ""
	for _, p := range params {
		if !strings.HasPrefix(strings.ToUpper(p), "TYPE=") {
			// vCard 2.1 writes a bare type ("TEL;WORK:..."), with no TYPE=. A
			// key=value parameter is not a type whatever it says, so it is not
			// read as one — VALUE=uri names an encoding, not a place.
			if strings.Contains(p, "=") {
				continue
			}
			if kind, aux := classifyVCardType(p); kind != "" {
				return kind
			} else if aux != "" && auxiliary == "" {
				auxiliary = aux
			}
			continue
		}
		for _, raw := range strings.Split(p[len("TYPE="):], ",") {
			if kind, aux := classifyVCardType(raw); kind != "" {
				return kind
			} else if aux != "" && auxiliary == "" {
				auxiliary = aux
			}
		}
	}
	return auxiliary
}

// classifyVCardType answers one type value as either a kind or an auxiliary.
//
// `pref` is neither: it is the one value that carries no information about the
// property at all, so a card naming only it has named no type, and the caller's
// default is the honest answer rather than `other`.
func classifyVCardType(raw string) (kind, auxiliary string) {
	v := strings.Trim(strings.ToLower(strings.TrimSpace(raw)), `"`)
	switch v {
	case "":
		return "", ""
	case "pref":
		return "", ""
	case "voice", "fax", "msg", "video", "textphone", "text", "internet", "x400":
		return "", v
	default:
		return v, ""
	}
}
