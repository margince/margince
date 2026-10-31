// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// Two additions a reader needs and a model cannot work out alone. One is where
// to open a record, the other what an amount in minor units says in money.

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

type recordLinksKey struct{}

// withRecordLinks binds the origin evidence links are built on. An empty origin
// binds nothing, and evidence then carries no link.
func withRecordLinks(ctx context.Context, origin string) context.Context {
	if origin == "" {
		return ctx
	}
	return context.WithValue(ctx, recordLinksKey{}, origin)
}

func recordLinkOrigin(ctx context.Context) string {
	if origin, ok := ctx.Value(recordLinksKey{}).(string); ok {
		return origin
	}
	return ""
}

// linkEvidence gives each reference the address of its record page. A record
// type the app has no page for keeps no link rather than a guessed one.
func linkEvidence(refs []EvidenceRef, origin string) {
	if origin == "" {
		return
	}
	for i := range refs {
		fragment := mailcopy.RecordFragment(string(refs[i].RecordType), refs[i].RecordID.String())
		if fragment != "" {
			refs[i].WebURL = mailcopy.Destination(origin, fragment)
		}
	}
}

// minorSuffix and currencyMembers are how money is spelled on this surface:
// `<name>_minor` beside the `currency` it is counted in. A forecast's figures
// sit beside the `base_currency` they were converted to.
const (
	minorSuffix   = "_minor"
	displaySuffix = "_display"
	fieldCurrency = "currency"
)

var currencyMembers = []string{fieldCurrency, "base_currency"}

// withReadableAmounts adds `<name>_display` ("72000.00 USD") beside every
// `<name>_minor` that sits next to a currency. A model then reads money in major
// units and never divides by the minor-unit factor itself. The figure comes from
// values.MajorUnits, the same table the app formats with.
//
// Bytes with no money pair are returned untouched, so a result that carries no
// amount keeps the handler's exact rendering.
func withReadableAmounts(data json.RawMessage) json.RawMessage {
	if !bytes.Contains(data, []byte(minorSuffix+`"`)) || !bytes.Contains(data, []byte(`currency"`)) {
		return data
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var tree any
	if err := decoder.Decode(&tree); err != nil {
		return data
	}
	if !addDisplays(tree) {
		return data
	}
	out, err := json.Marshal(tree)
	if err != nil {
		return data
	}
	return out
}

// addDisplays walks the tree and reports whether it added any figure.
//
//craft:ignore naked-any the node is a decoded JSON value of unknown shape, and the walk is what reads its type
func addDisplays(node any) bool {
	added := false
	switch typed := node.(type) {
	case map[string]any:
		if currency := currencyOf(typed); currency != "" {
			added = addPairDisplays(typed, currency)
		}
		for _, child := range typed {
			added = addDisplays(child) || added
		}
	case []any:
		for _, child := range typed {
			added = addDisplays(child) || added
		}
	}
	return added
}

// currencyOf is the currency an object's amounts are counted in, if it names one.
func currencyOf(object map[string]any) string {
	for _, member := range currencyMembers {
		if currency, ok := object[member].(string); ok && currency != "" {
			return currency
		}
	}
	return ""
}

func addPairDisplays(object map[string]any, currency string) bool {
	displays := map[string]string{}
	for key, value := range object {
		base, isMinor := strings.CutSuffix(key, minorSuffix)
		number, isNumber := value.(json.Number)
		if !isMinor || !isNumber || base == "" {
			continue
		}
		if _, taken := object[base+displaySuffix]; taken {
			continue
		}
		minor, err := number.Int64()
		if err != nil {
			continue
		}
		displays[base+displaySuffix] = values.MajorUnits(minor, currency) + " " + currency
	}
	for key, display := range displays {
		object[key] = display
	}
	return len(displays) > 0
}
