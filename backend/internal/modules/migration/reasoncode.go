// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"slices"
	"strconv"
	"strings"
)

// ReasonCode names why a row was skipped or warned about, so a screen can
// say it in the reader's language. The English sentence stays beside it for a
// reader that does not know the code.
type ReasonCode string

// Params are the values a code's sentence names, as text.
type Params map[string]string

// Reason is one disclosed cause: the code and the values its sentence needs.
type Reason struct {
	Code   ReasonCode
	Params Params
}

// Why builds a Reason from a code and name, value pairs.
func Why(code ReasonCode, nameValue ...string) Reason {
	r := Reason{Code: code}
	if len(nameValue) > 1 {
		r.Params = Params{}
	}
	for i := 0; i+1 < len(nameValue); i += 2 {
		r.Params[nameValue[i]] = nameValue[i+1]
	}
	return r
}

// WhyCount is Why with one numeric value, as the sentences that count need.
func WhyCount(code ReasonCode, name string, n int) Reason {
	return Why(code, name, strconv.Itoa(n))
}

// English is the sentence a reader without the code's translation gets.
func (r Reason) English() string { return r.In(LangEnglish) }

// In renders the reason in lang, falling back to English for a language the
// catalog does not carry, and to the bare code for a code it does not know.
func (r Reason) In(lang string) string {
	entry, ok := reasonCatalog[r.Code]
	if !ok {
		return string(r.Code)
	}
	template, ok := entry.text[lang]
	if !ok {
		template = entry.text[LangEnglish]
	}
	return renderTemplate(template, r.Params)
}

// WithoutTheLink is the reason of a link cell the run dropped rather than
// skipping the row for. A code with no such twin is returned as it is.
func (r Reason) WithoutTheLink() Reason {
	if twin, ok := droppedTwin[r.Code]; ok {
		r.Code = twin
	}
	return r
}

// Languages a reason is rendered in. The catalog holds each code in all three.
const (
	LangEnglish    = "en"
	LangGerman     = "de"
	LangVietnamese = "vi"
)

// ReasonLanguages is every language the catalog renders, in a new slice.
func ReasonLanguages() []string {
	return []string{LangEnglish, LangGerman, LangVietnamese}
}

// ReasonCodes is every code a report may carry, sorted, in a new slice. The
// contract's ImportReasonCode enum is this same list, and a test holds the two
// equal.
func ReasonCodes() []ReasonCode {
	codes := make([]ReasonCode, 0, len(reasonCatalog))
	for code := range reasonCatalog {
		codes = append(codes, code)
	}
	slices.Sort(codes)
	return codes
}

// ReasonParamNames are the values a code's sentence names, sorted.
func ReasonParamNames(code ReasonCode) []string {
	names := slices.Clone(reasonCatalog[code].params)
	slices.Sort(names)
	return names
}

// ReasonTemplate is the catalog's sentence for a code, with its {name} and
// {name:q} placeholders in place. It answers "" for an unknown code.
func ReasonTemplate(code ReasonCode, lang string) string {
	return reasonCatalog[code].text[lang]
}

// renderTemplate fills {name} with the value as it is and {name:q} with the
// value in Go quotes, which is how the English sentences always quoted a cell.
func renderTemplate(template string, params Params) string {
	args := make([]string, 0, 4*len(params))
	for name, value := range params {
		args = append(args, "{"+name+"}", value, "{"+name+":q}", strconv.Quote(value))
	}
	return strings.NewReplacer(args...).Replace(template)
}
